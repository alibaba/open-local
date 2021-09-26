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

package pkg

import (
	"fmt"
	"time"

	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
)

type VolumeType string
type MediaType string
type StrategyType string

const (
	Gi          uint64 = 1024 * 1024 * 1024
	Mi          uint64 = 1024 * 1024
	DefaultPort int32  = 23000

	StrategyBinpack StrategyType = "binpack"
	StrategySpread  StrategyType = "spread"

	AgentName           string = "open-local-agent"
	ProvisionerNameYoda string = "yodaplugin.csi.alibabacloud.com"
	ProvisionerName     string = "local.csi.aliyun.com"
	SchedulerName       string = "open-local-scheduler"

	EnvLogLevel = "LogLevel"
	LogPanic    = "Panic"
	LogFatal    = "Fatal"
	LogError    = "Error"
	LogWarn     = "Warn"
	LogInfo     = "Info"
	LogDebug    = "Debug"
	LogTrace    = "Trace"

	KubernetesNodeIdentityKey = "kubernetes.io/hostname"
	VolumeTypeKey             = "volumeType"
	VolumeFSTypeKey           = "fsType"
	VolumeMediaType           = "mediaType"
	VolumeFSTypeExt4          = "ext4"
	VolumeFSTypeExt3          = "ext3"
	VolumeFSTypeXFS           = "xfs"

	PVCName      = "csi.storage.k8s.io/pvc/name"
	PVCNameSpace = "csi.storage.k8s.io/pvc/namespace"
	VGName       = "vgName"
	MPName       = "MountPoint"
	DeviceName   = "Device"

	// VolumeType MUST BE case sensitive
	VolumeTypeMountPoint VolumeType = "MountPoint"
	VolumeTypeLVM        VolumeType = "LVM"
	VolumeTypeDevice     VolumeType = "Device"
	VolumeTypeQuota      VolumeType = "Quota"
	VolumeTypeUnknown    VolumeType = "Unknown"
	MediaTypeSSD         MediaType  = "ssd"
	MediaTypeHHD         MediaType  = "hdd"
	MediaTypeUnspecified MediaType  = "Unspecified"

	// This annotation is added to a PVC that has been triggered by scheduler to
	// be dynamically provisioned. Its value is the name of the selected node.
	AnnSelectedNode                      = "volume.kubernetes.io/selected-node"
	LabelReschduleTimestamp              = "pod.oecp.io/reschdule-timestamp"
	EnvExpandSnapInterval                = "Expand_Snapshot_Interval"
	EnvForceCreateVG                     = "Force_Create_VG"
	TagSnapshot                          = "SnapshotName"
	PendingWithoutScheduledFieldSelector = "status.phase=Pending,spec.nodeName="
	TriggerPendingPodCycle               = time.Second * 300

	ParamSnapshotName            = "yoda.io/snapshot-name"
	ParamSnapshotReadonly        = "csi.aliyun.com/readonly"
	ParamSnapshotInitialSize     = "csi.aliyun.com/snapshot-initial-size"
	ParamSnapshotThreshold       = "csi.aliyun.com/snapshot-expansion-threshold"
	ParamSnapshotExpansionSize   = "csi.aliyun.com/snapshot-expansion-size"
	EnvSnapshotPrefix            = "SNAPSHOT_PREFIX"
	DefaultSnapshotPrefix        = "snap"
	DefaultSnapshotInitialSize   = 4 * 1024 * 1024 * 1024
	DefaultSnapshotThreshold     = 0.5
	DefaultSnapshotExpansionSize = 1 * 1024 * 1024 * 1024

	Separator = "<:SEP:>"

	// lv tags
	Lvm2LVNameTag        = "LVM2_LV_NAME"
	Lvm2LVSizeTag        = "LVM2_LV_SIZE"
	Lvm2LVKernelMajorTag = "LVM2_LV_KERNEL_MAJOR"
	Lvm2LVKernelMinorTag = "LVM2_LV_KERNEL_MINOR"
	Lvm2LVAttrTag        = "LVM2_LV_ATTR"
	Lvm2LVUuidTag        = "LVM2_LV_UUID"
	Lvm2CopyPercentTag   = "LVM2_COPY_PERCENT"
	Lvm2LVTagsTag        = "LVM2_LV_TAGS"

	// vg tags
	Lvm2VGNameTag  = "LVM2_VG_NAME"
	Lvm2VGSizeTag  = "LVM2_VG_SIZE"
	Lvm2VGFreeTag  = "LVM2_VG_FREE"
	Lvm2VGUuidTag  = "LVM2_VG_UUID"
	Lvm2VGTagsTag  = "LVM2_VG_TAGS"
	Lvm2PVCountTag = "LVM2_PV_COUNT"

	// pv tags
	Lvm2PVUuidTag = "LVM2_PV_UUID"
	Lvm2PVNameTag = "LVM2_PV_NAME"
	Lvm2PVSizeTag = "LVM2_PV_SIZE"
	Lvm2PVFreeTag = "LVM2_PV_FREE"
	Lvm2PVTagsTag = "LVM2_PV_TAGS"

	// EVENT
	EventCreateVGFailed = "CreateVGFailed"

	NsenterCmd = "/bin/nsenter --mount=/proc/1/ns/mnt --ipc=/proc/1/ns/ipc --net=/proc/1/ns/net --uts=/proc/1/ns/uts "
)

var (
	ValidProvisionerNames []string = []string{
		ProvisionerNameYoda,
		ProvisionerName,
	}
	ValidVolumeType []VolumeType = []VolumeType{
		VolumeTypeMountPoint,
		VolumeTypeLVM,
		VolumeTypeDevice,
		VolumeTypeQuota,
	}
	SupportedFS                    = []string{VolumeFSTypeExt3, VolumeFSTypeExt4, VolumeFSTypeXFS}
	SchedulerStrategy StrategyType = StrategyBinpack
)

type UpdateStatus string

var (
	UpdateStatusPending  UpdateStatus = "pending"
	UpdateStatusAccepted UpdateStatus = "accepted"
	UpdateStatusFailed   UpdateStatus = "failed"
)

type StorageSpecUpdateStatus struct {
	Status   UpdateStatus                          `json:"status"`
	Reason   string                                `json:"reason"`
	SpecHash uint64                                `json:"specHash"`
	NewSpec  nodelocalstorage.NodeLocalStorageSpec `json:"newSpec"`
}

func VolumeTypeFromString(s string) (VolumeType, error) {
	for _, v := range ValidVolumeType {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid Local Volume type: %q, valid values are %s", s, ValidVolumeType)
}

type NodeAntiAffinityWeight struct {
	weights map[VolumeType]int
}

func NewNodeAntiAffinityWeight() *NodeAntiAffinityWeight {
	return &NodeAntiAffinityWeight{}
}

func (w *NodeAntiAffinityWeight) Put(volumeType VolumeType, weight int) {
	if len(w.weights) <= 0 {
		w.weights = make(map[VolumeType]int)
	}
	w.weights[volumeType] = weight
}

func (w *NodeAntiAffinityWeight) Get(volumeType VolumeType) int {

	return w.weights[volumeType]
}

func (w *NodeAntiAffinityWeight) Items(copy bool) map[VolumeType]int {
	if !copy {
		return w.weights
	}
	result := make(map[VolumeType]int)
	for k, v := range w.weights {
		result[k] = v
	}
	return result
}
