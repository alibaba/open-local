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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Cluster,shortName=nls
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.status.nodeStorageInfo.state.type`,name="State",type=string
// +kubebuilder:printcolumn:JSONPath=`.status.nodeStorageInfo.phase`,name="Phase",type=string
// +kubebuilder:printcolumn:JSONPath=`.status.nodeStorageInfo.state.lastHeartbeatTime`,name="AgentUpdateAt",type=date
// +kubebuilder:printcolumn:JSONPath=`.status.filteredStorageInfo.updateStatusInfo.lastUpdateTime`,name="SchedulerUpdateAt",type=date
// +kubebuilder:printcolumn:JSONPath=`.status.filteredStorageInfo.updateStatusInfo.updateStatus`,name="SchedulerUpdateStatus",type=string

// NodeLocalStorage is the Schema for the nodelocalstorages API
type NodeLocalStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeLocalStorageSpec   `json:"spec,omitempty"`
	Status NodeLocalStorageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// NodeLocalStorageList contains a list of NodeLocalStorage
type NodeLocalStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeLocalStorage `json:"items"`
}

// NodeLocalStorageSpec defines the desired state of NodeLocalStorage
type NodeLocalStorageSpec struct {
	// NodeName is the kube node name
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=1
	NodeName           string             `json:"nodeName,omitempty"`
	SpdkConfig         SpdkConfig         `json:"spdkConfig,omitempty"`
	ListConfig         ListConfig         `json:"listConfig,omitempty"`
	ResourceToBeInited ResourceToBeInited `json:"resourceToBeInited,omitempty"`
}

// NodeLocalStorageStatus defines the observed state of NodeLocalStorage
type NodeLocalStorageStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	NodeStorageInfo     NodeStorageInfo     `json:"nodeStorageInfo,omitempty"`
	FilteredStorageInfo FilteredStorageInfo `json:"filteredStorageInfo,omitempty"`
}

// SpdkConfig defines SPDK configuration
type SpdkConfig struct {
	// DeviceType is the type of SPDK block devices
	// +kubebuilder:validation:MaxLength=8
	// +kubebuilder:validation:MinLength=0
	DeviceType string `json:"deviceType,omitempty"`
	// RpcSocket is the unix domain socket for SPDK RPC
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=0
	RpcSocket string `json:"rpcSocket,omitempty"`
}

type ListConfig struct {
	// VGs defines the user specified VGs to be scheduled
	// only VGs specified here can be picked by scheduler
	VGs VGList `json:"vgs,omitempty"`
	// BlacklistMountPoints defines the user specified mount points which are not allowed for scheduling
	MountPoints MountPointList `json:"mountPoints,omitempty"`
	// Devices defines the user specified Devices to be scheduled,
	// only raw device specified here can be picked by scheduler
	Devices DeviceList `json:"devices,omitempty"`
}

type VGList struct {
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Include []string `json:"include,omitempty"`
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Exclude []string `json:"exclude,omitempty"`
}

type MountPointList struct {
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Include []string `json:"include,omitempty"`
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Exclude []string `json:"exclude,omitempty"`
}

type DeviceList struct {
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Include []string `json:"include,omitempty"`
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Exclude []string `json:"exclude,omitempty"`
}

type ResourceToBeInited struct {
	// VGs defines the user specified VGs,
	// which will be initialized by Filtered Agent
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	VGs []VGToBeInited `json:"vgs,omitempty"`
	// MountPoints defines the user specified mount points,
	// which will be initialized by Filtered Agent
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	MountPoints []MountPointToBeInited `json:"mountpoints,omitempty"`
}

type VGToBeInited struct {
	// Name is the name of volume group
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Device can be whole disk or disk partition
	// which will be initialized as Physical Volume
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:UniqueItems=false
	Devices []string `json:"devices"`
}

type MountPointToBeInited struct {
	// Path is the path of mount point
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	Path string `json:"path"`
	// Device is the device underlying the mount point
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	Device string `json:"device"`
	// FsType is filesystem type
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:MinLength=1
	FsType string `json:"fsType,omitempty"` /*ext4(default), xfs, ext3*/
	// Options is a list of mount options
	Options []string `json:"options,omitempty"`
}

// NodeStorageInfo is info of the full storage resources of the node,
// which is updated by Filtered Agent
type NodeStorageInfo struct {
	// DeviceInfos is the block device on node
	DeviceInfos []DeviceInfo `json:"deviceInfo,omitempty"`
	// VolumeGroups is LVM vgs
	VolumeGroups []VolumeGroup `json:"volumeGroups,omitempty"`
	// MountPoints is the list of mount points on node
	MountPoints []MountPoint `json:"mountPoints,omitempty"`
	// Phase is the current lifecycle phase of the node storage.
	// +optional
	Phase StoragePhase `json:"phase,omitempty"`
	// State is the last state of node local storage.
	// +optional
	State StorageState `json:"state,omitempty"`
}

type UpdateStatus string

var (
	UpdateStatusPending  UpdateStatus = "pending"
	UpdateStatusAccepted UpdateStatus = "accepted"
	UpdateStatusFailed   UpdateStatus = "failed"
)

type UpdateStatusInfo struct {
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	Status         UpdateStatus `json:"updateStatus,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

// FilteredStorageInfo is info of the storage resources of the node,
// which is picked by Filtered Scheduler according to ListConfig
type FilteredStorageInfo struct {
	// VolumeGroups is LVM vgs picked by Filtered according to WhitelistVGs/BlacklistVGs
	VolumeGroups []string `json:"volumeGroups,omitempty"`
	// MountPoints is mount points picked by Filtered according to WhitelistMountPoints/BlacklistMountPoints
	MountPoints []string `json:"mountPoints,omitempty"`
	// Devices is block devices picked by Filtered according to WhitelistDevices/BlacklistDevices
	Devices      []string         `json:"devices,omitempty"`
	UpdateStatus UpdateStatusInfo `json:"updateStatusInfo,omitempty"`
}

type StoragePhase string

// These are the valid phases of node.
const (
	// NodeStoragePending means the node has been created/added by the system, but not configured.
	NodeStoragePending StoragePhase = "Pending"
	// NodeStorageRunning means the node has been configured and has Kubernetes components running.
	NodeStorageRunning StoragePhase = "Running"
	// NodeStorageTerminated means the node has been removed from the cluster.
	NodeStorageTerminated StoragePhase = "Terminated"
)

// VolumeGroup is an alias for LVM VG
type VolumeGroup struct {
	// Name is the VG name
	Name string `json:"name"`
	// PhysicalVolumes are Unix block device nodes,
	PhysicalVolumes []string `json:"physicalVolumes"`
	// LogicalVolumes "Virtual/logical partition" that resides in a VG
	LogicalVolumes []LogicalVolume `json:"logicalVolumes,omitempty"`
	// Total is the VG size
	Total uint64 `json:"total"`
	// Available is the free size for VG
	Available uint64 `json:"available"`
	// Allocatable is the free size for Filtered
	Allocatable uint64 `json:"allocatable"`
	// Condition is the condition for Volume group
	Condition StorageConditionType `json:"condition,omitempty"`
}

// LogicalVolume is an alias for LVM LV
type LogicalVolume struct {
	// Name is the LV name
	Name string `json:"name"`
	// VGName is the VG name of this LV
	VGName string `json:"vgname"`
	// Size is the LV size
	Total uint64 `json:"total"`
	// ReadOnly indicates whether the LV is read-only
	ReadOnly bool `json:"readOnly,omitempty"`
	// Condition is the condition for LogicalVolume
	Condition StorageConditionType `json:"condition,omitempty"`
}

// MountPoint is the mount point on a node
type MountPoint struct {
	// Name is the mount point name
	Name string `json:"name,omitempty"` /*/mnt/testmount */
	// Total is the size of mount point
	Total uint64 `json:"total"`
	// Available is the free size for mount point
	Available uint64 `json:"available"`
	// FsType is filesystem type
	FsType string `json:"fsType,omitempty"` /*ext4, xfs, ext3*/
	// IsBind indicates whether the mount point is a bind
	IsBind bool `json:"isBind"`
	// Options is a list of mount options
	Options []string `json:"options,omitempty"` /*quota, ordered*/
	// Device is the device underlying the mount point
	Device string `json:"device,omitempty"` /*/dev/sda*/
	// ReadOnly indicates whether the mount point is read-only
	ReadOnly bool `json:"readOnly"`
	// Condition is the condition for mount point
	Condition StorageConditionType `json:"condition,omitempty"`
}

// DeviceInfos is a raw block device on host
type DeviceInfo struct {
	// Name is the block device name
	Name string `json:"name,omitempty"` /* /dev/sda*/
	// MediaType is the media type like ssd/hdd
	MediaType string `json:"mediaType,omitempty"` /*ssd,hdd*/
	// Total is the raw block device size
	Total uint64 `json:"total"` /**/
	// ReadOnly indicates whether the device is ready-only
	ReadOnly bool `json:"readOnly"`
	// Condition is the condition for mount point
	Condition StorageConditionType `json:"condition,omitempty"`
}

type StorageConditionType string

// These are valid conditions of node. Currently, we don't have enough information to decide
// node condition. In the future, we will add more. The proposed set of conditions are:
// StorageReady, NodeReachable
const (
	// NodeStorageReady means all disks are healthy and ready to service IO.
	StorageReady StorageConditionType = "DiskReady"
	// StorageFull means some disks (no disk fault)are full now
	// space on the node.
	StorageFull StorageConditionType = "DiskFull"

	// StorageFault means some disks are under disk failure
	StorageFault StorageConditionType = "DiskFault"
)

// The below types are used by kube_client and api_server.

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition;
// "ConditionFalse" means a resource is not in the condition; "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not. In the future, we could add other
// intermediate conditions, e.g. ConditionDegraded.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// StorageState stores the overall node disk state and detailed reason
type StorageState struct {
	Type   StorageConditionType `json:"type,omitempty"`
	Status ConditionStatus      `json:"status,omitempty"`
	// +optional
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// +optional
	Reason string `json:"reason,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
}
