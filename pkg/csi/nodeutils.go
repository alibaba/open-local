package csi

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alibaba/open-local/pkg"
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/server"
	"github.com/alibaba/open-local/pkg/restic"
	"github.com/alibaba/open-local/pkg/utils"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	log "k8s.io/klog/v2"
)

func (ns *nodeServer) createLV(ctx context.Context, req *csi.NodePublishVolumeRequest) (string, string, error) {
	vgName := ""
	ephemeralVolume := req.VolumeContext[pkg.Ephemeral] == "true"
	if ephemeralVolume {
		vgName = req.VolumeContext[pkg.VGName]
	} else {
		// parse vgname, consider invalid if empty
		pvName := req.VolumeContext[pkg.PVName]
		pv, err := ns.options.kubeclient.CoreV1().PersistentVolumes().Get(context.Background(), pvName, metav1.GetOptions{})
		if err != nil {
			return "", "", fmt.Errorf("createLV: fail to get pv: %s", err.Error())
		}
		vgName = utils.GetVGNameFromCsiPV(pv)
		if vgName == "" {
			return "", "", status.Errorf(codes.Internal, "error with input vgName is empty, pv is %s", pvName)
		}
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
		if ro, exist := req.VolumeContext[localtype.ParamReadonly]; exist && ro == "true" {
			// if volume is ro snapshot, then mount snapshot lv
			log.Infof("createLV: volume %s is readonly snapshot, mount snapshot lv %s directly", volumeID, req.VolumeContext[localtype.ParamSnapshotName])
			volumeID = req.VolumeContext[localtype.ParamSnapshotName]
		}
	}
	devicePath := filepath.Join("/dev/", vgName, volumeID)
	if _, err := ns.osTool.Stat(devicePath); os.IsNotExist(err) {
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

func collectMountOptions(fsType string, mntFlags []string) []string {
	var options []string
	options = append(options, mntFlags...)

	// By default, xfs does not allow mounting of two volumes with the same filesystem uuid.
	// Force ignore this uuid to be able to mount volume + its clone / restored snapshot on the same node.
	if fsType == "xfs" {
		options = append(options, "nouuid")
	}
	return options
}

// include normal lvm & aep lvm type
func (ns *nodeServer) mountLvmFS(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// target path
	targetPath := req.TargetPath
	// device path
	devicePath, _, err := ns.createLV(ctx, req)
	if err != nil {
		return err
	}

	isSnapshotReadOnly := false
	if value, exist := req.VolumeContext[localtype.ParamReadonly]; exist && value == "true" {
		isSnapshotReadOnly = true
	}

	// check targetPath
	if _, err := ns.osTool.Stat(targetPath); os.IsNotExist(err) {
		if err := ns.osTool.MkdirAll(targetPath, 0750); err != nil {
			return fmt.Errorf("mountLvmFS: fail to mkdir target path %s: %s", targetPath, err.Error())
		}
	}
	// check if mounted
	notMounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("mountLvmFS: fail to check if %s is mounted: %s", targetPath, err.Error())
	}
	// mount if not mounted
	if notMounted {
		// fs type
		fsType := req.GetVolumeCapability().GetMount().FsType
		if len(fsType) == 0 {
			fsType = DefaultFs
		}
		var options []string
		if req.GetReadonly() || isSnapshotReadOnly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
		options = append(options, mountFlags...)
		options = append(options, collectMountOptions(fsType, options)...)

		if err := ns.k8smounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
			return fmt.Errorf("mountLvmFS: fail to format and mount volume(volume id:%s, device path: %s): %s", req.VolumeId, devicePath, err.Error())
		}

		// 判断是否为 restic 快照
		// 将 s3 数据拷贝到 targetPath 中，完毕。
		// 这里注意 param 的传递
		snapshotName, isSnapshot := req.VolumeContext[localtype.ParamSnapshotName]
		if isSnapshot {
			if !checkIfRestored(targetPath) {
				if err := labelRestored(targetPath); err != nil {
					return err
				}
				// 写一个隐藏文件，标识是否已经restore过
				srcVolomeID := req.VolumeContext[localtype.ParamSourceVolumeID]
				s3URL := req.Secrets[S3_URL]
				s3AK := req.Secrets[S3_AK]
				s3SK := req.Secrets[S3_SK]
				// restic
				_, err := restic.RestoreData(s3URL, s3AK, s3SK, srcVolomeID, restic.GeneratePassword(), targetPath, snapshotName)
				if err != nil {
					return fmt.Errorf("fail to restore volume %s path %s: %s", srcVolomeID, targetPath, err.Error())
				}
			} else {
				log.Info("target path %s has been restored", targetPath)
			}
		}

		log.Infof("mountLvmFS: mount devicePath %s to targetPath %s successfully, options: %v", devicePath, targetPath, options)
	}

	// Step 4: record
	ephemeralVolume := req.GetVolumeContext()[pkg.Ephemeral] == "true"
	if ephemeralVolume {
		if err := ns.ephemeralVolumeStore.AddVolume(req.VolumeId, devicePath); err != nil {
			log.Errorf("mountLvmFS: fail to add volume: %s", err.Error())
		}
	}
	return nil
}

func (ns *nodeServer) mountLvmBlock(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// Step 1:
	// target path
	targetPath := req.TargetPath
	// device path
	devicePath, _, err := ns.createLV(ctx, req)
	if err != nil {
		return fmt.Errorf("mountLvmBlock: fail to create lv: %s", err.Error())
	}

	// Step 2: check
	// check if devicePath is block device
	var isBlock bool
	if isBlock, err = ns.osTool.IsBlockDevice(devicePath); err != nil {
		if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
			return fmt.Errorf("mountLvmBlock: fail to remove mount target %q: %v", targetPath, removeErr)
		}
		return fmt.Errorf("mountLvmBlock: check block device %s failed: %s", devicePath, err)
	}
	if !isBlock {
		return fmt.Errorf("mountLvmBlock: %s is not block device", devicePath)
	}
	// checking if the target file is already notMounted with a device.
	notMounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return status.Errorf(codes.Internal, "mountLvmBlock: check if %s is mountpoint failed: %s", targetPath, err)
	}
	// Step 3: mount device
	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	if notMounted {
		log.Infof("mountLvmBlock: mounting %s at %s", devicePath, targetPath)
		if err := ns.osTool.EnsureBlock(targetPath); err != nil {
			if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountLvmBlock: fail to remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountLvmBlock: ensure block %s failed: %s", targetPath, err.Error())
		}
		if err := ns.osTool.MountBlock(devicePath, targetPath, mountOptions...); err != nil {
			if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
				return status.Errorf(codes.Internal, "mountLvmBlock: fail to remove mount target %q: %v", targetPath, removeErr)
			}
			return status.Errorf(codes.Internal, "mountLvmBlock: fail to mount block %s at %s: %s", devicePath, targetPath, err)
		}
	} else {
		log.Infof("mountLvmBlock: target path %s is already mounted", targetPath)
	}

	return nil
}

func (ns *nodeServer) mountMountPointVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourcePath := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[string(pkg.MPName)]; ok {
		sourcePath = value
	}
	if sourcePath == "" {
		return fmt.Errorf("mountMountPointVolume: sourcePath of volume %s is empty", req.VolumeId)
	}

	notmounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := ns.osTool.MkdirAll(targetPath, 0750); err != nil {
				return fmt.Errorf("mountMountPointVolume: fail to mkdir %s: %s", targetPath, err.Error())
			}
		} else {
			return fmt.Errorf("mountMountPointVolume: check if targetPath %s is mounted: %s", targetPath, err.Error())
		}
	}
	if !notmounted {
		log.Infof("mountMountPointVolume: volume %s(%s) is already mounted", req.VolumeId, targetPath)
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
	log.Infof("mountMountPointVolume: mount volume %s to %s with flags %v and fsType %s", req.VolumeId, targetPath, options, fsType)
	if err = ns.k8smounter.Mount(sourcePath, targetPath, fsType, options); err != nil {
		return fmt.Errorf("mountMountPointVolume: fail to mount %s to %s: %s", sourcePath, targetPath, err.Error())
	}
	return nil
}

func (ns *nodeServer) mountDeviceVolumeFS(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	targetPath := req.TargetPath
	pvName := req.VolumeContext[pkg.PVName]
	pv, err := ns.options.kubeclient.CoreV1().PersistentVolumes().Get(context.Background(), pvName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	sourceDevice := utils.GetDeviceNameFromCsiPV(pv)
	if sourceDevice == "" {
		return fmt.Errorf("mountDeviceVolumeFS: mount device %s with empty source path", req.VolumeId)
	}

	// Check targetPath
	if _, err := ns.osTool.Stat(targetPath); os.IsNotExist(err) {
		if err := ns.osTool.MkdirAll(targetPath, 0750); err != nil {
			return fmt.Errorf("mountDeviceVolumeFS: fail to mkdir %s : %s", targetPath, err.Error())
		}
	}

	notMounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("mountDeviceVolumeFS: fail to check if %s is mounted: %s", targetPath, err.Error())
	}
	if notMounted {
		// fs type
		fsType := req.GetVolumeCapability().GetMount().FsType
		if len(fsType) == 0 {
			fsType = DefaultFs
		}
		var options []string
		if req.GetReadonly() {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
		options = append(options, mountFlags...)
		options = append(options, collectMountOptions(fsType, options)...)

		if err := ns.k8smounter.FormatAndMount(sourceDevice, targetPath, fsType, options); err != nil {
			return fmt.Errorf("mountDeviceVolumeFS: fail to format and mount volume(volume id:%s, device path: %s): %s", req.VolumeId, sourceDevice, err.Error())
		}
		log.Infof("mountDeviceVolumeFS: mount devicePath %s to targetPath %s successfully, options: %v", sourceDevice, targetPath, options)
	}

	return nil
}

func (ns *nodeServer) mountDeviceVolumeBlock(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	// Step 1: get targetPath and sourceDevice
	targetPath := req.TargetPath
	pvName := req.VolumeContext[pkg.PVName]
	pv, err := ns.options.kubeclient.CoreV1().PersistentVolumes().Get(context.Background(), pvName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	sourceDevice := utils.GetDeviceNameFromCsiPV(pv)
	log.Infof("mountDeviceVolumeBlock: targetPath %s, sourceDevice %s", targetPath, sourceDevice)

	// Step 2: check if sourceDevice is block device
	var isBlock bool
	if isBlock, err = ns.osTool.IsBlockDevice(sourceDevice); err != nil {
		if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
			return fmt.Errorf("mountDeviceVolumeBlock: fail to remove mount target %q: %v", targetPath, removeErr)
		}
		return fmt.Errorf("mountDeviceVolumeBlock: fail to check block device %s: %s", sourceDevice, err)
	}
	if !isBlock {
		return fmt.Errorf("mountDeviceVolumeBlock: %s is not block device", sourceDevice)
	}

	// Step 3: Checking if the target file is already notMounted with a device.
	notMounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("mountDeviceVolumeBlock: fail to check if %s is mountpoint: %s", targetPath, err)
	}

	// Step 4: mount device
	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	if notMounted {
		log.Infof("mountDeviceVolumeBlock: mounting %s at %s", sourceDevice, targetPath)
		if err := ns.osTool.EnsureBlock(targetPath); err != nil {
			if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
				return fmt.Errorf("mountDeviceVolumeBlock: fail to remove mount target %q: %s", targetPath, removeErr.Error())
			}
			return fmt.Errorf("mountDeviceVolumeBlock: fail to ensure block %s: %s", targetPath, err.Error())
		}
		if err := ns.osTool.MountBlock(sourceDevice, targetPath, mountOptions...); err != nil {
			if removeErr := ns.osTool.Remove(targetPath); removeErr != nil {
				return fmt.Errorf("mountDeviceVolumeBlock: fail to remove mount target %q: %v", targetPath, removeErr)
			}
			return fmt.Errorf("mountDeviceVolumeBlock: fail to mount block %s at %s: %s", sourceDevice, targetPath, err)
		}
	} else {
		log.Infof("mountDeviceVolumeBlock: target path %s is already mounted", targetPath)
	}

	return nil
}

// create lvm volume
func (ns *nodeServer) createVolume(volumeContext map[string]string, volumeID, vgName, lvmType string) (string, string, error) {
	var err error
	var pvSize int64
	var unit string
	ephemeralVolume := volumeContext[pkg.Ephemeral] == "true"
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
		pvSize, unit, _, err = getPvInfo(ns.options.kubeclient, volumeID)
		if err != nil {
			return "", "", err
		}
	}

	// check vg exist
	if !ns.spdkSupported {
		ckCmd := fmt.Sprintf("%s vgck %s", localtype.NsenterCmd, vgName)
		_, err = ns.osTool.RunCommand(ckCmd)
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

		ephemeralVolume := volumeContext[pkg.Ephemeral] == "true"
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
		if err := ns.createLvm(vgName, volumeID, lvmType, unit, pvSize); err != nil {
			return "", "", err
		}
	}
	return "", "", nil
}

func (ns *nodeServer) createLvm(vgName, volumeID, lvmType, unit string, pvSize int64) error {
	// Create lvm volume
	if lvmType == StripingType {
		pvNumber := getPVNumber(vgName)
		if pvNumber == 0 {
			return fmt.Errorf("createVolume:: VG is exist: %s, bug get pv number as 0", vgName)
		}
		cmd := fmt.Sprintf("%s lvcreate -i %d -n %s -L %d%s %s", localtype.NsenterCmd, pvNumber, volumeID, pvSize, unit, vgName)
		_, err := ns.osTool.RunCommand(cmd)
		if err != nil {
			log.Errorf("createVolume:: lvcreate command %s error: %v", cmd, err)
			return err
		}
		log.Infof("Successful Create Striping LVM volume: %s, with command: %s", volumeID, cmd)
	} else if lvmType == LinearType {
		cmd := fmt.Sprintf("%s lvcreate -n %s -L %d%s -Wy -y %s", localtype.NsenterCmd, volumeID, pvSize, unit, vgName)
		_, err := ns.osTool.RunCommand(cmd)
		if err != nil {
			log.Errorf("createVolume:: lvcreate linear command %s error: %v", cmd, err)
			return err
		}
		log.Infof("Successful Create Linear LVM volume: %s, with command: %s", volumeID, cmd)
	}
	return nil
}

func (ns *nodeServer) removeLVMByDevicePath(devicePath string) error {
	cmd := fmt.Sprintf("%s lvremove -v -f %s", localtype.NsenterCmd, devicePath)
	_, err := ns.osTool.RunCommand(cmd)
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

// get pvSize, pvSizeUnit, pvObject
func getPvInfo(client kubernetes.Interface, volumeID string) (int64, string, *v1.PersistentVolume, error) {
	pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
	if err != nil {
		return 0, "", nil, err
	}
	pvQuantity := pv.Spec.Capacity["storage"]
	pvSize := pvQuantity.Value()

	pvSizeMB := pvSize / (1024 * 1024)
	return pvSizeMB, "m", pv, nil
}

const RestoreFileName = ".local.restored"

func checkIfRestored(path string) bool {
	restoredFile := filepath.Join(path, RestoreFileName)

	if _, err := os.Stat(restoredFile); os.IsNotExist(err) {
		log.Infof("not found: %s\n", restoredFile)
		return false
	} else if err != nil {
		log.Errorf("error looking for %s: %s\n", restoredFile, err)
		return false
	}

	return true
}

func labelRestored(path string) error {
	if err := ioutil.WriteFile(filepath.Join(path, RestoreFileName), nil, 0644); err != nil {
		return fmt.Errorf("error writing restored file: %s", err.Error())
	}

	return nil
}
