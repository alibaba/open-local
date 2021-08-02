/*
Copyright 2021 OECP Authors.

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

package common

import (
	lssv1alpha1 "github.com/oecp/open-local/pkg/apis/storage/v1alpha1"
)

// Configuration stores all the user-defined parameters to the controller
type Configuration struct {
	// Nodename is the kube node name
	Nodename string
	// Config is the configfile path of open-local agent
	ConfigPath string
	// SysPath is the the mount point of the host sys path
	SysPath string
	// MountPath defines the specified mount path we discover
	MountPath string
	// DisconverInterval is the duration(second) that the agent checks at one time
	DiscoverInterval int
	// CRDSpec is spec of NodeLocalStorage CRD
	CRDSpec *lssv1alpha1.NodeLocalStorageSpec
	// CRDSpec is spec of NodeLocalStorage CRD
	CRDStatus *lssv1alpha1.NodeLocalStorageStatus
	// LogicalVolumeNamePrefix is the prefix of LogicalVolume Name
	LogicalVolumeNamePrefix string
	// RegExp is used to filter device names
	RegExp string
	// InitConfig is config for agent to create nodelocalstorage
	InitConfig string
}

const (
	// DefaultConfigPath is the default configfile path of open-local agent
	DefaultConfigPath string = "/etc/controller/config/"
	// DefaultInterval is the duration(second) that the agent checks at one time
	DefaultInterval int    = 60
	DefaultEndpoint string = "unix://tmp/csi.sock"
)
