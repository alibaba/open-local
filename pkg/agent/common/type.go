package common

import (
	lssv1alpha1 "github.com/oecp/open-local-storage-service/pkg/apis/storage/v1alpha1"
)

// Configuration stores all the user-defined parameters to the controller
type Configuration struct {
	// Nodename is the kube node name
	Nodename string
	// Config is the configfile path of open-local-storage-service agent
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
	// DefaultConfigPath is the default configfile path of open-local-storage-service agent
	DefaultConfigPath string = "/etc/controller/config/"
	// DefaultInterval is the duration(second) that the agent checks at one time
	DefaultInterval int = 60
)
