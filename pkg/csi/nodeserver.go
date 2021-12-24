/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/server"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
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
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	nodeID     string
	driverName string
	mounter    utils.Mounter
	client     kubernetes.Interface
	k8smounter k8smount.Interface
	sysPath    string
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

	mounter := k8smount.New("")

	// local volume daemon
	// GRPC server to provide volume manage
	go server.Start()

	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		nodeID:            nodeID,
		mounter:           utils.NewMounter(),
		k8smounter:        mounter,
		client:            kubeClient,
		driverName:        dName,
		sysPath:           sysPath,
	}
}

// volume_id: yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866
// staging_target_path: /var/lib/kubelet/plugins/kubernetes.io/csi/pv/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/globalmount
// target_path: /var/lib/kubelet/pods/2a7bbb9c-c915-4006-84d7-0e3ac9d8d70f/volumes/kubernetes.io~csi/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/mount
func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Debugf("NodePublishVolume:: local volume request with %v", req)

	// parse request args.
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		log.Errorf("NodePublishVolume: mount local volume %s with path %s", req.VolumeId, targetPath)
		return nil, status.Error(codes.Internal, "NodePublishVolume: targetPath is empty")
	}

	volumeType := ""
	if _, ok := req.VolumeContext[VolumeTypeTag]; ok {
		volumeType = req.VolumeContext[VolumeTypeTag]
	}

	volCap := req.GetVolumeCapability()
	switch volumeType {
	case LvmVolumeType:
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountLvmBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmBlock): mount lvm volume %s with path %s with error: %s", req.VolumeId, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountLvmFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(mountLvmFS): mount lvm volume %s with path %s with error: %s", req.VolumeId, targetPath, err.Error())
			}
		}
	case MountPointType:
		err := ns.mountMountPointVolume(ctx, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: mount mountpoint volume %s with path %s with error: %s", req.VolumeId, targetPath, err.Error())
		}
	case DeviceVolumeType:
		switch volCap.GetAccessType().(type) {
		case *csi.VolumeCapability_Block:
			err := ns.mountDeviceVolumeBlock(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(Block): mount device volume %s with path %s with error: %s", req.VolumeId, targetPath, err.Error())
			}
		case *csi.VolumeCapability_Mount:
			err := ns.mountDeviceVolumeFS(ctx, req)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "NodePublishVolume(FileSystem): mount device volume %s with path %s with error: %s", req.VolumeId, targetPath, err.Error())
			}
		}
	default:
		return nil, status.Errorf(codes.Internal, "NodePublishVolume: unsupported volume %s with type %s", req.VolumeId, volumeType)
	}

	if err := ns.setIOThrottling(ctx, req); err != nil {
		return nil, err
	}

	log.Infof("NodePublishVolume: Successful mount local volume %s to %s", req.VolumeId, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	log.Infof("NodeUnpublishVolume: Starting to unmount target path %s for volume %s", targetPath, req.VolumeId)

	isMnt, err := ns.mounter.IsMounted(targetPath)
	if err != nil {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			log.Infof("NodeUnpublishVolume: Target path not exist for volume %s with path %s", req.VolumeId, targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		log.Errorf("NodeUnpublishVolume: Stat volume %s at path %s with error %v", req.VolumeId, targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !isMnt {
		_, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Infof("NodeUnpublishVolume: Target path %s not exist", targetPath)
				return &csi.NodeUnpublishVolumeResponse{}, nil
			} else {
				log.Errorf("NodeUnpublishVolume: Stat volume %s at path %s with error %s", req.VolumeId, targetPath, err.Error())
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodeUnpublishVolume: Stat volume %s at path %s with error %s", req.VolumeId, targetPath, err.Error()))
			}
		}
		log.Errorf("NodeUnpublishVolume: volume %s at path %s still existed", req.VolumeId, targetPath)
		return nil, status.Error(codes.Internal, fmt.Sprintf("NodeUnpublishVolume: volume %s at path %s still existed", req.VolumeId, targetPath))
	}

	err = ns.mounter.Unmount(req.GetTargetPath())
	if err != nil {
		log.Errorf("NodeUnpublishVolume: Umount volume %s for path %s with error %v", req.VolumeId, targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	isMnt, err = ns.mounter.IsMounted(targetPath)
	if err != nil {
		log.Errorf("NodeUnpublishVolume: check if path %s is mounted error %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if isMnt {
		log.Errorf("NodeUnpublishVolume: Umount volume %s for path %s not successful", req.VolumeId, targetPath)
		return nil, status.Error(codes.Internal, fmt.Sprintf("Umount volume %s not successful", req.VolumeId))
	}

	log.Infof("NodeUnpublishVolume: Successful umount target path %s for volume %s", targetPath, req.VolumeId)
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
	if err := ns.resizeVolume(ctx, volumeID, targetPath); err != nil {
		log.Errorf("NodePublishVolume: Resize local volume %s with error: %s", volumeID, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
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
		log.Errorf("resizeVolume:: local volume get pv info error %s", volumeID)
		return status.Errorf(codes.Internal, "resizeVolume:: local volume get pv info error %s", volumeID)
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
			log.Errorf("NodeExpandVolume:: Lvm Resize Error, volumeId: %s, devicePath: %s, volumePath: %s, err: %s", volumeID, devicePath, targetPath, err.Error())
			return err
		}
		if !ok {
			log.Errorf("NodeExpandVolume:: Lvm Resize failed, volumeId: %s, devicePath: %s, volumePath: %s", volumeID, devicePath, targetPath)
			return status.Error(codes.Internal, "Fail to resize volume fs")
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
		log.Errorf("getPvInfo: fail to get pv, err: %v", err)
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
		log.Debugf("pod(volume id %s) uuid is %s", req.VolumeId, podUID)
		namespace := req.VolumeContext[localtype.PVCNameSpace]
		pods, err := ns.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
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
		blkioPath := fmt.Sprintf("%s/fs/cgroup/blkio/kubepods.slice", ns.sysPath)
		switch qosClass {
		case v1.PodQOSGuaranteed:
			blkioPath = path.Join(blkioPath, fmt.Sprintf("kubepods-pod%s.slice", strings.Replace(podUID, "-", "_", -1)))
		case v1.PodQOSBurstable:
			blkioPath = path.Join(blkioPath, "kubepods-burstable.slice", fmt.Sprintf("kubepods-burstable-pod%s.slice", strings.Replace(podUID, "-", "_", -1)))
		case v1.PodQOSBestEffort:
			blkioPath = path.Join(blkioPath, "kubepods-besteffort.slice", fmt.Sprintf("kubepods-besteffort-pod%s.slice", strings.Replace(podUID, "-", "_", -1)))
		}
		log.Debugf("pod(volume id %s) qosClass: %s", req.VolumeId, qosClass)
		log.Debugf("pod(volume id %s) blkio path: %s", req.VolumeId, blkioPath)
		// get lv lvpath
		lvpath, err := ns.createLV(ctx, req)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to get lv path %s: %s", req.VolumeId, err.Error())
		}
		stat := syscall.Stat_t{}
		_ = syscall.Stat(lvpath, &stat)
		maj := uint64(stat.Rdev / 256)
		min := uint64(stat.Rdev % 256)
		log.Debugf("volume %s maj:min: %d:%d", req.VolumeId, maj, min)
		log.Debugf("volume %s path: %s", req.VolumeId, lvpath)
		if iopsExist {
			log.Debugf("volume %s iops: %s", req.VolumeId, iops)
			cmdstr := fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, iops), fmt.Sprintf("%s/%s", blkioPath, localtype.IOPSReadFile))
			_, err := exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s/%s", blkioPath, localtype.IOPSReadFile), err.Error())
			}
			cmdstr = fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, iops), fmt.Sprintf("%s/%s", blkioPath, localtype.IOPSWriteFile))
			_, err = exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s/%s", blkioPath, localtype.IOPSWriteFile), err.Error())
			}
		}
		if bpsExist {
			log.Debugf("volume %s bps: %s", req.VolumeId, bps)
			cmdstr := fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, bps), fmt.Sprintf("%s/%s", blkioPath, localtype.BPSReadFile))
			_, err := exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s/%s", blkioPath, localtype.BPSReadFile), err.Error())
			}
			cmdstr = fmt.Sprintf("echo %s > %s", fmt.Sprintf("%d:%d %s", maj, min, bps), fmt.Sprintf("%s/%s", blkioPath, localtype.BPSWriteFile))
			_, err = exec.Command("sh", "-c", cmdstr).CombinedOutput()
			if err != nil {
				return status.Errorf(codes.Internal, "failed to write blkio file %s: %s", fmt.Sprintf("%s/%s", blkioPath, localtype.BPSWriteFile), err.Error())
			}
		}
	}
	return nil
}
