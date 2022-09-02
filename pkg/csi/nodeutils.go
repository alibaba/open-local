package csi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alibaba/open-local/pkg"
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/server"
	"github.com/alibaba/open-local/pkg/utils"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	log "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	k8smount "k8s.io/utils/mount"
)

func (ns *nodeServer) createLV(ctx context.Context, req *csi.NodePublishVolumeRequest) (string, string, error) {
	// parse vgname, consider invalid if empty
	vgName := ""
	if _, ok := req.VolumeContext[VgNameTag]; ok {
		vgName = req.VolumeContext[VgNameTag]
	}
	if vgName == "" {
		log.Errorf("createLV: request with empty vgName in volume: %s", req.VolumeId)
		return "", "", status.Error(codes.Internal, "error with input vgName is empty")
	}

	// parse lvm type
	lvmType := LinearType
	if _, ok := req.VolumeContext[LvmTypeTag]; ok {
		lvmType = req.VolumeContext[LvmTypeTag]
	}

	log.Infof("createLV: vg %s, volume %s, LVM Type %s", vgName, req.GetVolumeId(), lvmType)

	volumeID := req.GetVolumeId()
	var isSnapshot bool
	if _, isSnapshot = req.VolumeContext[localtype.ParamSnapshotName]; isSnapshot {
		if ro, exist := req.VolumeContext[localtype.ParamSnapshotReadonly]; exist && ro == "true" {
			// if volume is ro snapshot, then mount snapshot lv
			log.Infof("createLV: volume %s is readonly snapshot, mount snapshot lv %s directly", volumeID, req.VolumeContext[localtype.ParamSnapshotName])
			volumeID = req.VolumeContext[localtype.ParamSnapshotName]
		} else {
			return "", "", status.Errorf(codes.Unimplemented, "createLV: support ro snapshot only, please set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
		}
	}
	devicePath := filepath.Join("/dev/", vgName, volumeID)
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		newDev, bdevName, err := ns.createVolume(req.VolumeContext, volumeID, vgName, lvmType)
		if err != nil {
			log.Errorf("createLV: create volume %s with error: %s", volumeID, err.Error())
			return "", "", status.Error(codes.Internal, err.Error())
		}

		if ns.spdkSupported {
			return newDev, bdevName, nil
		} else {
			return devicePath, "", nil
		}
	}

	return devicePath, "", nil
}

// include normal lvm & aep lvm type
func (ns *nodeServer) mountLvmFS(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// target path
	targetPath := req.TargetPath
	// fs type
	fsType := req.GetVolumeCapability().GetMount().FsType
	if len(fsType) == 0 {
		fsType = DefaultFs
	}
	// device path
	devicePath, _, err := ns.createLV(ctx, req)
	if err != nil {
		return err
	}

	var isSnapshotReadOnly bool = false
	if _, isSnapshot := req.VolumeContext[localtype.ParamSnapshotName]; isSnapshot {
		if ro, exist := req.VolumeContext[localtype.ParamSnapshotReadonly]; exist && ro == "true" {
			// if volume is ro snapshot, then mount snapshot lv
			isSnapshotReadOnly = true
		} else {
			return status.Errorf(codes.Unimplemented, "mountLvmFS: support ro snapshot only, please set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
		}
	}

	// Check targetPath
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0750); err != nil {
			log.Errorf("mountLvmFS: mkdir target path %s with error: %s", targetPath, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
	}

	isMnt := utils.IsMounted(targetPath)
	if !isMnt {
		var options []string
		if req.GetReadonly() || isSnapshotReadOnly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
		options = append(options, mountFlags...)

		diskMounter := &k8smount.SafeFormatAndMount{Interface: ns.k8smounter, Exec: utilexec.New()}
		if err := diskMounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
			log.Errorf("mountLvmFS: Volume: %s, Device: %s, FormatAndMount error: %s", req.VolumeId, devicePath, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		log.Infof("mountLvmFS:: mount successful devicePath: %s, targetPath: %s, options: %v", devicePath, targetPath, options)
	}
	ephemeralVolume := req.GetVolumeContext()["csi.storage.k8s.io/ephemeral"] == "true"
	if ephemeralVolume {
		if err := ns.ephemeralVolumeStore.AddVolume(req.VolumeId, devicePath); err != nil {
			log.Warningf("fail to add volume: %s", err.Error())
		}
	}
	return nil
}

func (ns *nodeServer) mountLvmBlock(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// target path
	targetPath := req.TargetPath
	// device path
	devicePath, _, err := ns.createLV(ctx, req)
	if err != nil {
		return err
	}

	// check if devicePath is block device
	var isBlock bool
	if isBlock, err = IsBlockDevice(devicePath); err != nil {
		if removeErr := os.Remove(targetPath); removeErr != nil {
			return status.Errorf(codes.Internal, "mountLvmBlock: Could not remove mount target %q: %v", targetPath, removeErr)
		}
		return status.Errorf(codes.Internal, "mountLvmBlock: check block device %s failed: %s", devicePath, err)
	}
	if !isBlock {
		return status.Errorf(codes.InvalidArgument, "mountLvmBlock: %s is not block device", devicePath)
	}

	// checking if the target file is already mounted with a device.
	mounted, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		return status.Errorf(codes.Internal, "mountLvmBlock: check if %s is mountpoint failed: %s", targetPath, err)
	}

	// mount device
	mountOptions := []string{"bind"}
	// if req.GetReadonly() {
	// 	mountOptions = append(mountOptions, "ro")
	// }
	if !mounted {
		log.Infof("mountLvmBlock: mounting %s at %s", devicePath, targetPath)
		if err := ns.mounter.EnsureBlock(targetPath); err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountLvmBlock: Could not remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountLvmBlock: ensure block %s failed: %s", targetPath, err.Error())
		}
		if err := ns.mounter.MountBlock(devicePath, targetPath, mountOptions...); err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountLvmBlock: Could not remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountLvmBlock: Could not mount block %s at %s: %s", devicePath, targetPath, err)
		}
	} else {
		log.Infof("mountLvmBlock: Target path %s is already mounted", targetPath)
	}

	return nil
}

func (ns *nodeServer) mountMountPointVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourcePath := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[string(pkg.VolumeTypeMountPoint)]; ok {
		sourcePath = value
	}
	if sourcePath == "" {
		log.Errorf("mountLocalVolume: volume: %s, sourcePath empty", req.VolumeId)
		return status.Error(codes.Internal, "Mount LocalVolume with empty source path "+req.VolumeId)
	}

	notmounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				log.Errorf("NodePublishVolume: volume %s mkdir target path %s with error: %s", req.VolumeId, targetPath, err.Error())
				return status.Error(codes.Internal, err.Error())
			}
		} else {
			log.Errorf("mountLocalVolume: check volume: %s mounted with error %v", req.VolumeId, err)
			return status.Error(codes.Internal, err.Error())
		}
	}
	if !notmounted {
		log.Infof("NodePublishVolume: VolumeId: %s, Local Path %s is already mounted", req.VolumeId, targetPath)
		return nil
	}

	// start to mount
	mnt := req.VolumeCapability.GetMount()
	options := append(mnt.MountFlags, "bind")
	if req.Readonly {
		options = append(options, "ro")
	}
	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}
	log.Infof("NodePublishVolume: Starting mount local volume %s with flags %v and fsType %s", req.VolumeId, options, fsType)
	if err = ns.k8smounter.Mount(sourcePath, targetPath, fsType, options); err != nil {
		log.Errorf("mountLocalVolume: Mount volume: %s with error %v", req.VolumeId, err)
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}

func (ns *nodeServer) mountDeviceVolumeFS(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourceDevice := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[string(pkg.VolumeTypeDevice)]; ok {
		sourceDevice = value
	}
	if sourceDevice == "" {
		log.Errorf("mountDeviceVolume: device volume: %s, sourcePath empty", req.VolumeId)
		return status.Error(codes.Internal, "Mount Device with empty source path "+req.VolumeId)
	}

	// Step Start to format
	// fs type
	fsType := req.GetVolumeCapability().GetMount().FsType
	if len(fsType) == 0 {
		fsType = DefaultFs
	}

	// Check targetPath
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0750); err != nil {
			log.Errorf("mountDeviceVolume: volume %s mkdir target path %s with error: %s", sourceDevice, targetPath, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
	}

	isMnt := utils.IsMounted(targetPath)
	if !isMnt {
		var options []string
		if req.GetReadonly() {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
		options = append(options, mountFlags...)
		// do format-mount or mount
		diskMounter := &k8smount.SafeFormatAndMount{Interface: ns.k8smounter, Exec: utilexec.New()}
		if err := diskMounter.FormatAndMount(sourceDevice, targetPath, fsType, options); err != nil {
			log.Errorf("mountDeviceVolume: Volume: %s, Device: %s, FormatAndMount error: %s", req.VolumeId, sourceDevice, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		log.Infof("mountDeviceVolume:: mount successful sourceDevice: %s, targetPath: %s, options: %v", sourceDevice, targetPath, options)
	}

	return nil
}

func (ns *nodeServer) mountDeviceVolumeBlock(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// Step 1: get targetPath and sourceDevice
	targetPath := req.GetTargetPath()
	sourceDevice, exists := req.VolumeContext[string(pkg.VolumeTypeDevice)]
	if !exists {
		return status.Error(codes.InvalidArgument, "Device path not provided")
	}
	log.Infof("mountDeviceVolumeBlock: targetPath %s, sourceDevice %s", targetPath, sourceDevice)

	// Step 2: check if sourceDevice is block device
	var isBlock bool
	var err error
	if isBlock, err = IsBlockDevice(sourceDevice); err != nil {
		if removeErr := os.Remove(targetPath); removeErr != nil {
			return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: Could not remove mount target %q: %v", targetPath, removeErr)
		}
		return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: check block device %s failed: %s", sourceDevice, err)
	}
	if !isBlock {
		return status.Errorf(codes.InvalidArgument, "mountDeviceVolumeBlock: %s is not block device", sourceDevice)
	}

	// Step 3: Checking if the target file is already mounted with a device.
	mounted, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: check if %s is mountpoint failed: %s", targetPath, err)
	}

	// Step 4: mount device
	mountOptions := []string{"bind"}
	// if req.GetReadonly() {
	// 	mountOptions = append(mountOptions, "ro")
	// }
	if !mounted {
		log.Infof("mountDeviceVolumeBlock: mounting %s at %s", sourceDevice, targetPath)
		if err := ns.mounter.EnsureBlock(targetPath); err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: Could not remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: ensure block %s failed: %s", targetPath, err.Error())
		}
		if err := ns.mounter.MountBlock(sourceDevice, targetPath, mountOptions...); err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: Could not remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountDeviceVolumeBlock: Could not mount block %s at %s: %s", sourceDevice, targetPath, err)
		}
	} else {
		log.Infof("mountDeviceVolumeBlock: Target path %s is already mounted", targetPath)
	}

	return nil
}

// create lvm volume
func (ns *nodeServer) createVolume(volumeContext map[string]string, volumeID, vgName, lvmType string) (string, string, error) {
	pvSize, unit, _, err := getPvInfo(ns.client, volumeID)
	if err != nil {
		return "", "", err
	}

	if pvSize == 0 {
		ephemeralVolume := volumeContext["csi.storage.k8s.io/ephemeral"] == "true" ||
			volumeContext["csi.storage.k8s.io/ephemeral"] == ""
		if ephemeralVolume {
			sizeStr, exist := volumeContext[localtype.ParamLVSize]
			if !exist {
				sizeStr = "1Gi"
			}
			quan, err := resource.ParseQuantity(sizeStr)
			if err != nil {
				return "", "", err
			}
			pvSize = quan.Value() / 1024 / 1024
			unit = "m"
		} else {
			log.Errorf("createVolume: Volume: %s, VG: %s, parse pv Size zero or snapshot lv is deleted", volumeID, vgName)
			return "", "", status.Errorf(codes.Internal, "createVolume: Volume: %s, VG: %s, parse pv Size zero or snapshot lv is deleted", volumeID, vgName)
		}
	}

	// check vg exist
	if !ns.spdkSupported {
		ckCmd := fmt.Sprintf("%s vgck %s", localtype.NsenterCmd, vgName)
		_, err = utils.Run(ckCmd)
		if err != nil {
			log.Errorf("createVolume:: VG is not exist: %s", vgName)
			return "", "", err
		}
	}

	// Create lvm volume
	if ns.spdkSupported {
		lvName := spdk.EnsureLVNameValid(volumeID)

		bdevName, err := ns.spdkclient.CreateLV(vgName, lvName, uint64(pvSize*1024*1024))
		if err != nil {
			return "", "", err
		}

		newDev, _ := ns.spdkclient.FindVhostDevice(bdevName)
		if newDev == "" {
			newDev, err = ns.spdkclient.CreateVhostDevice("ctrlr-"+uuid.New().String(), bdevName)
			if err != nil {
				return "", "", err
			}
		}

		ephemeralVolume := volumeContext["csi.storage.k8s.io/ephemeral"] == "true"
		if ephemeralVolume {
			if err := ns.ephemeralVolumeStore.AddVolume(volumeID, bdevName); err != nil {
				log.Error("fail to add volume: ", err.Error())
				if err := ns.spdkclient.CleanBdev(bdevName); err != nil {
					log.Error("createVolume - CleanBdev failed")
				}
				return "", "", err
			}
		}

		log.Infof("createVolume: Volume: %s, VG: %s, Size: %d, lvName: %s, dev: %s", volumeID, vgName, pvSize, lvName, newDev)
		return newDev, bdevName, nil
	} else {
		if err := createLvm(vgName, volumeID, lvmType, unit, pvSize); err != nil {
			return "", "", err
		}
	}
	return "", "", nil
}

func createLvm(vgName, volumeID, lvmType, unit string, pvSize int64) error {
	// Create lvm volume
	if lvmType == StripingType {
		pvNumber := getPVNumber(vgName)
		if pvNumber == 0 {
			log.Errorf("createVolume:: VG is exist: %s, bug get pv number as 0", vgName)
			return errors.New("")
		}
		cmd := fmt.Sprintf("%s lvcreate -i %d -n %s -L %d%s %s", localtype.NsenterCmd, pvNumber, volumeID, pvSize, unit, vgName)
		_, err := utils.Run(cmd)
		if err != nil {
			log.Errorf("createVolume:: lvcreate command %s error: %v", cmd, err)
			return err
		}
		log.Infof("Successful Create Striping LVM volume: %s, with command: %s", volumeID, cmd)
	} else if lvmType == LinearType {
		cmd := fmt.Sprintf("%s lvcreate -n %s -L %d%s -Wy -y %s", localtype.NsenterCmd, volumeID, pvSize, unit, vgName)
		_, err := utils.Run(cmd)
		if err != nil {
			log.Errorf("createVolume:: lvcreate linear command %s error: %v", cmd, err)
			return err
		}
		log.Infof("Successful Create Linear LVM volume: %s, with command: %s", volumeID, cmd)
	}
	return nil
}

func removeLVMByDevicePath(devicePath string) error {
	cmd := fmt.Sprintf("%s lvremove -v -f %s", localtype.NsenterCmd, devicePath)
	_, err := utils.Run(cmd)
	if err != nil {
		log.Errorf("removeLVMByDevicePath:: lvremove command %s error: %v", cmd, err)
		return err
	}
	log.Infof("Successful Remove LVM devicePath: %s, with command: %s", devicePath, cmd)
	return nil
}

func getPVNumber(vgName string) int {
	var pvCount = 0
	cmd := server.LvmCommads{}
	vgList, err := cmd.ListVG()
	if err != nil {
		log.Errorf("Get pv for vg %s with error %s", vgName, err.Error())
		return 0
	}
	for _, vg := range vgList {
		if vg.Name == vgName {
			pvCount = int(vg.PvCount)
		}
	}
	return pvCount
}

// IsBlockDevice checks if the given path is a block device
func IsBlockDevice(fullPath string) (bool, error) {
	var st unix.Stat_t
	err := unix.Stat(fullPath, &st)
	if err != nil {
		return false, err
	}

	return (st.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}

// get pvSize, pvSizeUnit, pvObject
func getPvInfo(client kubernetes.Interface, volumeID string) (int64, string, *v1.PersistentVolume, error) {
	pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
	if err != nil {
		return 0, "", nil, err
	}
	pvQuantity := pv.Spec.Capacity["storage"]
	pvSize := pvQuantity.Value()
	//pvSizeGB := pvSize / (1024 * 1024 * 1024)

	//if pvSizeGB == 0 {
	pvSizeMB := pvSize / (1024 * 1024)
	return pvSizeMB, "m", pv, nil
	//}
	//return pvSizeGB, "g", pv
}
