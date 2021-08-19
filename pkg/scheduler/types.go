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

package scheduler

import (
	"github.com/alibaba/open-local/pkg"
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
