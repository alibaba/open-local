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

package cache

import (
	"sync"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
)

type InlineVolumeInfo struct {
	VgName       string `json:"vgName,string"`
	VolumeName   string `json:"volumeName,string"`
	VolumeSize   int64  `json:"volumeSize,string"`
	PodName      string `json:"podName,string"`
	PodNamespace string `json:"podNamespace,string"`
	Recorded     bool   `json:"recorded,string"`
}

type NodeInfo struct {
	NodeName string
	// SupportSPDK indicate if SPDK is supported
	SupportSPDK bool
	// VGs is the volume group
	VGs         map[ResourceName]SharedResource
	MountPoints map[ResourceName]ExclusiveResource
	// Devices only contains the whitelist raw devices
	Devices             map[ResourceName]ExclusiveResource
	AllocatedNum        int64
	LocalPVs            map[string]corev1.PersistentVolume
	PodInlineVolumeInfo map[string][]InlineVolumeInfo
}

type NodeCache struct {
	rwLock sync.RWMutex
	NodeInfo
}

type ResourceType string
type ResourceName string

const (
	SharedResourceType    ResourceType = "Shared"
	ExclusiveResourceType ResourceType = "Exclusive"
)

type ExclusiveResource struct {
	Name      string              `json:"name"`
	Device    string              `json:"device"`
	Capacity  int64               `json:"capacity,string"`
	MediaType localtype.MediaType `json:"mediaType"`
	// "IsAllocated = true" means the disk is used by PV
	IsAllocated bool `json:"isAllocated,string"`
}

type SharedResource struct {
	Name      string `json:"name"`
	Capacity  int64  `json:"capacity,string"`
	Requested int64  `json:"requested,string"`
}

type AllocatedUnit struct {
	NodeName   string
	VolumeType localtype.VolumeType
	Requested  int64 // requested size from pvc
	Allocated  int64 // actual allocated size for the pvc
	VgName     string
	Device     string
	MountPoint string
	PVCName    string
}

// pvc and binding info mapping
type BindingMap map[string]*AllocatedUnit

func (bm BindingMap) IsPVCExists(pvc string) bool {
	if bm == nil {
		return false
	}
	if _, ok := bm[pvc]; ok {
		return ok
	}
	return false
}

// PvcStatus refers to the pvc(for a pod) and its status of selected node
// string => pvc namespace/name
// bool => true/false true means volume.kubernetes.io/selected-node
type PvcStatusInfo map[string]bool

type PodPvcMapping struct {
	// PodPvcStatus count ref for a specified pod
	// string => pod namespace/name
	// PvcStatus => pvc status
	// podName <=> pvcStatusInfo
	PodPvcInfo map[string]PvcStatusInfo
	// store the pvc to pod mapping for faster index by pvc
	// pvcName <=> podName
	PvcPod map[string]string
}

func NewPodPvcMapping() *PodPvcMapping {
	return &PodPvcMapping{
		PodPvcInfo: make(map[string]PvcStatusInfo),
		PvcPod:     make(map[string]string),
	}
}

func NewPvcStatusInfo() PvcStatusInfo {
	return make(PvcStatusInfo)
}

// PutPod adds or updates the pod and pvc mapping
// it assures they are open-local type and contain all the requested PVCs
func (p *PodPvcMapping) PutPod(podName string, pvcs []*corev1.PersistentVolumeClaim) {
	info := NewPvcStatusInfo()
	var pvcName string
	for _, pvc := range pvcs {
		f := utils.PvcContainsSelectedNode(pvc)
		pvcName = utils.PVCName(pvc)
		info[pvcName] = f
		p.PvcPod[pvcName] = podName
		p.PodPvcInfo[podName] = info
		log.V(6).Infof("[Put]pvc (%s on %s) status changed to %t ", pvcName, podName, f)
	}
}

// DeletePod deletes pod and all its pvcs for cache
func (p *PodPvcMapping) DeletePod(podName string, pvcs []*corev1.PersistentVolumeClaim) {
	var pvcName string
	delete(p.PodPvcInfo, podName)
	log.V(6).Infof("[DeletePod]deleted pod cache %s", podName)

	for _, pvc := range pvcs {
		pvcName = utils.PVCName(pvc)
		delete(p.PvcPod, pvcName)
		log.V(6).Infof("[DeletePod]deleted pvc %s from cache", pvcName)
	}
}

// PutPvc adds or updates the pod and pvc mapping
// it assure the pvcs contains all the requested PVCs
func (p *PodPvcMapping) PutPvc(pvc *corev1.PersistentVolumeClaim) {
	pvcName := utils.PVCName(pvc)
	podName := p.PvcPod[pvcName]
	info := p.PodPvcInfo[podName]
	if len(podName) <= 0 || info == nil {
		log.V(6).Infof("pvc %s is not yet in pvc mapping", utils.PVCName(pvc))
		return
	}
	f := utils.PvcContainsSelectedNode(pvc)
	info[pvcName] = f
	log.V(6).Infof("[PutPvc]pvc (%s on %s) status changed to %t ", pvcName, podName, f)
}

// DeletePvc deletes pvc key from change
func (p *PodPvcMapping) DeletePvc(pvc *corev1.PersistentVolumeClaim) {
	pvcName := utils.PVCName(pvc)
	delete(p.PvcPod, pvcName)
	log.V(6).Infof("[DeletePvc]deleted pvc %s from cache", pvcName)
}

// IsPodPvcReady defines whether a pvc and its related pvcs are ready(with selected node)
// for pv provisioning; it returns true only when all pvcs of a pod are marked as ready for
// provisioning.
func (p *PodPvcMapping) IsPodPvcReady(pvc *corev1.PersistentVolumeClaim) bool {
	pvcName := utils.PVCName(pvc)
	podName := p.PvcPod[pvcName]
	pvcInfo := p.PodPvcInfo[podName]
	for _, selected := range pvcInfo {
		if !selected {
			return false
		}
	}
	return true
}
