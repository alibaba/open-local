package pkg

import (
	"fmt"
	"time"

	nodelocalstorage "github.com/oecp/open-local-storage-service/pkg/apis/storage/v1alpha1"
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

	AgentName              string = "openlss-agent"
	ProvisionerNameYoda    string = "yodaplugin.csi.alibabacloud.com"
	ProvisionerNameOpenLSS string = "openlss.csi.oecp.io"
	SchedulerName          string = "openlss-scheduler"

	KubernetesNodeIdentityKey = "kubernetes.io/hostname"
	//TODO(yuzhi.wx) need to confirm with jubao
	VolumeTypeKey    = "volumeType"
	VolumeFSTypeKey  = "fsType"
	VolumeMediaType  = "mediaType"
	VolumeFSTypeExt4 = "ext4"
	VolumeFSTypeExt3 = "ext3"
	VolumeFSTypeXFS  = "xfs"

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
	AnnoSnapshotReadonly                 = "storage.oecp.io/readonly"
	LabelReschduleTimestamp              = "pod.oecp.io/reschdule-timestamp"
	EnvExpandSnapInterval                = "Expand_Snapshot_Interval"
	TagSnapshot                          = "SnapshotName"
	TagSnapshotReadonly                  = "storage.oecp.io/readonly"
	PendingWithoutScheduledFieldSelector = "status.phase=Pending,spec.nodeName="
	TriggerPendingPodCycle               = time.Second * 300
)

var (
	ValidProvisionerNames []string = []string{
		ProvisionerNameYoda,
		ProvisionerNameOpenLSS,
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
	return "", fmt.Errorf("invalid LSS Volume type: %q, valid values are %s", s, ValidVolumeType)
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
	if copy == false {
		return w.weights
	}
	result := make(map[VolumeType]int)
	for k, v := range w.weights {
		result[k] = v
	}
	return result
}
