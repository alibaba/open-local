package csi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	localtype "github.com/oecp/open-local/pkg"
	"github.com/oecp/open-local/pkg/csi/server"
	"github.com/oecp/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	utilexec "k8s.io/utils/exec"
	k8smount "k8s.io/utils/mount"
)

// include normal lvm & aep lvm type
func (ns *nodeServer) mountLvm(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	targetPath := req.TargetPath
	// parse vgname, consider invalid if empty
	vgName := ""
	if _, ok := req.VolumeContext[VgNameTag]; ok {
		vgName = req.VolumeContext[VgNameTag]
	}
	if vgName == "" {
		log.Errorf("NodePublishVolume: request with empty vgName in volume: %s", req.VolumeId)
		return status.Error(codes.Internal, "error with input vgName is empty")
	}

	// parse lvm type and fstype
	lvmType := LinearType
	if _, ok := req.VolumeContext[LvmTypeTag]; ok {
		lvmType = req.VolumeContext[LvmTypeTag]
	}
	fsType := DefaultFs
	if _, ok := req.VolumeContext[FsTypeTag]; ok {
		fsType = req.VolumeContext[FsTypeTag]
	}
	nodeAffinity := DefaultNodeAffinity
	if _, ok := req.VolumeContext[NodeAffinity]; ok {
		nodeAffinity = req.VolumeContext[NodeAffinity]
	}
	log.Infof("NodePublishVolume: Starting to mount lvm at path: %s, with vg: %s, with volume: %s, LVM Type: %s, NodeAffinty: %s", targetPath, vgName, req.GetVolumeId(), lvmType, nodeAffinity)

	// Create LVM if not exist
	//volumeNewCreated := false
	volumeID := req.GetVolumeId()
	var isSnapshot bool = false
	var isSnapshotReadOnly bool = false
	if _, isSnapshot = req.VolumeContext[localtype.ParamSnapshotName]; isSnapshot {
		if ro, exist := req.VolumeContext[localtype.ParamSnapshotReadonly]; exist && ro == "true" {
			// if volume is ro snapshot, then mount snapshot lv
			log.Infof("NodePublishVolume: volume %s is readonly snapshot, mount snapshot lv %s directly", volumeID, req.VolumeContext[localtype.ParamSnapshotName])
			isSnapshotReadOnly = true
			volumeID = req.VolumeContext[localtype.ParamSnapshotName]
		} else {
			return status.Errorf(codes.Unimplemented, "NodePublishVolume: support ro snapshot only, please set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
		}
	}
	devicePath := filepath.Join("/dev/", vgName, volumeID)
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		//volumeNewCreated = true
		err := ns.createVolume(ctx, volumeID, vgName, lvmType)
		if err != nil {
			log.Errorf("NodePublishVolume: create volume %s with error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
	}

	// Check targetPath
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0750); err != nil {
			log.Errorf("NodePublishVolume: volume %s mkdir target path %s with error: %s", volumeID, targetPath, err.Error())
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
			log.Errorf("NodeStageVolume: Volume: %s, Device: %s, FormatAndMount error: %s", req.VolumeId, devicePath, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		log.Infof("NodePublishVolume:: mount successful devicePath: %s, targetPath: %s, options: %v", devicePath, targetPath, options)
	}
	// upgrade PV with NodeAffinity
	if nodeAffinity == "true" && !isSnapshot {
		oldPv, err := ns.client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
		if err != nil {
			log.Errorf("NodePublishVolume: Get Persistent Volume(%s) Error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		if oldPv.Spec.NodeAffinity == nil {
			oldData, err := json.Marshal(oldPv)
			if err != nil {
				log.Errorf("NodePublishVolume: Marshal Persistent Volume(%s) Error: %s", volumeID, err.Error())
				return status.Error(codes.Internal, err.Error())
			}
			pvClone := oldPv.DeepCopy()

			// construct new persistent volume data
			values := []string{ns.nodeID}
			nSR := v1.NodeSelectorRequirement{Key: "kubernetes.io/hostname", Operator: v1.NodeSelectorOpIn, Values: values}
			matchExpress := []v1.NodeSelectorRequirement{nSR}
			nodeSelectorTerm := v1.NodeSelectorTerm{MatchExpressions: matchExpress}
			nodeSelectorTerms := []v1.NodeSelectorTerm{nodeSelectorTerm}
			required := v1.NodeSelector{NodeSelectorTerms: nodeSelectorTerms}
			pvClone.Spec.NodeAffinity = &v1.VolumeNodeAffinity{Required: &required}
			newData, err := json.Marshal(pvClone)
			if err != nil {
				log.Errorf("NodePublishVolume: Marshal New Persistent Volume(%s) Error: %s", volumeID, err.Error())
				return status.Error(codes.Internal, err.Error())
			}
			patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, pvClone)
			if err != nil {
				log.Errorf("NodePublishVolume: CreateTwoWayMergePatch Volume(%s) Error: %s", volumeID, err.Error())
				return status.Error(codes.Internal, err.Error())
			}

			// Upgrade PersistentVolume with NodeAffinity
			_, err = ns.client.CoreV1().PersistentVolumes().Patch(context.Background(), volumeID, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			if err != nil {
				log.Errorf("NodePublishVolume: Patch Volume(%s) Error: %s", volumeID, err.Error())
				return status.Error(codes.Internal, err.Error())
			}
			log.Infof("NodePublishVolume: upgrade Persistent Volume(%s) with nodeAffinity: %s", volumeID, ns.nodeID)
		}
	}
	return nil
}

func (ns *nodeServer) mountMountPointVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourcePath := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[MountPointType]; ok {
		sourcePath = value
	}
	if sourcePath == "" {
		log.Errorf("mountLocalVolume: volume: %s, sourcePath empty", req.VolumeId)
		return status.Error(codes.Internal, "Mount LocalVolume with empty source path "+req.VolumeId)
	}

	notmounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		log.Errorf("mountLocalVolume: check volume: %s mounted with error %v", req.VolumeId, err)
		return status.Error(codes.Internal, err.Error())
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

func (ns *nodeServer) mountDeviceVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourceDevice := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[DeviceVolumeType]; ok {
		sourceDevice = value
	}
	if sourceDevice == "" {
		log.Errorf("mountDeviceVolume: device volume: %s, sourcePath empty", req.VolumeId)
		return status.Error(codes.Internal, "Mount Device with empty source path "+req.VolumeId)
	}

	// Step Start to format
	mnt := req.VolumeCapability.GetMount()
	options := append(mnt.MountFlags, "shared")
	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	// do format-mount or mount
	diskMounter := &k8smount.SafeFormatAndMount{Interface: ns.k8smounter, Exec: utilexec.New()}
	if err := diskMounter.FormatAndMount(sourceDevice, targetPath, fsType, options); err != nil {
		log.Errorf("mountDeviceVolume: Volume: %s, Device: %s, FormatAndMount error: %s", req.VolumeId, sourceDevice, err.Error())
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (ns *nodeServer) checkTargetMounted(targetPath string) (bool, error) {
	isMnt, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return isMnt, status.Error(codes.Internal, err.Error())
			}
			isMnt = false
		} else {
			return false, err
		}
	}
	return isMnt, nil
}

func (ns *nodeServer) updatePVNodeAffinity(volumeID string) error {
	oldPv, err := ns.client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
	if err != nil {
		log.Errorf("NodePublishVolume: Get Persistent Volume(%s) Error: %s", volumeID, err.Error())
		return status.Error(codes.Internal, err.Error())
	}
	if oldPv.Spec.NodeAffinity == nil {
		oldData, err := json.Marshal(oldPv)
		if err != nil {
			log.Errorf("NodePublishVolume: Marshal Persistent Volume(%s) Error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		pvClone := oldPv.DeepCopy()

		// construct new persistent volume data
		values := []string{ns.nodeID}
		nSR := v1.NodeSelectorRequirement{Key: "kubernetes.io/hostname", Operator: v1.NodeSelectorOpIn, Values: values}
		matchExpress := []v1.NodeSelectorRequirement{nSR}
		nodeSelectorTerm := v1.NodeSelectorTerm{MatchExpressions: matchExpress}
		nodeSelectorTerms := []v1.NodeSelectorTerm{nodeSelectorTerm}
		required := v1.NodeSelector{NodeSelectorTerms: nodeSelectorTerms}
		pvClone.Spec.NodeAffinity = &v1.VolumeNodeAffinity{Required: &required}
		newData, err := json.Marshal(pvClone)
		if err != nil {
			log.Errorf("NodePublishVolume: Marshal New Persistent Volume(%s) Error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, pvClone)
		if err != nil {
			log.Errorf("NodePublishVolume: CreateTwoWayMergePatch Volume(%s) Error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}

		// Upgrade PersistentVolume with NodeAffinity
		_, err = ns.client.CoreV1().PersistentVolumes().Patch(context.Background(), volumeID, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			log.Errorf("NodePublishVolume: Patch Volume(%s) Error: %s", volumeID, err.Error())
			return status.Error(codes.Internal, err.Error())
		}
		log.Infof("NodePublishVolume: upgrade Persistent Volume(%s) with nodeAffinity: %s", volumeID, ns.nodeID)
	}
	return nil
}

// create lvm volume
func (ns *nodeServer) createVolume(ctx context.Context, volumeID, vgName, lvmType string) error {
	pvSize, unit, _ := getPvInfo(ns.client, volumeID)
	if pvSize == 0 {
		log.Errorf("createVolume: Volume: %s, VG: %s, parse pv Size zero or snapshot lv is deleted", volumeID, vgName)
		return status.Errorf(codes.Internal, "createVolume: Volume: %s, VG: %s, parse pv Size zero or snapshot lv is deleted", volumeID, vgName)
	}
	var err error

	// check vg exist
	ckCmd := fmt.Sprintf("%s vgck %s", localtype.NsenterCmd, vgName)
	_, err = utils.Run(ckCmd)
	if err != nil {
		log.Errorf("createVolume:: VG is not exist: %s", vgName)
		return err
	}

	// Create lvm volume
	if err := createLvm(vgName, volumeID, lvmType, unit, pvSize); err != nil {
		return err
	}

	return nil
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
		cmd := fmt.Sprintf("%s lvcreate -n %s -L %d%s %s", localtype.NsenterCmd, volumeID, pvSize, unit, vgName)
		_, err := utils.Run(cmd)
		if err != nil {
			log.Errorf("createVolume:: lvcreate linear command %s error: %v", cmd, err)
			return err
		}
		log.Infof("Successful Create Linear LVM volume: %s, with command: %s", volumeID, cmd)
	}
	return nil
}

func getPVNumber(vgName string) int {
	var pvCount = 0
	vgList, err := server.ListVG()
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
