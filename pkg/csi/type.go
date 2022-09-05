/*
Copyright © 2021 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package csi

import (
	"fmt"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	log "k8s.io/klog/v2"
)

const (
	DefaultEndpoint                    string = "unix://tmp/csi.sock"
	DefaultDriverName                  string = "local.csi.aliyun.com"
	DefaultEphemeralVolumeDataFilePath string = "/var/lib/kubelet/open-local-volumes.json"
	// connection timeout
	DefaultConnectTimeout = 3
	// VolumeOperationAlreadyExists is message fmt returned to CO when there is another in-flight call on the given volumeID
	VolumeOperationAlreadyExists = "An operation with the given volume=%q is already in progress"

	// VgNameTag is the vg name tag
	VgNameTag = "vgName"
	// VolumeTypeTag is the pv type tag
	VolumeTypeTag = "volumeType"
	// PvTypeTag is the pv type tag
	PvTypeTag = "pvType"
	// FsTypeTag is the fs type tag
	FsTypeTag = "fsType"
	// LvmTypeTag is the lvm type tag
	LvmTypeTag = "lvmType"
	// NodeAffinity is the pv node schedule tag
	NodeAffinity = "nodeAffinity"
	// DefaultFs default fs
	DefaultFs = "ext4"
	// DefaultNodeAffinity default NodeAffinity
	DefaultNodeAffinity = "true"
	// LinearType linear type
	LinearType = "linear"
	// DirectTag is direct-assigned volume tag
	DirectTag = "direct"
	// StripingType striping type
	StripingType = "striping"
)

type CSIPlugin struct {
	*nodeServer
	*controllerServer
	srv     *grpc.Server
	options *driverOptions
}

var (
	VolumeCaps = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}

	// controllerCaps represents the capability of controller service
	ControllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	}

	NodeCaps = []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
	}
)

type PvcPodSchedulerMap struct {
	mux  *sync.RWMutex
	info map[string]string
}

func newPvcPodSchedulerMap() *PvcPodSchedulerMap {
	return &PvcPodSchedulerMap{
		mux:  &sync.RWMutex{},
		info: make(map[string]string),
	}
}

func (infoMap *PvcPodSchedulerMap) Add(pvcNamespace, pvcName, schedulerName string) {
	infoMap.mux.Lock()
	defer infoMap.mux.Unlock()

	infoMap.info[fmt.Sprintf("%s/%s", pvcNamespace, pvcName)] = schedulerName
}

func (infoMap *PvcPodSchedulerMap) Get(pvcNamespace, pvcName string) string {
	infoMap.mux.RLock()
	defer infoMap.mux.RUnlock()

	return infoMap.info[fmt.Sprintf("%s/%s", pvcNamespace, pvcName)]
}

func (infoMap *PvcPodSchedulerMap) Remove(pvcNamespace, pvcName string) {
	infoMap.mux.Lock()
	defer infoMap.mux.Unlock()

	delete(infoMap.info, fmt.Sprintf("%s/%s", pvcNamespace, pvcName))
}

type SchedulerArch string

var (
	SchedulerArchExtender  SchedulerArch = "extender"
	SchedulerArchFramework SchedulerArch = "scheduling-framework"
	SchedulerArchUnknown   SchedulerArch = "unknown"
)

type SchedulerArchMap struct {
	info map[string]SchedulerArch
}

func newSchedulerArchMap(extenderSchedulerNames []string, frameworkSchedulerNames []string) *SchedulerArchMap {
	info := make(map[string]SchedulerArch)
	for _, name := range extenderSchedulerNames {
		info[name] = SchedulerArchExtender
	}
	for _, name := range frameworkSchedulerNames {
		info[name] = SchedulerArchFramework
	}
	log.V(2).Infof("schedulerArchMap info is %v", info)
	return &SchedulerArchMap{
		info: info,
	}
}

func (archMap *SchedulerArchMap) Get(schedulerName string) SchedulerArch {
	arch, exist := archMap.info[schedulerName]
	if !exist {
		return SchedulerArchUnknown
	}
	return arch
}
