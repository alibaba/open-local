package scheduler

import (
	"github.com/oecp/open-local-storage-service/pkg"
)

// BindingInfo represents the pvc and mp/lvm/device mapping
type BindingInfo struct {
	// node is the name of selected node
	Node string `json:"node"`
	// path for mount point
	Disk string `json:"disk"`
	// VgName is the name of selected volume group
	VgName string `json:"vgName"`
	// Device is the name for raw block device: /dev/vdb
	Device string `json:"device"`
	// [lvm] or [disk] or [device] or [quota]
	VolumeType pkg.VolumeType `json:"volumeType"`

	// PersistentVolumeClaim is the metakey for pvc: {namespace}/{name}
	PersistentVolumeClaim string `json:"persistentVolumeClaim"`
}
