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

package metrics

import (
	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Subsystem is prometheus subsystem name.
	Subsystem = "local"
)

var (
	VolumeGroupTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "volume_group_total",
			Help:      "Allocatable size of VG.",
		},
		[]string{"nodename", "vgname"},
	)
	VolumeGroupUsedByLocal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "volume_group_used",
			Help:      "Used size of VG.",
		},
		[]string{"nodename", "vgname"},
	)
	MountPointTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "mount_point_total",
			Help:      "Total size of MountPoint.",
		},
		[]string{"nodename", "name", "type"},
	)
	MountPointAvailable = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "mount_point_available",
			Help:      "Available size of MountPoint.",
		},
		[]string{"nodename", "name", "type"},
	)
	MountPointBind = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "mount_point_bind",
			Help:      "Is MountPoint Bind.",
		},
		[]string{"nodename", "name"},
	)
	DeviceTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "device_total",
			Help:      "Total size of Device.",
		},
		[]string{"nodename", "name", "type"},
	)
	DeviceAvailable = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "device_available",
			Help:      "Available size of Device.",
		},
		[]string{"nodename", "name", "type"},
	)
	DeviceBind = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "device_bind",
			Help:      "Is Device Bind.",
		},
		[]string{"nodename", "name"},
	)
	AllocatedNum = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "allocated_num",
			Help:      "allocated number.",
		},
		[]string{"nodename"},
	)
	// storage_name
	// LVM:         VG name
	// MountPoint:  mount path
	// Device:      device path
	LocalPV = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "local_pv",
			Help:      "local PV.",
		},
		[]string{"nodename", "pv_name", "pv_type", "pvc_name", "pvc_ns", "status", "storage_name"},
	)
	InlineVolume = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: Subsystem,
			Name:      "inline_volume",
			Help:      "pod inline volume.",
		},
		[]string{"pod_name", "pod_namespace", "nodename", "vgname", "volume_name"},
	)
)

func UpdateMetrics(c *cache.ClusterNodeCache) {
	// metrics reset
	AllocatedNum.Reset()
	DeviceAvailable.Reset()
	DeviceBind.Reset()
	DeviceTotal.Reset()
	MountPointAvailable.Reset()
	MountPointBind.Reset()
	MountPointTotal.Reset()
	VolumeGroupUsedByLocal.Reset()
	VolumeGroupTotal.Reset()
	LocalPV.Reset()
	InlineVolume.Reset()

	// metrics update
	for nodeName := range c.Nodes {
		cache := c.GetNodeCache(nodeName)
		for vgname, info := range cache.VGs {
			VolumeGroupTotal.WithLabelValues(nodeName, string(vgname)).Set(float64(info.Capacity))
			VolumeGroupUsedByLocal.WithLabelValues(nodeName, string(vgname)).Set(float64(info.Requested))
		}
		for mpname, info := range cache.MountPoints {
			MountPointTotal.WithLabelValues(nodeName, string(mpname), string(info.MediaType)).Set(float64(info.Capacity))
			MountPointAvailable.WithLabelValues(nodeName, string(mpname), string(info.MediaType)).Set(float64(info.Capacity))
			if info.IsAllocated {
				MountPointBind.WithLabelValues(nodeName, string(mpname)).Set(1)
			} else {
				MountPointBind.WithLabelValues(nodeName, string(mpname)).Set(0)
			}
		}
		for devicename, info := range cache.Devices {
			DeviceAvailable.WithLabelValues(nodeName, string(devicename), string(info.MediaType)).Set(float64(info.Capacity))
			DeviceTotal.WithLabelValues(nodeName, string(devicename), string(info.MediaType)).Set(float64(info.Capacity))
			if info.IsAllocated {
				DeviceBind.WithLabelValues(nodeName, string(devicename)).Set(1)
			} else {
				DeviceBind.WithLabelValues(nodeName, string(devicename)).Set(0)
			}
		}
		var pvType, storageName string
		for pvname, pv := range cache.LocalPVs {
			switch pv.Spec.CSI.VolumeAttributes[pkg.VolumeTypeKey] {
			case string(pkg.VolumeTypeLVM):
				pvType = string(pkg.VolumeTypeLVM)
				storageName = pv.Spec.CSI.VolumeAttributes[pkg.VGName]
			case string(pkg.VolumeTypeMountPoint):
				pvType = string(pkg.VolumeTypeMountPoint)
				storageName = pv.Spec.CSI.VolumeAttributes[pkg.MPName]
			case string(pkg.VolumeTypeDevice):
				pvType = string(pkg.VolumeTypeDevice)
				storageName = pv.Spec.CSI.VolumeAttributes[pkg.DeviceName]
			}
			LocalPV.WithLabelValues(
				nodeName,
				pvname,
				pvType,
				pv.Spec.CSI.VolumeAttributes[pkg.PVCName],
				pv.Spec.CSI.VolumeAttributes[pkg.PVCNameSpace],
				string(pv.Status.Phase),
				storageName).Set(float64(utils.GetPVStorageSize(&pv)))
		}

		for _, volumes := range cache.PodInlineVolumeInfo {
			for _, volume := range volumes {
				InlineVolume.WithLabelValues(
					volume.PodName,
					volume.PodNamespace,
					nodeName,
					volume.VgName,
					volume.VolumeName,
				).Set(float64(volume.VolumeSize))
			}
		}

		AllocatedNum.WithLabelValues(nodeName).Set(float64(cache.AllocatedNum))
	}
}
