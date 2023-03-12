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
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-local/pkg"
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/adapter"
	"github.com/alibaba/open-local/pkg/csi/client"
	"github.com/alibaba/open-local/pkg/csi/server"
	"github.com/alibaba/open-local/pkg/restic"
	"github.com/alibaba/open-local/pkg/signals"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/docker/go-units"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	log "k8s.io/klog/v2"
)

type controllerServer struct {
	inFlight           *InFlight
	pvcPodSchedulerMap *PvcPodSchedulerMap
	schedulerArchMap   *SchedulerArchMap
	adapter            adapter.Adapter

	nodeLister corelisters.NodeLister
	podLister  corelisters.PodLister
	pvcLister  corelisters.PersistentVolumeClaimLister
	pvLister   corelisters.PersistentVolumeLister

	options *driverOptions
}

func newControllerServer(options *driverOptions) *controllerServer {
	pvcPodSchedulerMap := newPvcPodSchedulerMap()
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(options.kubeclient, time.Second*30)
	kubeInformerFactory.Core().V1().Pods().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if pod == nil {
					return
				}
				for _, v := range pod.Spec.Volumes {
					if v.PersistentVolumeClaim != nil {
						pvcPodSchedulerMap.Add(pod.Namespace, v.PersistentVolumeClaim.ClaimName, pod.Spec.SchedulerName)
					}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				pod := newObj.(*v1.Pod)
				if pod == nil {
					return
				}
				for _, v := range pod.Spec.Volumes {
					if v.PersistentVolumeClaim != nil {
						pvcPodSchedulerMap.Add(pod.Namespace, v.PersistentVolumeClaim.ClaimName, pod.Spec.SchedulerName)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if pod == nil {
					return
				}
				for _, v := range pod.Spec.Volumes {
					if v.PersistentVolumeClaim != nil {
						pvcPodSchedulerMap.Remove(pod.Namespace, v.PersistentVolumeClaim.ClaimName)
					}
				}
			},
		},
	)
	cm := &controllerServer{
		inFlight:           NewInFlight(),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		pvcPodSchedulerMap: pvcPodSchedulerMap,
		schedulerArchMap:   newSchedulerArchMap(options.extenderSchedulerNames, options.frameworkSchedulerNames),
		adapter:            adapter.NewExtenderAdapter(),
		options:            options,
	}
	stopCh := signals.SetupSignalHandler()
	kubeInformerFactory.Start(stopCh)
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(
		stopCh,
		kubeInformerFactory.Core().V1().Nodes().Informer().HasSynced,
		kubeInformerFactory.Core().V1().Pods().Informer().HasSynced,
		kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer().HasSynced,
		kubeInformerFactory.Core().V1().PersistentVolumes().Informer().HasSynced,
	); !ok {
		log.Fatalf("failed to wait for caches to sync")
	}
	log.Info("informer sync successfully")

	return cm
}

// CreateVolume csi interface
func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log.V(4).Infof("CreateVolume: called with args %+v", *req)
	volumeID := req.GetName()
	// check request
	if err := validateCreateVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: fail to validate CreateVolumeRequest: %s", err.Error())
	}

	// get node name for client.Connection
	parameters := req.GetParameters()
	volumeType := parameters[pkg.VolumeTypeKey]
	pvcName := parameters[pkg.PVCName]
	pvcNameSpace := parameters[pkg.PVCNameSpace]
	if pvcName == "" || pvcNameSpace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: pvcName(%s) or pvcNamespace(%s) can not be empty", pvcNameSpace, pvcName)
	}
	pvc, err := cs.pvcLister.PersistentVolumeClaims(pvcNameSpace).Get(pvcName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get pvc: %s", err.Error())
	}
	nodeName, exist := pvc.Annotations[pkg.AnnoSelectedNode]
	if !exist {
		return nil, status.Errorf(codes.Unimplemented, "CreateVolume: no annotation %s found in pvc %s/%s. Check if volumeBindingMode of storageclass is WaitForFirstConsumer, cause we only support WaitForFirstConsumer mode", pkg.AnnoSelectedNode, pvcNameSpace, pvcName)
	}
	log.Infof("CreateVolume: starting to Create %s volume %s with: PVC(%s/%s), nodeSelected(%s)", volumeType, volumeID, pvcNameSpace, pvcName, nodeName)

	// 若特定 volumeID 已在执行中
	// 则立即返回
	if ok := cs.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		cs.inFlight.Delete(volumeID)
	}()

	paramMap := map[string]string{}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: fail to connect to node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// 通过与调度器互动
	// 将重要的属性更新在 paramMap
	schedulerName := cs.pvcPodSchedulerMap.Get(pvcNameSpace, pvcName)
	if cs.schedulerArchMap.Get(schedulerName) == SchedulerArchExtender {
		log.Infof("CreateVolume: scheduler arch of pvc(%s/%s) is %s", pvcNameSpace, pvcName, SchedulerArchExtender)
		switch volumeType {
		case string(pkg.VolumeTypeLVM):
			// 若为只读快照，则直接退出
			if utils.IsReadOnlySnapshotPVC2(pvc, cs.options.snapclient) {
				break
			}

			// 获取 vgName
			paramMap, err = cs.scheduleLVMVolume(nodeName, pvcName, pvcNameSpace, parameters)
			if err != nil {
				code := codes.Internal
				if strings.Contains(err.Error(), "Insufficient") {
					code = codes.ResourceExhausted
				}
				return nil, status.Errorf(code, "CreateVolume: fail to schedule LVM %s: %s", volumeID, err.Error())
			}
			vgName := paramMap[VgNameTag]
			log.Infof("CreateVolume: schedule LVM %s with %s, %s", volumeID, nodeName, vgName)

			if vgName == "" {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: empty vgName in %s", volumeID)
			}

			// create lv
			options := &client.LVMOptions{}
			options.Name = req.Name
			options.VolumeGroup = vgName
			if value, ok := parameters[LvmTypeTag]; ok && value == StripingType {
				options.Striping = true
			}
			options.Size = uint64(req.GetCapacityRange().GetRequiredBytes())
			if lvName, err := conn.GetVolume(ctx, vgName, volumeID); err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get lv %s from node %s: %s", req.Name, nodeName, err.Error())
			} else {
				if lvName == "" {
					log.Info("CreateVolume: volume %s not found, creating volume on node %s", volumeID, nodeName)
					outstr, err := conn.CreateVolume(ctx, options)
					if err != nil {
						return nil, status.Errorf(codes.Internal, "CreateVolume: fail to create lv %s/%s(options: %v): %s", vgName, volumeID, options, err.Error())
					}
					log.Infof("CreateLvm: create lvm %s/%s in node %s with response %s successfully", vgName, volumeID, nodeName, outstr)
				} else {
					log.Infof("CreateVolume: lv %s already created at node %s", req.Name, nodeName)
				}
			}
		case string(pkg.VolumeTypeMountPoint):
			var err error
			paramMap, err = cs.scheduleMountpointVolume(nodeName, pvcName, pvcNameSpace, parameters)
			if err != nil {
				code := codes.Internal
				if strings.Contains(err.Error(), "Insufficient") {
					code = codes.ResourceExhausted
				}
				return nil, status.Errorf(code, "CreateVolume: fail to schedule mountpoint %s at node %s: %s", req.Name, nodeName, err.Error())
			}
			log.Infof("CreateVolume: create mountpoint %s at node %s successfully", req.Name, nodeName)
		case string(pkg.VolumeTypeDevice):
			var err error
			paramMap, err = cs.scheduleDeviceVolume(nodeName, pvcName, pvcNameSpace, parameters)
			if err != nil {
				code := codes.Internal
				if strings.Contains(err.Error(), "Insufficient") {
					code = codes.ResourceExhausted
				}
				return nil, status.Errorf(code, "CreateVolume: fail to schedule device volume %s at node %s: %s", req.Name, nodeName, err.Error())
			}
			log.Infof("CreateVolume: create device %s at node %s successfully", req.Name, nodeName)
		default:
			return nil, status.Errorf(codes.Unimplemented, "CreateVolume: no support volume type %s", volumeType)
		}
	} else if cs.schedulerArchMap.Get(schedulerName) == SchedulerArchFramework {
		log.Infof("CreateVolume: scheduler arch of pvc(%s/%s) is %s", pvcNameSpace, pvcName, SchedulerArchFramework)
	} else {
		return nil, status.Errorf(codes.Unknown, "CreateVolume: scheduler arch of pvc(%s/%s) is %s, plz check again", pvcNameSpace, pvcName, SchedulerArchUnknown)
	}

	// 处理快照逻辑
	isSnapshot := false
	if volumeSource := req.GetVolumeContentSource(); volumeSource != nil {
		if volumeType == string(pkg.VolumeTypeLVM) {
			// validate
			if _, ok := volumeSource.GetType().(*csi.VolumeContentSource_Snapshot); !ok {
				return nil, status.Error(codes.InvalidArgument, "CreateVolume: unsupported volumeContentSource type")
			}
			log.Infof("CreateVolume: source of volume %s is snapshot", volumeID)
			isSnapshot = true

			// get snapshot name
			sourceSnapshot := volumeSource.GetSnapshot()
			if sourceSnapshot == nil {
				return nil, status.Error(codes.InvalidArgument, "CreateVolume: fail to retrive snapshot from the volumeContentSource")
			}
			snapshotID := sourceSnapshot.GetSnapshotId()
			log.Infof("CreateVolume: snapshotID of volume %s is %s", volumeID, snapshotID)
			// get src volume ID
			snapContent, err := utils.GetVolumeSnapshotContent(cs.options.snapclient, snapshotID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get snapshot content: %s", err.Error())
			}

			srcVolumeID := *snapContent.Spec.Source.VolumeHandle
			log.Infof("CreateVolume: srcVolumeID of snapshot %s(volumeID %s) is %s", volumeID, snapshotID, srcVolumeID)
			paramMap[localtype.ParamSourceVolumeID] = srcVolumeID
			paramMap[localtype.ParamSnapshotID] = snapshotID

			if utils.IsReadOnlySnapshotPVC2(pvc, cs.options.snapclient) {
				// 只读快照需要特殊处理
				// 必须依赖于 源PV
				log.Infof("pvc %s/%s snapshot is ro", pvcNameSpace, pvcName)
				// get node name and vg name from src volume
				pv, err := cs.pvLister.Get(srcVolumeID)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get source pv of ro snasphot %s: %s", snapshotID, err.Error())
				}
				vgName := utils.GetVGNameFromCsiPV(pv)
				if vgName == "" {
					return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get vgName from pv %s", pv.Name)
				}
				// 只读快照要求必须与 源PV 同 VG
				paramMap[VgNameTag] = vgName
				paramMap[localtype.ParamReadonly] = "true"
			} else {
				// 读写快照需要获取 secret
				log.Infof("pvc %s/%s snapshot is rw", pvcNameSpace, pvcName)
			}
			log.Infof("CreateVolume: get restic snapshot snapshot volume %s info", volumeID)
		} else {
			return nil, status.Errorf(codes.Unimplemented, "unsupported snapshot %s", volumeType)
		}
	}

	for key, value := range paramMap {
		parameters[key] = value
	}
	parameters[pkg.AnnoSelectedNode] = nodeName
	response := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: parameters,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						pkg.KubernetesNodeIdentityKey: nodeName,
					},
				},
			},
		},
	}
	if isSnapshot {
		response.Volume.ContentSource = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: paramMap[localtype.ParamSnapshotID],
				},
			},
		}
	}

	log.Infof("CreateVolume: create volume %s(size: %d) successfully", volumeID, req.GetCapacityRange().GetRequiredBytes())
	return response, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log.V(4).Infof("DeleteVolume: called with args %+v", *req)
	// check request
	if err := validateDeleteVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "DeleteVolume: fail to validate DeleteVolumeRequest: %s", err.Error())
	}

	volumeID := req.GetVolumeId()
	if ok := cs.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		cs.inFlight.Delete(volumeID)
	}()

	// check if volume content source is snapshot
	pv, err := cs.pvLister.Get(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get pv: %s", err.Error())
	}
	isSnapshot := false
	isSnapshotReadOnly := false
	if pv.Spec.CSI != nil {
		attributes := pv.Spec.CSI.VolumeAttributes
		if value, exist := attributes[localtype.ParamSnapshotID]; exist && value != "" {
			isSnapshot = true
		}
		if value, exist := attributes[localtype.ParamReadonly]; exist && value == "true" {
			isSnapshotReadOnly = true
		}
	}
	if isSnapshot && isSnapshotReadOnly {
		log.Infof("DeleteVolume: volume %s is ro snapshot volume, skip delete lv", volumeID)
		// break switch
		return &csi.DeleteVolumeResponse{}, nil
	}

	// delete volume
	nodeName := utils.GetNodeNameFromCsiPV(pv)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get node name of pv %s", pv.Name)
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to connect to node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	volumeType := pv.Spec.CSI.VolumeAttributes[pkg.VolumeTypeKey]
	switch volumeType {
	case string(pkg.VolumeTypeLVM):
		vgName := utils.GetVGNameFromCsiPV(pv)
		if vgName == "" {
			log.Warningf("DeleteVolume: delete local volume %s with empty vgName(may be hacked)", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}

		if lvName, err := conn.GetVolume(ctx, vgName, volumeID); err != nil {
			if strings.Contains(err.Error(), "Failed to find logical volume") {
				log.Warningf("DeleteVolume: lvm volume not found, skip deleting %s", volumeID)
				return &csi.DeleteVolumeResponse{}, nil
			} else if strings.Contains(err.Error(), "Volume group \""+vgName+"\" not found") {
				log.Warningf("DeleteVolume: Volume group not found, skip deleting %s", volumeID)
				return &csi.DeleteVolumeResponse{}, nil
			} else {
				return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get lv %s: %s", volumeID, err.Error())
			}
		} else {
			if lvName != "" {
				log.Infof("DeleteVolume: found lv %s/%s at node %s, now deleting", vgName, volumeID, nodeName)
				if err := conn.DeleteVolume(ctx, vgName, volumeID); err != nil {
					return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to delete lv %s: %s", volumeID, err.Error())
				}
				log.Infof("DeleteVolume: delete lv %s/%s at node %s successfully", vgName, volumeID, nodeName)
			} else {
				log.Warningf("DeleteVolume: empty lv name, skip deleting %s", volumeID)
				return &csi.DeleteVolumeResponse{}, nil
			}
		}
	case string(pkg.VolumeTypeMountPoint):
		path := utils.GetMountPointFromCsiPV(pv)
		if path == "" {
			return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get mountpath of pv %s", pv.Name)
		}
		if err := conn.CleanPath(ctx, path); err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to delete mountpoint %s: %s", path, err.Error())
		}
		log.Infof("DeleteVolume: delete MountPoint volume(%s) successfully", volumeID)
	case string(pkg.VolumeTypeDevice):
		device := utils.GetDeviceNameFromCsiPV(pv)
		if device == "" {
			return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get device of pv %s", pv.Name)
		}
		if err := conn.CleanDevice(ctx, device); err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to delete device: %s", err.Error())
		}
		log.Infof("DeleteVolume: delete Device volume(%s) successfully", volumeID)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "DeleteVolume: volumeType %s not supported %s", volumeType, volumeID)
	}
	log.Infof("DeleteVolume: delete local volume %s successfully", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// CreateSnapshot create lvm snapshot
// 处理两类快照
// 只读快照
// 读写快照
func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log.V(4).Infof("CreateSnapshot: called with args %+v", *req)

	// check request
	snapshotName := req.GetName()
	if len(snapshotName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot: snapshot name not provided")
	}
	srcVolumeID := req.GetSourceVolumeId()
	if len(srcVolumeID) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "CreateSnapshot: snapshot %s volume source ID not provided", snapshotName)
	}

	// get vgName
	srcPV, err := cs.pvLister.Get(srcVolumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get pv: %s", err.Error())
	}
	vgName := utils.GetVGNameFromCsiPV(srcPV)
	if vgName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get vgName of pv %s", srcPV.Name)
	}
	log.Infof("CreateSnapshot: vg of snapshot %s is %s", snapshotName, vgName)

	// get nodeName
	nodeName := utils.GetNodeNameFromCsiPV(srcPV)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get node name of pv %s", srcPV.Name)
	}

	if ok := cs.inFlight.Insert(snapshotName); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, snapshotName)
	}
	defer func() {
		cs.inFlight.Delete(snapshotName)
	}()

	// get grpc client
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get grpc client at node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	var sizeBytes int64
	readonly := false
	if value, exist := req.Parameters[localtype.ParamReadonly]; exist && value == "true" {
		readonly = true
	}
	if readonly {
		// 只读快照
		log.Infof("snapshot %s is readonly", snapshotName)
		// get snapshot initial size from parameter
		initialSize, _, _, err := getSnapshotInitialInfo(req.Parameters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSnapshot: get snapshot %s initial info error: %s", req.Name, err.Error())
		}
		// update initialSize if initialSize is bigger than pv request size
		srcPVSize, _ := srcPV.Spec.Capacity.Storage().AsInt64()
		if srcPVSize < int64(initialSize) {
			initialSize = uint64(srcPVSize)
		}

		// create lvm snapshot
		var lvmName string
		if lvmName, err = conn.GetVolume(ctx, vgName, snapshotName); err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
		}
		if lvmName == "" {
			log.Infof("CreateSnapshot: ro snapshot %s/%s not found, now creating with initialSize %d on node %s", vgName, snapshotName, initialSize, nodeName)
			sizeBytes, err = conn.CreateSnapshot(ctx, vgName, snapshotName, srcVolumeID, true, int64(initialSize), nil)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateSnapshot: create lvm snapshot %s failed: %s", snapshotName, err.Error())
			}
			log.Infof("CreateSnapshot: create ro snapshot %s successfully", snapshotName)
		} else {
			log.Infof("CreateSnapshot: lvm snapshot %s in node %s already exists", snapshotName, nodeName)
		}
		if sizeBytes == 0 {
			// 只读快照只返回 0
			sizeBytes = int64(initialSize)
		}
	} else {
		log.Infof("snapshot %s is readwrite, now creating...", snapshotName)
		// create rw snapshot
		sizeBytes, err = conn.CreateSnapshot(ctx, vgName, snapshotName, srcVolumeID, false, 0, req.Secrets)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to create snapshot %s: %s", snapshotName, err.Error())
		}
		log.Infof("CreateSnapshot: create rw snapshot %s successfully", snapshotName)
		if sizeBytes == 0 {
			srcPVSize, _ := srcPV.Spec.Capacity.Storage().AsInt64()
			sizeBytes = srcPVSize
		}
	}

	log.Infof("CreateSnapshot: create snapshot %s successfully", snapshotName)
	return cs.newCreateSnapshotResponse(snapshotName, req.SourceVolumeId, sizeBytes)
}

// DeleteSnapshot delete lvm snapshot
func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	log.Infof("DeleteSnapshot: called with args %+v", *req)
	// check req
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot: Snapshot ID not provided")
	}

	// get volumeID from snapshotcontent
	snapContent, err := utils.GetVolumeSnapshotContent(cs.options.snapclient, snapshotID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get snapContent %s error: %s", snapshotID, err.Error())
	}
	srcVolumeID := *snapContent.Spec.Source.VolumeHandle
	snapshotClass, err := cs.options.snapclient.SnapshotV1().VolumeSnapshotClasses().Get(context.TODO(), *snapContent.Spec.VolumeSnapshotClassName, metav1.GetOptions{})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get snapshotClass %s error: %s", *snapContent.Spec.VolumeSnapshotClassName, err.Error())
	}

	// 判断是否为只读快照
	readonly := false
	if value, exist := snapshotClass.Parameters[localtype.ParamReadonly]; exist && value == "true" {
		readonly = true
	}

	if ok := cs.inFlight.Insert(snapshotID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, snapshotID)
	}
	defer func() {
		cs.inFlight.Delete(snapshotID)
	}()

	if readonly {
		// 要求删除快照时 源pv 还在，不然快照删除不了
		// 这里可以保证 源pv 在有快照的情况下 delete volume 会失败: logical volume ** has snapshot, please remove snapshot first
		pv, err := cs.pvLister.Get(srcVolumeID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get pv: %s", err.Error())
		}
		vgName := utils.GetVGNameFromCsiPV(pv)
		if vgName == "" {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get vgName of pv %s", pv.Name)
		}
		log.Infof("DeleteSnapshot: vg of snapshot %s is %s", snapshotID, vgName)

		nodeName := utils.GetNodeNameFromCsiPV(pv)
		if nodeName == "" {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get node name of pv %s", pv.Name)
		}
		conn, err := cs.getNodeConn(nodeName)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
		}
		defer conn.Close()

		// delete lvm snapshot
		var lvmName string
		if lvmName, err = conn.GetVolume(ctx, vgName, snapshotID); err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get lvm snapshot %s failed: %s", snapshotID, err.Error())
		}
		if lvmName != "" {
			log.Infof("DeleteSnapshot: lvm ro snapshot %s found, now deleting...", snapshotID)
			err := conn.DeleteSnapshot(ctx, vgName, snapshotID, true, nil)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "DeleteSnapshot: delete lvm snapshot %s failed: %s", snapshotID, err.Error())
			}
		} else {
			log.Infof("DeleteSnapshot: lvm snapshot %s in node %s not found, skip...", snapshotID, nodeName)
			// return immediately
			return &csi.DeleteSnapshotResponse{}, nil
		}
	} else {
		log.Infof("DeleteSnapshot: rw snapshot %s found, now deleting with restic...", snapshotID)
		s3URL := req.Secrets[localtype.S3_URL]
		s3AK := req.Secrets[localtype.S3_AK]
		s3SK := req.Secrets[localtype.S3_SK]
		// restic 快照逻辑
		// 此处 snapshot id 即为 tag
		r, err := restic.NewResticClient(s3URL, s3AK, s3SK, restic.GeneratePassword(), cs.options.kubeclient)
		if err != nil {
			return nil, err
		}
		out, err := r.DeleteDataByTag(srcVolumeID, snapshotID, true)
		if err != nil {
			return nil, err
		}
		log.Infof("delete restic data by ID %s successfully: %#v", snapshotID, out)
	}

	log.Infof("DeleteSnapshot: delete snapshot %s successfully", snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

// ControllerExpandVolume expand volume
func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	log.V(4).Infof("ControllerExpandVolume: called with args %+v", *req)

	// Step 1: get vgName
	volumeID := req.GetVolumeId()
	pv, err := cs.pvLister.Get(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get pv: %s", err.Error())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get node name of pv %s: %s", pv.Name, err.Error())
	}
	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get vgName of pv %s", pv.Name)
	}

	// Step 2: check whether the volume can be expanded
	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())

	// Step 3: get grpc client
	nodeName := utils.GetNodeNameFromCsiPV(pv)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get node name of pv %s", pv.Name)
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get grpc client at node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// Step 4: expand volume
	if err := conn.ExpandVolume(ctx, vgName, volumeID, uint64(volSizeBytes)); err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to expand lv %s/%s: %s", vgName, volumeID, err.Error())
	}

	log.Infof("ControllerExpandVolume: expand lvm %s/%s in node %s successfully", vgName, volumeID, nodeName)
	return &csi.ControllerExpandVolumeResponse{CapacityBytes: volSizeBytes, NodeExpansionRequired: true}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.V(4).Infof("ControllerPublishVolume: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.V(4).Infof("ControllerUnpublishVolume: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	log.V(4).Infof("GetCapacity: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log.V(4).Infof("ListVolumes: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	log.V(4).Infof("ListSnapshots: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	log.V(4).Infof("ValidateVolumeCapabilities: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if isValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported volume capabilities not %v", volCaps)
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	log.V(4).Infof("ControllerGetCapabilities: called with args %+v", *req)
	var caps []*csi.ControllerServiceCapability
	for _, cap := range ControllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (cs *controllerServer) newCreateSnapshotResponse(snapshotId, sourceVolumeId string, snapshotSize int64) (*csi.CreateSnapshotResponse, error) {
	ts := &timestamppb.Timestamp{
		Seconds: time.Now().Unix(),
		Nanos:   int32(time.Now().Nanosecond()),
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snapshotId,
			SourceVolumeId: sourceVolumeId,
			SizeBytes:      snapshotSize,
			ReadyToUse:     true,
			CreationTime:   ts,
		},
	}, nil
}

func (cs *controllerServer) getNodeConn(nodeSelected string) (client.Connection, error) {
	node, err := cs.nodeLister.Get(nodeSelected)
	if err != nil {
		return nil, err
	}
	addr, err := getNodeAddr(node, nodeSelected)
	if err != nil {
		log.Errorf("CreateVolume: Get node %s address with error: %s", nodeSelected, err.Error())
		return nil, err
	}
	conn, err := client.NewGrpcConnection(addr, time.Duration(cs.options.grpcConnectionTimeout*int(time.Second)))
	return conn, err
}

func getNodeAddr(node *v1.Node, nodeID string) (string, error) {
	ip, err := GetNodeIP(node, nodeID)
	if err != nil {
		return "", err
	}
	if ip.To4() == nil {
		// ipv6: https://stackoverflow.com/a/22752227
		return fmt.Sprintf("[%s]", ip.String()) + ":" + server.GetLvmdPort(), nil
	}
	return ip.String() + ":" + server.GetLvmdPort(), nil
}

// GetNodeIP get node address
func GetNodeIP(node *v1.Node, nodeID string) (net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[v1.NodeAddressType][]v1.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[v1.NodeInternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[v1.NodeExternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	return nil, fmt.Errorf("Node IP unknown; known addresses: %v", addresses)
}

type PVAllocatedInfo struct {
	VGName     string `json:"vgName"`
	DeviceName string `json:"deviceName"`
	VolumeType string `json:"volumeType"`
}

func (cs *controllerServer) scheduleLVMVolume(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	vgName := ""
	paraList := map[string]string{}
	if value, ok := parameters[VgNameTag]; ok {
		vgName = value
	}
	if vgName == "" {
		volumeInfo, err := cs.adapter.ScheduleVolume(string(pkg.VolumeTypeLVM), pvcName, pvcNameSpace, vgName, nodeSelected)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "lvm schedule with error "+err.Error())
		}
		if volumeInfo.VgName == "" || volumeInfo.Node == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Lvm Schedule finished, but get empty: %v", volumeInfo)
		}
		vgName = volumeInfo.VgName
	}
	paraList[VgNameTag] = vgName
	return paraList, nil
}

func (cs *controllerServer) scheduleMountpointVolume(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	paraList := map[string]string{}
	volumeInfo, err := cs.adapter.ScheduleVolume(string(pkg.VolumeTypeMountPoint), pvcName, pvcNameSpace, "", nodeSelected)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "lvm schedule with error "+err.Error())
	}
	if volumeInfo.Disk == "" {
		log.Errorf("mountpoint Schedule finished, but get empty Disk: %v", volumeInfo)
		return nil, status.Error(codes.InvalidArgument, "mountpoint schedule finish but Disk empty")
	}
	paraList[string(pkg.VolumeTypeMountPoint)] = volumeInfo.Disk
	return paraList, nil
}

func (cs *controllerServer) scheduleDeviceVolume(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	paraList := map[string]string{}
	volumeInfo, err := cs.adapter.ScheduleVolume(string(pkg.VolumeTypeDevice), pvcName, pvcNameSpace, "", nodeSelected)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "device schedule with error "+err.Error())
	}
	if volumeInfo.Device == "" {
		log.Errorf("Device Schedule finished, but get empty Disk: %v", volumeInfo)
		return nil, status.Error(codes.InvalidArgument, "Device schedule finish but Disk empty")
	}
	paraList[string(pkg.VolumeTypeDevice)] = volumeInfo.Device
	return paraList, nil
}

func validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	volName := req.GetName()
	if len(volName) == 0 {
		return fmt.Errorf("Volume name not provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return fmt.Errorf("Volume capabilities not provided")
	}

	if !isValidVolumeCapabilities(volCaps) {
		modes := utils.GetAccessModes(volCaps)
		stringModes := strings.Join(*modes, ", ")
		errString := "Volume capabilities " + stringModes + " not supported. Only AccessModes[ReadWriteOnce] supported."
		return fmt.Errorf(errString)
	}
	return nil
}

func validateDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	return nil
}

func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) bool {
	hasSupport := func(cap *csi.VolumeCapability) bool {
		for _, c := range VolumeCaps {
			if c == cap.AccessMode.GetMode() {
				return true
			}
		}
		return false
	}

	foundAll := true
	for _, c := range volCaps {
		if !hasSupport(c) {
			foundAll = false
		}
	}
	return foundAll
}

func getSnapshotInitialInfo(param map[string]string) (initialSize uint64, threshold float64, increaseSize uint64, err error) {
	initialSize = localtype.DefaultSnapshotInitialSize
	threshold = localtype.DefaultSnapshotThreshold
	increaseSize = localtype.DefaultSnapshotExpansionSize
	err = nil

	// Step 1: get snapshot initial size
	if str, exist := param[localtype.ParamSnapshotInitialSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			return 0, 0, 0, status.Errorf(codes.Internal, "getSnapshotInitialInfo: get initialSize from snapshot annotation failed: %s", err.Error())
		}
		initialSize = uint64(size)
	}
	// Step 2: get snapshot expand threshold
	if str, exist := param[localtype.ParamSnapshotThreshold]; exist {
		str = strings.ReplaceAll(str, "%", "")
		thr, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, 0, 0, status.Errorf(codes.Internal, "getSnapshotInitialInfo: parse float failed: %s", err.Error())
		}
		threshold = thr / 100
	}
	// Step 3: get snapshot increase size
	if str, exist := param[localtype.ParamSnapshotExpansionSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			return 0, 0, 0, status.Errorf(codes.Internal, "getSnapshotInitialInfo: get increase size from snapshot annotation failed: %s", err.Error())
		}
		increaseSize = uint64(size)
	}
	log.Infof("getSnapshotInitialInfo: initialSize(%d), threshold(%f), increaseSize(%d)", initialSize, threshold, increaseSize)
	return
}
