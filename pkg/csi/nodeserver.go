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

package csi

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/utils"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	volume "github.com/kata-containers/kata-containers/src/runtime/pkg/direct-volume"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
	k8smount "k8s.io/utils/mount"
)

const (
	// VgNameTag is the vg name tag
	VgNameTag = "vgName"
	// VolumeTypeTag is the pv type tag
	VolumeTypeTag = "volumeType"
	// PvTypeTag is the pv type tag
	PvTypeTag = "pvType"
	// FsTypeTag is the fs type tag
	FsTypeTag = "fsType"
	// LvmTypeTag is the lvm type tag
	LvmTypeTag = "lvmType"
	// NodeAffinity is the pv node schedule tag
	NodeAffinity = "nodeAffinity"
	// DefaultFs default fs
	DefaultFs = "ext4"
	// DefaultNodeAffinity default NodeAffinity
	DefaultNodeAffinity = "true"
	// LinearType linear type
	LinearType = "linear"
	// DirectTag is direct-assigned volume tag
	DirectTag = "direct"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	nodeID               string
	driverName           string
	mounter              utils.Mounter
	client               kubernetes.Interface
	localclient          clientset.Interface
	k8smounter           k8smount.Interface
	sysPath              string
	ephemeralVolumeStore Store
	inFlight             *InFlight
	spdkSupported        bool
	spdkclient           *spdk.SpdkClient
}

var (
	masterURL  string
	kubeconfig string
)

func newNodeServer(d *csicommon.CSIDriver, dName, nodeID, sysPath string) csi.NodeServer {
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	localclient, err := clientset.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building local clientset: %s", err.Error())
	}

	mounter := k8smount.New("")

	store, err := NewVolumeStore(DefaultEphemeralVolumeDataFilePath)
	if err != nil {
		log.Fatalf("fail to initialize ephemeral volume store: %s", err.Error())
	}

	ns := &nodeServer{
		DefaultNodeServer:    csicommon.NewDefaultNodeServer(d),
		nodeID:               nodeID,
		mounter:              utils.NewMounter(),
		k8smounter:           mounter,
		client:               kubeClient,
		localclient:          localclient,
		driverName:           dName,
		sysPath:              sysPath,
		ephemeralVolumeStore: store,
		inFlight:             NewInFlight(),
		spdkSupported:        false,
	}

	go ns.checkSPDKSupport()

	return ns
}

func (ns *nodeServer) addDirectVolume(volumePath, device, fsType string) error {
	mountInfo := struct {
		VolumeType string            `json:"volume-type"`
		Device     string            `json:"device"`
		FsType     string            `json:"fstype"`
		Metadata   map[string]string `json:"metadata,omitempty"`
		Options    []string          `json:"options,omitempty"`
	}{
		VolumeType: "block",
		Device:     device,
		FsType:     fsType,
	}

	mi, err := json.Marshal(mountInfo)
	if err != nil {
		log.Error("addDirectVolume - json.Marshal failed: ", err.Error())
		return status.Errorf(codes.Internal, "json.Marshal failed: %s", err.Error())
	}

	if err := volume.Add(volumePath, string(mi)); err != nil {
		log.Error("addDirectVolume - add direct volume failed: ", err.Error())
		return status.Errorf(codes.Internal, "add direct volume failed: %s", err.Error())
	}

	log.Infof("add direct volume done: %s %s", volumePath, string(mi))
	return nil
}

func (ns *nodeServer) mountBlockDevice(device, targetPath string) error {
	mounted, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		log.Errorf("mountBlockDevice - check if %s is mounted failed: %s", targetPath, err.Error())
		return status.Errorf(codes.Internal, "check if %s is mounted failed: %s", targetPath, err.Error())
	}

	mountOptions := []string{"bind"}
	if mounted {
		log.Infof("Target path %s is already mounted", targetPath)
	} else {
		log.Infof("mounting %s at %s", device, targetPath)

		if err := ns.mounter.MountBlock(device, targetPath, mountOptions...); err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				log.Errorf("Remove mount target %s failed: %s", targetPath, removeErr.Error())
				return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", targetPath, removeErr)
			}
			log.Errorf("mount block %s at %s failed: %s", device, targetPath, err.Error())
			return status.Errorf(codes.Internal, "Could not mount block %s at %s: %s", device, targetPath, err.Error())
		}
	}

	return nil
}

func (ns *nodeServer) publishDirectVolume(ctx context.Context, req *csi.NodePublishVolumeRequest, volumeType string) (err error) {
	// in case publish volume failed, clean up the resources
	defer func() {
		if err != nil {
			log.Error("publishDirectVolume failed: ", err.Error())

			ephemeralDevice, exist := ns.ephemeralVolumeStore.GetDevice(req.VolumeId)
			if exist && ephemeralDevice != "" {
				if err := removeLVMByDevicePath(ephemeralDevice); err != nil {
					log.Errorf("fail to remove lvm device (%s): %s", ephemeralDevice, err.Error())
				}
			}

			if err := volume.Remove(req.GetTargetPath()); err != nil {
				log.Warn("NodePublishVolume - direct volume remove failed:", err.Error())
			}
		}
	}()

	device := ""
	if volumeType != LvmVolumeType {
		if value, ok := req.VolumeContext[DeviceVolumeType]; ok {
			device = value
		} else {
			log.Error("source device is empty")
			return status.Error(codes.Internal, "publishDirectVolume: source device is empty")
		}
	} else {
		var err error
		device, _, err = ns.createLV(ctx, req)
		if err != nil {
			log.Error("publishDirectVolume - create logical volume failed: ", err.Error())
			return status.Errorf(codes.Internal, "publishDirectVolume - create logical volume failed: %s", err.Error())
		}

		ephemeralVolume := req.GetVolumeContext()["csi.storage.k8s.io/ephemeral"] == "true"
		if ephemeralVolume {
			if err := ns.ephemeralVolumeStore.AddVolume(req.VolumeId, device); err != nil {
				log.Warningf("fail to add volume: %s", err.Error())
			}
		}
	}

	mount := false
	volCap := req.GetVolumeCapability()
	if volumeType == MountPointType {
		mount = true
	}

	if _, ok := volCap.GetAccessType().(*csi.VolumeCapability_Mount); ok {
		mount = true
	}

	if mount {
		fsType := volCap.GetMount().FsType
		if len(fsType) == 0 {
			fsType = DefaultFs
		}

		if err := ns.addDirectVolume(req.GetTargetPath(), device, fsType); err != nil {
			log.Error("addDirectVolume failed: ", err.Error())
			return status.Errorf(codes.Internal, "addDirectVolume failed: %s", err.Error())
		}

		if err := utils.FormatBlockDevice(device, fsType); err != nil {
			log.Error("FormatBlockDevice failed: ", err.Error())
			return status.Errorf(codes.Internal, "FormatBlockDevice failed: %s", err.Error())
		}
	} else {
		if err := ns.mountBlockDevice(device, req.GetTargetPath()); err != nil {
			log.Error("mountBlockDevice failed: ", err.Error())
			return status.Errorf(codes.Internal, "mountBlockDevice failed: %s", err.Error())
		}
	}

	return nil
}

func (ns *nodeServer) publishSpdkVolume(ctx context.Context, req *csi.NodePublishVolumeRequest, volumeType string) (err error) {
	// the spdk vhost-user-blk block device file
	device := ""
	bdevName := ""

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	// in case publish volume failed, clean up the resources
	defer func() {
		if err != nil {
			log.Error("publishSpdkVolume failed: ", err.Error())

			bdev, exist := ns.ephemeralVolumeStore.GetDevice(volumeID)
			if exist && bdev != "" {
				if err := ns.spdkclient.CleanBdev(bdev); err != nil {
					log.Error("NodePublishVolume - CleanBdev failed")
				}
			}

			if err := volume.Remove(targetPath); err != nil {
				log.Warn("NodePublishVolume - direct volume remove failed:", err.Error())
			}
		}
	}()

	// create block device
	if volumeType != LvmVolumeType {
		if sourceDevice, exists := req.VolumeContext[DeviceVolumeType]; exists {
			bdevName = "bdev-aio" + strings.Replace(sourceDevice, "/", "_", -1)
			if _, err := ns.spdkclient.CreateBdev(bdevName, sourceDevice); err != nil {
				return status.Errorf(codes.Internal, "create bdev failed: %s", err.Error())
			}

			device, _ = ns.spdkclient.FindVhostDevice(bdevName)
			if device == "" {
				var err error
				device, err = ns.spdkclient.CreateVhostDevice("ctrlr-"+uuid.New().String(), bdevName)
				if err != nil {
					_ = ns.spdkclient.DeleteBdev(bdevName)
					return status.Errorf(codes.Internal, "create vhost device failed: %s", err.Error())
				}
			}

			ephemeralVolume := req.GetVolumeContext()["csi.storage.k8s.io/ephemeral"] == "true"
			if ephemeralVolume {
				if err := ns.ephemeralVolumeStore.AddVolume(volumeID, bdevName); err != nil {
					if err := ns.spdkclient.CleanBdev(bdevName); err != nil {
						log.Error("NodePublishVolume - CleanBdev failed")
					}
					return status.Errorf(codes.Internal, "fail to add volume: %s", err.Error())
				}
			}
		}
	} else {
		var err error
		device, bdevName, err = ns.createLV(ctx, req)
		if err != nil {
			return status.Errorf(codes.Internal, "NodePublishVolume - create logical volume failed: %s", err.Error())
		}
	}

	mount := false
	volCap := req.GetVolumeCapability()
	if volumeType == MountPointType {
		mount = true
	}

	if _, ok := volCap.GetAccessType().(*csi.VolumeCapability_Mount); ok {
		mount = true
	}

	fsType := DefaultFs
	if mount {
		fsType = volCap.GetMount().FsType
		if len(fsType) == 0 {
			fsType = DefaultFs
		}

		if err := ns.addDirectVolume(targetPath, device, fsType); err != nil {
			return status.Errorf(codes.Internal, "addDirectVolume failed: %s", err.Error())
		}
	}

	if mount && bdevName != "" {
		// ensure the block device is formatted
		if !ns.spdkclient.MakeFs(bdevName, fsType) {
			return status.Error(codes.Internal, "MakeFs failed")
		}
	} else { // handle block volume
		// there isn't a real device attached to the device file. k8s doesn'
		// take this situation into consider, it maps the device as usual. device
		// mapping will failed. We create a small image file and attach it to a loop
		// device. the loop device will be replaced by vhost-user-blk device when
		// create container.
		device, err := spdk.GenerateDeviceTempBackingFile(device)
		if err != nil {
			return status.Errorf(codes.Internal, "GenerateDeviceTempBackingFile failed: %s", err.Error())
		}

		if err := ns.mountBlockDevice(device, targetPath); err != nil {
			return status.Errorf(codes.Internal, "mountBlockDevice failed: %s", err.Error())
		}
	}

	return nil
}

// volume_id: yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866
// staging_target_path: /var/lib/kubelet/plugins/kubernetes.io/csi/pv/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/globalmount
// target_path: /var/lib/kubelet/pods/2a7bbb9c-c915-4006-84d7-0e3ac9d8d70f/volumes/kubernetes.io~csi/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/mount
func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: Volume ID not provided")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.Internal, "NodePublishVolume: targetPath is empty")
	}
	log.Infof("NodePublishVolume: Start to mount volume %s to target path %s", volumeID, targetPath)

	volumeType := ""
	if _, ok := req.VolumeContext[VolumeTypeTag]; ok {
		volumeType = req.VolumeContext[VolumeTypeTag]
	}

	ephemeralVolume := req.GetVolumeContext()["csi.storage.k8s.io/ephemeral"] == "true"
	if ephemeralVolume {
		_, vgNameExist := req.VolumeContext[localtype.ParamVGName]
		if !vgNameExist {
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: must set vgName in volumeAttributes when creating ephemeral local volume")
		}
		volumeType = LvmVolumeType
	}

	if ok := ns.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		ns.inFlight.Delete(volumeID)
	}()

	volCap := req.GetVolumeCapability()

	// check if the volume is a direct-assigned volume, direct volume will be used as virtio-blk
	direct := false
	if val, ok := req.VolumeContext[DirectTag]; ok {
		var err error
		direct, err = strconv.ParseBool(val)
		if err != nil {
			direct = false
		}
	}

	if ns.spdkSupported || direct {
		if volumeType == MountPointType {
			log.Error("The volume type should not be MountPointType")
			return nil, status.Errorf(codes.InvalidArgument, "The volume type should not be MountPointType")
		}

		var err error
		if ns.spdkSupported {
			err = ns.publishSpdkVolume(ctx, req, volumeType)
		} else {
			err = ns.publishDirectVolume(ctx, req, volumeType)
		}

		if err != nil {
			log.Errorf("NodePublishVolume(%s to %s) failed: %s", volumeID, targetPath, err.Error())
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: volume %s with error: %s", volumeID, err.Error())
		} else {
			log.Infof("NodePublishVolume: Successful add local volume %s to %s", volumeID, targetPath)
			return &csi.NodePublishVolumeResponse{}, nil
		}
	}

	switch volumeType {
	case LvmVolumeType:
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountLvmBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmBlock): mount lvm volume %s with path %s with error: %s", volumeID, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountLvmFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmFS): mount lvm volume %s with path %s with error: %s", volumeID, targetPath, err.Error())
			}
		}
		if err := ns.setIOThrottling(ctx, req); err != nil {
			return nil, err
		}
	case MountPointType:
		err := ns.mountMountPointVolume(ctx, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: mount mountpoint volume %s with path %s with error: %s", volumeID, targetPath, err.Error())
		}
	case DeviceVolumeType:
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountDeviceVolumeBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(Block): mount device volume %s with path %s with error: %s", volumeID, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountDeviceVolumeFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(FileSystem): mount device volume %s with path %s with error: %s", volumeID, targetPath, err.Error())
			}
		}
	default:
		return nil, status.Errorf(codes.Internal, "NodePublishVolume: unsupported volume %s with type %s", volumeID, volumeType)
	}

	log.Infof("NodePublishVolume: Successful mount local volume %s to %s", volumeID, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume: Volume ID not provided")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.Internal, "NodeUnpublishVolume: targetPath is empty")
	}
	log.Infof("NodeUnpublishVolume: Start to unmount target path %s for volume %s", targetPath, volumeID)

	if ok := ns.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		ns.inFlight.Delete(volumeID)
	}()

	if err := volume.Remove(targetPath); err != nil {
		log.Warn("NodeUnpublishVolume - direct volume remove failed:", err.Error())
	}

	isMnt, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume: fail to check if targetPath %s is mounted", targetPath)
	}
	if isMnt {
		err := ns.mounter.Unmount(targetPath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume: Umount volume %s for path %s with error %v", volumeID, targetPath, err)
		}
	}

	ephemeralDevice, exist := ns.ephemeralVolumeStore.GetDevice(volumeID)
	if exist && ephemeralDevice != "" {
		// /dev/mapper/yoda--pool0-yoda--5c523416--7288--4138--95e0--f9392995959f
		if ns.spdkSupported {
			err = ns.spdkclient.CleanBdev(ephemeralDevice)
		} else {
			err = removeLVMByDevicePath(ephemeralDevice)
		}

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		err = ns.ephemeralVolumeStore.DeleteVolume(volumeID)
		if err != nil {
			log.Warningf("failed to remove volume: %s", err.Error())
		}
	}

	log.Infof("NodeUnpublishVolume: Successful umount target path %s for volume %s", targetPath, volumeID)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (
	*csi.NodeExpandVolumeResponse, error) {
	log.Debugf("NodeExpandVolume: local node expand volume with: %v", req)
	volumeID := req.VolumeId
	targetPath := req.VolumePath
	expectSize := req.CapacityRange.RequiredBytes
	if !ns.spdkSupported {
		if err := ns.resizeVolume(ctx, volumeID, targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: Resize local volume %s with error: %s", volumeID, err.Error())
		}
	} else {
		//TODO: call kata-runtime to resize the volume
		//Resize(targetPath, expectSize)
		log.Warn("Warning: direct volume resize is to be implemented")
	}

	log.Infof("NodeExpandVolume: Successful expand local volume: %v to %d", req.VolumeId, expectSize)
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *nodeServer) GetNodeID() string {
	return ns.nodeID
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// currently there is a single NodeServer capability according to the spec
	nscap := &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
			},
		},
	}
	nscap2 := &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			},
		},
	}
	nscap3 := &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
			},
		},
	}

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			nscap, nscap2, nscap3,
		},
	}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
		// make sure that the driver works on this particular node only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				TopologyNodeKey: ns.nodeID,
			},
		},
	}, nil
}

// NodeGetVolumeStats used for csi metrics
func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	var err error
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		err = fmt.Errorf("NodeGetVolumeStats target local path %v is empty", targetPath)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return utils.GetMetrics(targetPath)
}

func (ns *nodeServer) resizeVolume(ctx context.Context, volumeID, targetPath string) error {
	vgName := ""

	// Get volumeType
	volumeType := LvmVolumeType
	_, _, pv := getPvInfo(ns.client, volumeID)
	if pv != nil && pv.Spec.CSI != nil {
		if value, ok := pv.Spec.CSI.VolumeAttributes["volumeType"]; ok {
			volumeType = value
		}
	} else {
		return status.Errorf(codes.Internal, "resizeVolume: local volume get pv info error %s", volumeID)
	}

	switch volumeType {
	case LvmVolumeType:
		// Get lvm info
		if value, ok := pv.Spec.CSI.VolumeAttributes["vgName"]; ok {
			vgName = value
		}
		if vgName == "" {
			return status.Errorf(codes.Internal, "resizeVolume: Volume %s with vgname empty", pv.Name)
		}

		devicePath := filepath.Join("/dev", vgName, volumeID)

		log.Infof("NodeExpandVolume:: volumeId: %s, devicePath: %s", volumeID, devicePath)

		// use resizer to expand volume filesystem
		resizer := mountutils.NewResizeFs(utilexec.New())
		ok, err := resizer.Resize(devicePath, targetPath)
		if err != nil {
			return fmt.Errorf("NodeExpandVolume: Lvm Resize Error, volumeId: %s, devicePath: %s, volumePath: %s, err: %s", volumeID, devicePath, targetPath, err.Error())
		}
		if !ok {
			return status.Errorf(codes.Internal, "NodeExpandVolume:: Lvm Resize failed, volumeId: %s, devicePath: %s, volumePath: %s", volumeID, devicePath, targetPath)
		}
		log.Infof("NodeExpandVolume:: lvm resizefs successful volumeId: %s, devicePath: %s, volumePath: %s", volumeID, devicePath, targetPath)
		return nil
	}
	return nil
}

// get pvSize, pvSizeUnit, pvObject
func getPvInfo(client kubernetes.Interface, volumeID string) (int64, string, *v1.PersistentVolume) {
	pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Errorf("getPvInfo: fail to get pv, err: %v", err)
		}
		return 0, "", nil
	}
	pvQuantity := pv.Spec.Capacity["storage"]
	pvSize := pvQuantity.Value()
	//pvSizeGB := pvSize / (1024 * 1024 * 1024)

	//if pvSizeGB == 0 {
	pvSizeMB := pvSize / (1024 * 1024)
	return pvSizeMB, "m", pv
	//}
	//return pvSizeGB, "g", pv
}

func (ns *nodeServer) setIOThrottling(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	volCap := req.GetVolumeCapability()
	targetPath := req.GetTargetPath()
	volumeID := req.VolumeId
	iops, iopsExist := req.VolumeContext[localtype.VolumeIOPS]
	bps, bpsExist := req.VolumeContext[localtype.VolumeBPS]
	if iopsExist || bpsExist {
		// get pod
		var pod v1.Pod
		var podUID string
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			// /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/yoda-c018ff81-d346-452e-b7b8-a45f1d1c230e/76cf946e-d074-4455-a272-4d3a81264fab
			podUID = strings.Split(targetPath, "/")[10]
		case *csi.VolumeCapability_Mount:
			// /var/lib/kubelet/pods/2a7bbb9c-c915-4006-84d7-0e3ac9d8d70f/volumes/kubernetes.io~csi/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/mount
			podUID = strings.Split(targetPath, "/")[5]
		}
		log.Debugf("pod(volume id %s) uuid is %s", volumeID, podUID)
		namespace := req.VolumeContext[localtype.PVCNameSpace]
		// set ResourceVersion to 0
		// https://arthurchiao.art/blog/k8s-reliability-list-data-zh/
		pods, err := ns.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{ResourceVersion: "0"})
		for _, podItem := range pods.Items {
			if podItem.UID == types.UID(podUID) {
				pod = podItem
			}
		}
		if err != nil {
			return status.Errorf(codes.Internal, "NodePublishVolume: failed to get pod(uuid: %s): %s", podUID, err.Error())
		}
		// pod qosClass and blkioPath
		qosClass := pod.Status.QOSClass
		blkioPath := path.Join(fmt.Sprintf("%s/fs/cgroup/blkio/%s", ns.sysPath, utils.CgroupPathFormatter.ParentDir), utils.CgroupPathFormatter.QOSDirFn(qosClass), utils.CgroupPathFormatter.PodDirFn(qosClass, podUID))
		log.Debugf("pod(volume id %s) qosClass: %s", volumeID, qosClass)
		log.Debugf("pod(volume id %s) blkio path: %s", volumeID, blkioPath)
		// get lv lvpath
		// todo: not support device kind
		lvpath, _, err := ns.createLV(ctx, req)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to get lv path %s: %s", volumeID, err.Error())
		}
		stat := syscall.Stat_t{}
		_ = syscall.Stat(lvpath, &stat)
		maj := uint64(stat.Rdev / 256)
		min := uint64(stat.Rdev % 256)
		log.Debugf("volume %s maj:min: %d:%d", volumeID, maj, min)
		log.Debugf("volume %s path: %s", volumeID, lvpath)
		if iopsExist {
			log.Debugf("volume %s iops: %s", volumeID, iops)
			cmdstr := fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, iops), fmt.Sprintf("%s%s", blkioPath, localtype.IOPSReadFile))
			_, err := exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s%s", blkioPath, localtype.IOPSReadFile), err.Error())
			}
			cmdstr = fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, iops), fmt.Sprintf("%s%s", blkioPath, localtype.IOPSWriteFile))
			_, err = exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s%s", blkioPath, localtype.IOPSWriteFile), err.Error())
			}
		}
		if bpsExist {
			log.Debugf("volume %s bps: %s", volumeID, bps)
			cmdstr := fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, bps), fmt.Sprintf("%s%s", blkioPath, localtype.BPSReadFile))
			_, err := exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s%s", blkioPath, localtype.BPSReadFile), err.Error())
			}
			cmdstr = fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, bps), fmt.Sprintf("%s%s", blkioPath, localtype.BPSWriteFile))
			_, err = exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s%s", blkioPath, localtype.BPSWriteFile), err.Error())
			}
		}
	}
	return nil
}
func (ns *nodeServer) checkSPDKSupport() {
	for {
		nls, err := ns.localclient.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), ns.nodeID, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("node local storage %s not found, waiting for the controller to create the resource", ns.nodeID)
			} else {
				log.Errorf("get NodeLocalStorages failed: %s", err.Error())
			}
		} else {
			if nls.Spec.SpdkConfig.DeviceType != "" {
				if ns.spdkclient == nil {
					if ns.spdkclient = spdk.NewSpdkClient(nls.Spec.SpdkConfig.RpcSocket); ns.spdkclient != nil {
						ns.spdkSupported = true
						break
					}
				}
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}
