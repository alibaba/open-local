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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/open-local/pkg"
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	volume "github.com/kata-containers/kata-containers/src/runtime/pkg/direct-volume"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	log "k8s.io/klog/v2"
	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type nodeServer struct {
	k8smounter           mountutils.SafeFormatAndMount
	ephemeralVolumeStore Store
	inFlight             *InFlight
	spdkSupported        bool
	spdkclient           *spdk.SpdkClient

	options *driverOptions
}

func newNodeServer(options *driverOptions) *nodeServer {
	store, err := NewVolumeStore(DefaultEphemeralVolumeDataFilePath)
	if err != nil {
		log.Fatalf("fail to initialize ephemeral volume store: %s", err.Error())
	}

	ns := &nodeServer{
		k8smounter: mountutils.SafeFormatAndMount{
			Interface: mountutils.New(""),
			Exec:      utilexec.New(),
		},
		ephemeralVolumeStore: store,
		inFlight:             NewInFlight(),
		spdkSupported:        false,
		options:              options,
	}

	go ns.checkSPDKSupport()

	return ns
}

// volume_id: yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866
// staging_target_path: /var/lib/kubelet/plugins/kubernetes.io/csi/pv/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/globalmount
// target_path: /var/lib/kubelet/pods/2a7bbb9c-c915-4006-84d7-0e3ac9d8d70f/volumes/kubernetes.io~csi/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/mount
func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.V(4).Infof("NodePublishVolume: called with args %+v", *req)
	// Step 1: check
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: Volume ID not provided")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: targetPath is empty")
	}
	log.Infof("NodePublishVolume: start to mount volume %s to target path %s", volumeID, targetPath)

	// Step 2: get volumeType
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
		volumeType = string(pkg.VolumeTypeLVM)
	}

	// check if the volume is a direct-assigned volume, direct volume will be used as virtio-blk
	direct := false
	if val, ok := req.VolumeContext[DirectTag]; ok {
		var err error
		direct, err = strconv.ParseBool(val)
		if err != nil {
			direct = false
		}
	}

	// Step 3: spdk or direct
	if ok := ns.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		ns.inFlight.Delete(volumeID)
	}()
	if ns.spdkSupported || direct {
		if volumeType == string(pkg.VolumeTypeMountPoint) || volumeType == string(pkg.VolumeTypeDevice) {
			return nil, status.Errorf(codes.InvalidArgument, "The volume type should not be %s or %s", string(pkg.VolumeTypeMountPoint), string(pkg.VolumeTypeDevice))
		}

		var err error
		if ns.spdkSupported {
			err = ns.publishSpdkVolume(ctx, req, volumeType)
		} else {
			err = ns.publishDirectVolume(ctx, req, volumeType)
		}

		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: volume %s with error: %s", volumeID, err.Error())
		}
		log.Infof("NodePublishVolume: add local volume %s to %s successfully", volumeID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Step 4: switch
	volCap := req.GetVolumeCapability()
	switch volumeType {
	case string(pkg.VolumeTypeLVM):
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountLvmBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmBlock): fail to mount lvm volume %s with path %s: %s", volumeID, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountLvmFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmFS): fail to mount lvm volume %s with path %s: %s", volumeID, targetPath, err.Error())
			}
		}
		if err := ns.setIOThrottling(ctx, req); err != nil {
			return nil, err
		}
	case string(pkg.VolumeTypeMountPoint):
		err := ns.mountMountPointVolume(ctx, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: fail to mount mountpoint volume %s with path %s: %s", volumeID, targetPath, err.Error())
		}
	case string(pkg.VolumeTypeDevice):
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountDeviceVolumeBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(Block): fail to mount device volume %s with path %s: %s", volumeID, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountDeviceVolumeFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(FileSystem): fail to mount device volume %s with path %s: %s", volumeID, targetPath, err.Error())
			}
		}
	default:
		return nil, status.Errorf(codes.Internal, "NodePublishVolume: unsupported volume %s with type %s", volumeID, volumeType)
	}

	log.Infof("NodePublishVolume: mount local volume %s to %s successfully", volumeID, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.V(4).Infof("NodeUnpublishVolume: called with args %+v", *req)
	// Step 1: check
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume: Volume ID not provided")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.Internal, "NodeUnpublishVolume: targetPath is empty")
	}
	log.Infof("NodeUnpublishVolume: start to umount target path %s for volume %s", targetPath, volumeID)

	// Step 2: umount
	if ok := ns.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		ns.inFlight.Delete(volumeID)
	}()
	if ns.spdkSupported {
		if err := volume.Remove(targetPath); err != nil {
			log.Errorf("NodeUnpublishVolume: direct volume remove failed: %s", err.Error())
		}
	}

	if err := mountutils.CleanupMountPoint(targetPath, ns.k8smounter, true /*extensiveMountPointCheck*/); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume: fail to umount volume %s for path %s: %s", volumeID, targetPath, err.Error())
	}

	// Step 3: delete ephemeral device
	var err error
	ephemeralDevice, exist := ns.ephemeralVolumeStore.GetDevice(volumeID)
	if exist && ephemeralDevice != "" {
		// /dev/mapper/yoda--pool0-yoda--5c523416--7288--4138--95e0--f9392995959f
		if ns.spdkSupported {
			err = ns.spdkclient.CleanBdev(ephemeralDevice)
		} else {
			err = removeLVMByDevicePath(ephemeralDevice)
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume: fail to remove ephemeral volume: %s", err.Error())
		}

		err = ns.ephemeralVolumeStore.DeleteVolume(volumeID)
		if err != nil {
			log.Errorf("NodeUnpublishVolume: failed to remove volume in volume store: %s", err.Error())
		}
	}

	log.Infof("NodeUnpublishVolume: umount target path %s for volume %s successfully", targetPath, volumeID)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log.V(4).Infof("NodeStageVolume: called with args %+v", *req)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log.V(4).Infof("NodeUnstageVolume: called with args %+v", *req)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (
	*csi.NodeExpandVolumeResponse, error) {
	log.V(4).Infof("NodeExpandVolume: called with args %+v", *req)
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
		log.Warning("Warning: direct volume resize is to be implemented")
	}

	log.Infof("NodeExpandVolume: Successful expand local volume: %v to %d", req.VolumeId, expectSize)
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log.V(4).Infof("NodeGetCapabilities: called with args %+v", *req)
	return &csi.NodeGetCapabilitiesResponse{Capabilities: NodeCaps}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	log.V(4).Infof("NodeGetInfo: called with args %+v", *req)
	return &csi.NodeGetInfoResponse{
		NodeId: ns.options.nodeID,
		// make sure that the driver works on this particular node only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				pkg.KubernetesNodeIdentityKey: ns.options.nodeID,
			},
		},
	}, nil
}

// NodeGetVolumeStats used for csi metrics
func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log.V(4).Infof("NodeGetVolumeStats: called with args %+v", *req)
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodeGetVolumeStats target local path %v is empty", targetPath)
	}

	return utils.GetMetrics(targetPath)
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
	notMounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		log.Errorf("mountBlockDevice - check if %s is mounted failed: %s", targetPath, err.Error())
		return status.Errorf(codes.Internal, "check if %s is mounted failed: %s", targetPath, err.Error())
	}

	mountOptions := []string{"bind"}
	if !notMounted {
		log.Infof("Target path %s is already mounted", targetPath)
	} else {
		log.Infof("mounting %s at %s", device, targetPath)

		if err := utils.MountBlock(device, targetPath, mountOptions...); err != nil {
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
				log.Errorf("NodePublishVolume - direct volume remove failed: %s", err.Error())
			}
		}
	}()

	device := ""
	if volumeType != string(pkg.VolumeTypeLVM) {
		if value, ok := req.VolumeContext[pkg.VolumeTypeKey]; ok {
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
				log.Errorf("fail to add volume: %s", err.Error())
			}
		}
	}

	mount := false
	volCap := req.GetVolumeCapability()
	if volumeType == string(pkg.VolumeTypeMountPoint) {
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
				log.Warningf("NodePublishVolume - direct volume remove failed: %s", err.Error())
			}
		}
	}()

	// create block device
	if volumeType != string(pkg.VolumeTypeLVM) {
		if sourceDevice, exists := req.VolumeContext[string(pkg.VolumeTypeDevice)]; exists {
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
	if volumeType == string(pkg.VolumeTypeMountPoint) {
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

func (ns *nodeServer) resizeVolume(ctx context.Context, volumeID, targetPath string) error {
	vgName := ""

	// Get volumeType
	volumeType := string(pkg.VolumeTypeLVM)
	_, _, pv, err := getPvInfo(ns.options.kubeclient, volumeID)
	if err != nil {
		return err
	}
	if pv != nil && pv.Spec.CSI != nil {
		if value, ok := pv.Spec.CSI.VolumeAttributes["volumeType"]; ok {
			volumeType = value
		}
	} else {
		return status.Errorf(codes.Internal, "resizeVolume: local volume get pv info error %s", volumeID)
	}

	switch volumeType {
	case string(pkg.VolumeTypeLVM):
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
		log.Infof("pod(volume id %s) uuid is %s", volumeID, podUID)
		namespace := req.VolumeContext[localtype.PVCNameSpace]
		// set ResourceVersion to 0
		// https://arthurchiao.art/blog/k8s-reliability-list-data-zh/
		pods, err := ns.options.kubeclient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{ResourceVersion: "0"})
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
		blkioPath := fmt.Sprintf("%s/fs/cgroup/blkio/%s%s%s", ns.options.sysPath, utils.CgroupPathFormatter.ParentDir, utils.CgroupPathFormatter.QOSDirFn(qosClass), utils.CgroupPathFormatter.PodDirFn(qosClass, podUID))
		log.Infof("pod(volume id %s) qosClass: %s", volumeID, qosClass)
		log.Infof("pod(volume id %s) blkio path: %s", volumeID, blkioPath)
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
		log.Infof("volume %s maj:min: %d:%d", volumeID, maj, min)
		log.Infof("volume %s path: %s", volumeID, lvpath)
		if iopsExist {
			log.Infof("volume %s iops: %s", volumeID, iops)
			cmdstr := fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, iops), fmt.Sprintf("%s/%s", blkioPath, localtype.IOPSReadFile))
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
			log.Infof("volume %s bps: %s", volumeID, bps)
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
		nls, err := ns.options.localclient.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), ns.options.nodeID, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("node local storage %s not found, waiting for the controller to create the resource", ns.options.nodeID)
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
