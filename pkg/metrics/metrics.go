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
)
