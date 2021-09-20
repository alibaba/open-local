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
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/adapter"
	"github.com/alibaba/open-local/pkg/csi/client"
	"github.com/alibaba/open-local/pkg/csi/server"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csilib "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/docker/go-units"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1beta1"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// VolumeTypeKey volume type key words
	VolumeTypeKey = "volumeType"
	// LvmVolumeType lvm volume type
	LvmVolumeType = "LVM"
	// MountPointType type
	MountPointType = "MountPoint"
	// DeviceVolumeType type
	DeviceVolumeType = "Device"
	// PvcNameTag in annotations
	PvcNameTag = "csi.storage.k8s.io/pvc/name"
	// PvcNsTag in annotations
	PvcNsTag = "csi.storage.k8s.io/pvc/namespace"
	// NodeSchedueTag in annotations
	NodeSchedueTag = "volume.kubernetes.io/selected-node"
	// StorageSchedueTag in annotations
	StorageSchedueTag = "volume.kubernetes.io/selected-storage"
	// LastAppliyAnnotationTag tag
	LastAppliyAnnotationTag = "kubectl.kubernetes.io/last-applied-configuration"
	// CsiProvisionerIdentity tag
	CsiProvisionerIdentity = "storage.kubernetes.io/csiProvisionerIdentity"
	// CsiProvisionerTag tag
	CsiProvisionerTag = "volume.beta.kubernetes.io/storage-provisioner"
	// StripingType striping type
	StripingType = "striping"
	// connection timeout
	connectTimeout = 3 * time.Second

	// TopologyNodeKey define host name of node
	TopologyNodeKey = "kubernetes.io/hostname"
	// TopologyYodaNodeKey define host name of node
	TopologyYodaNodeKey = "topology.yodaplugin.csi.alibabacloud.com/hostname"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	client     kubernetes.Interface
	snapclient snapshot.Interface
	driverName string
}

var supportVolumeTypes = []string{LvmVolumeType, MountPointType, DeviceVolumeType}

func newControllerServer(d *csicommon.CSIDriver) *controllerServer {
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}
	snapClient, err := snapshot.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building snapshot clientset: %s", err.Error())
	}

	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		client:                  kubeClient,
		snapclient:              snapClient,
	}
}

// the map of req.Name and csi.Volume
var createdVolumeMap = map[string]*csi.Volume{}

// CreateVolume csi interface
func (cs *controllerServer) CreateVolume(ctx context.Context, req *csilib.CreateVolumeRequest) (*csilib.CreateVolumeResponse, error) {
	// Step 1: check
	if err := cs.Driver.ValidateControllerServiceRequest(csilib.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		log.Errorf("CreateVolume: Invalid create local volume req: %v", req)
		return nil, err
	}
	if req.Name == "" {
		log.Errorf("CreateVolume: local volume Name is empty")
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: local Volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		log.Errorf("CreateVolume: local Volume Capabilities cannot be empty")
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}
	if value, ok := createdVolumeMap[req.Name]; ok {
		log.Infof("CreateVolume: local volume already be created, pvName: %s, VolumeId: %s", req.Name, value.VolumeId)
		return &csi.CreateVolumeResponse{Volume: value}, nil
	}
	// Step 2: get necessary info
	volumeID := req.GetName()
	pvcName, pvcNameSpace, volumeType, nodeSelected, storageSelected := "", "", "", "", ""
	var err error
	parameters := req.GetParameters()
	if value, ok := parameters[VolumeTypeKey]; ok {
		for _, supportVolType := range supportVolumeTypes {
			if supportVolType == value {
				volumeType = value
			}
		}
	}
	if volumeType == "" {
		log.Errorf("CreateVolume: Create volume %s with error volumeType %v", volumeID, parameters)
		return nil, status.Error(codes.InvalidArgument, "Local driver only support LVM/MountPoint/Device/PmemDirect/PmemQuotaPath volume type, no "+volumeType)
	}
	if value, ok := parameters[PvcNameTag]; ok {
		pvcName = value
	}
	if value, ok := parameters[PvcNsTag]; ok {
		pvcNameSpace = value
	}

	if nodeSelected, err = getNodeName(cs.client, pvcName, pvcNameSpace); err != nil {
		return nil, status.Errorf(codes.Internal, "get node name failed: %s", err.Error())
	}
	if value, ok := parameters[StorageSchedueTag]; ok {
		storageSelected = value
	}
	log.Infof("Starting to Create %s volume %s with: pvcName(%s), pvcNameSpace(%s), nodeSelected(%s), storageSelected(%s)", volumeType, volumeID, pvcName, pvcNameSpace, nodeSelected, storageSelected)

	// Step 3: Storage schedule
	isSnapshot := false
	paraList := map[string]string{}
	switch volumeType {
	case LvmVolumeType:
		var err error
		// check volume content source is snapshot
		if volumeSource := req.GetVolumeContentSource(); volumeSource != nil {
			// validate
			if _, ok := volumeSource.GetType().(*csi.VolumeContentSource_Snapshot); !ok {
				log.Errorf("CreateVolume: unsupported volumeContentSource type")
				return nil, status.Error(codes.InvalidArgument, "CreateVolume: unsupported volumeContentSource type")
			}
			log.Infof("CreateVolume: kind of volume %s is snapshot", volumeID)
			// get snapshot name
			sourceSnapshot := volumeSource.GetSnapshot()
			if sourceSnapshot == nil {
				log.Errorf("CreateVolume: error retrieving snapshot from the volumeContentSource")
				return nil, status.Error(codes.InvalidArgument, "CreateVolume: error retrieving snapshot from the volumeContentSource")
			}
			snapshotID := sourceSnapshot.GetSnapshotId()
			log.Infof("CreateVolume: snapshotID is %s", snapshotID)
			// get src volume ID
			snapContent, err := getVolumeSnapshotContent(cs.snapclient, snapshotID)
			if err != nil {
				log.Errorf("CreateVolume: get snapshot content failed: %s", err.Error())
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: get snapshot content failed: %s", err.Error())
			}
			srcVolumeID := *snapContent.Spec.Source.VolumeHandle
			log.Infof("CreateVolume: srcVolumeID is %s", srcVolumeID)
			// check if is readonly snapshot
			class, err := getVolumeSnapshotClass(cs.snapclient, *snapContent.Spec.VolumeSnapshotClassName)
			if err != nil {
				log.Errorf("get snapshot class failed: %s", err.Error())
				return nil, status.Errorf(codes.InvalidArgument, "get snapshot class failed: %s", err.Error())
			}
			ro, exist := class.Parameters[localtype.ParamSnapshotReadonly]
			if !exist || ro != "true" {
				log.Errorf("CreateVolume: only support readonly snapshot now, you must set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
				return nil, status.Errorf(codes.Unimplemented, "CreateVolume: only support readonly snapshot now, you must set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
			}
			// get node name and vg name from src volume
			nodeSelected, storageSelected, _, err := getPvSpec(cs.client, srcVolumeID, cs.driverName)
			if err != nil {
				log.Errorf("CreateVolume: get pv spec failed: %s", err.Error())
				return nil, status.Errorf(codes.Internal, "CreateVolume: get pv spec failed: %s", err.Error())
			}
			// set paraList for NodeStageVolume and NodePublishVolume
			parameters[NodeSchedueTag] = nodeSelected
			paraList[VgNameTag] = storageSelected
			paraList[localtype.ParamSnapshotName] = snapshotID
			paraList[localtype.ParamSnapshotReadonly] = "true"
			isSnapshot = true
			log.Infof("CreateVolume: get snapshot volume %s info: node(%s) vg(%s)", volumeID, nodeSelected, storageSelected)
			// break switch
			break
		}

		// Node and Storage have been scheduled (select volumeGroup)
		if storageSelected != "" && nodeSelected != "" {
			paraList, err = lvmScheduled(storageSelected, parameters)
			if err != nil {
				log.Errorf("CreateVolume: lvm all scheduled volume %s with error: %s", volumeID, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse lvm all schedule info error "+err.Error())
			}
			log.Infof("CreateVolume: lvm scheduled with %s, %s", nodeSelected, storageSelected)
		} else if nodeSelected != "" {
			paraList, err = lvmPartScheduled(nodeSelected, pvcName, pvcNameSpace, parameters)
			if err != nil {
				log.Errorf("CreateVolume: lvm part scheduled volume %s with error: %s", volumeID, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse lvm part schedule info error "+err.Error())
			}
			if value, ok := paraList[VgNameTag]; ok && value != "" {
				storageSelected = value
			}
			log.Infof("CreateVolume: lvm part scheduled with %s, %s", nodeSelected, storageSelected)
		} else {
			nodeID := ""
			nodeID, paraList, err = lvmNoScheduled(parameters)
			if err != nil {
				log.Errorf("CreateVolume: lvm No scheduled volume %s with error: %s", volumeID, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse lvm schedule info error "+err.Error())
			}
			nodeSelected = nodeID
			if value, ok := paraList[VgNameTag]; ok && value != "" {
				storageSelected = value
			}
			log.Infof("CreateVolume: lvm no scheduled with %s, %s", nodeSelected, storageSelected)
		}

		// if vgName configed in storageclass, use it first;
		if value, ok := paraList[VgNameTag]; ok && value != "" {
			storageSelected = value
		}

		// Volume Options
		options := &client.LVMOptions{}
		options.Name = req.Name
		options.VolumeGroup = storageSelected
		if value, ok := parameters[LvmTypeTag]; ok && value == StripingType {
			options.Striping = true
		}
		options.Size = uint64(req.GetCapacityRange().GetRequiredBytes())

		if nodeSelected != "" && storageSelected != "" {
			conn, err := cs.getNodeConn(nodeSelected)
			if err != nil {
				log.Errorf("CreateVolume: New lvm %s Connection to node %s with error: %s", req.Name, nodeSelected, err.Error())
				return nil, err
			}
			defer conn.Close()
			if lvmName, err := conn.GetLvm(ctx, storageSelected, volumeID); err == nil && lvmName == "" {
				outstr, err := conn.CreateLvm(ctx, options)
				if err != nil {
					log.Errorf("CreateVolume: Create lvm %s/%s, options: %v with error: %s", storageSelected, volumeID, options, err.Error())
					return nil, errors.New("Create Lvm with error " + err.Error())
				}
				log.Infof("CreateLvm: Successful Create lvm %s/%s in node %s with response %s", storageSelected, volumeID, nodeSelected, outstr)
			} else if err != nil {
				log.Errorf("CreateVolume: Get lvm %s from node %s with error: %s", req.Name, nodeSelected, err.Error())
				return nil, err
			} else {
				log.Infof("CreateVolume: lvm volume already created %s at node %s", req.Name, nodeSelected)
			}
		}
	case MountPointType:
		var err error
		// Node and Storage have been scheduled
		if storageSelected != "" && nodeSelected != "" {
			paraList, err = mountpointScheduled(storageSelected, parameters)
			if err != nil {
				log.Errorf("CreateVolume: create mountpoint volume %s/%s at node %s error: %s", storageSelected, req.Name, nodeSelected, err.Error())
				return nil, status.Error(codes.InvalidArgument, "CreateVolume: Parse mountpoint all scheduled info error "+err.Error())
			}
		} else if nodeSelected != "" {
			paraList, err = mountpointPartScheduled(nodeSelected, pvcName, pvcNameSpace, parameters)
			if err != nil {
				log.Errorf("CreateVolume: part schedule mountpoint volume %s at node %s error: %s", req.Name, nodeSelected, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse mountpoint part schedule info error "+err.Error())
			}
		} else {
			nodeID := ""
			nodeID, paraList, err = mountpointNoScheduled(parameters)
			if err != nil {
				log.Errorf("CreateVolume: schedule mountpoint volume %s error: %s", req.Name, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse mountpoint schedule info error "+err.Error())
			}
			nodeSelected = nodeID
		}
		log.Infof("CreateVolume: Successful create mountpoint volume %s/%s at node %s", storageSelected, req.Name, nodeSelected)
	case DeviceVolumeType:
		var err error
		// Node and Storage have been scheduled
		if storageSelected != "" && nodeSelected != "" {
			paraList, err = deviceScheduled(storageSelected, parameters)
			if err != nil {
				log.Errorf("CreateVolume: create device volume %s/%s at node %s error: %s", storageSelected, req.Name, nodeSelected, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse Device all scheduled info error "+err.Error())
			}
		} else if nodeSelected != "" {
			paraList, err = devicePartScheduled(nodeSelected, pvcName, pvcNameSpace, parameters)
			if err != nil {
				log.Errorf("CreateVolume: part schedule device volume %s at node %s error: %s", req.Name, nodeSelected, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse Device part schedule info error "+err.Error())
			}
		} else {
			nodeID := ""
			nodeID, paraList, err = deviceNoScheduled(parameters)
			if err != nil {
				log.Errorf("CreateVolume: schedule device volume %s error: %s", req.Name, err.Error())
				return nil, status.Error(codes.InvalidArgument, "Parse Device schedule info error "+err.Error())
			}
			nodeSelected = nodeID
		}
		log.Infof("CreateVolume: Successful create device volume %s/%s at node %s", storageSelected, req.Name, nodeSelected)
	default:
		log.Errorf("CreateVolume: Create with no support volume type %s", volumeType)
		return nil, status.Error(codes.InvalidArgument, "Create with no support type "+volumeType)
	}
	// Append necessary parameters
	for key, value := range paraList {
		parameters[key] = value
	}
	// remove not necessary labels
	for key := range parameters {
		if key == LastAppliyAnnotationTag {
			delete(parameters, key)
		} else if key == CsiProvisionerTag {
			delete(parameters, key)
		} else if key == CsiProvisionerIdentity {
			delete(parameters, key)
		}
	}

	var response *csilib.CreateVolumeResponse
	if nodeSelected == "" {
		response = &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      volumeID,
				CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
				VolumeContext: parameters,
			},
		}
	} else {
		parameters[NodeSchedueTag] = nodeSelected
		response = &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      volumeID,
				CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
				VolumeContext: parameters,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{
							TopologyNodeKey: nodeSelected,
						},
					},
				},
			},
		}
	}

	// add volume content source info if needed
	if isSnapshot {
		response.Volume.ContentSource = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: paraList[localtype.ParamSnapshotName],
				},
			},
		}
	}

	createdVolumeMap[req.Name] = response.Volume
	log.Infof("Success create Volume: %s, Size: %d", volumeID, req.GetCapacityRange().GetRequiredBytes())
	return response, nil
}

func (server *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log.Infof("DeleteVolume: deleting local volume %s", req.GetVolumeId())
	volumeID := req.GetVolumeId()
	nodeName, vgName, pvObj, err := getPvSpec(server.client, volumeID, server.driverName)
	if err != nil {
		log.Errorf("DeleteVolume: get pv spec %s with error: %s", volumeID, err.Error())
		return nil, err
	}
	volumeType := ""
	if value, ok := pvObj.Spec.CSI.VolumeAttributes[VolumeTypeKey]; ok {
		volumeType = value
	}

	switch volumeType {
	case LvmVolumeType:
		// check volume content source is snapshot
		var isSnapshot bool = false
		var isSnapshotReadOnly bool = false
		if pvObj.Spec.CSI != nil {
			attributes := pvObj.Spec.CSI.VolumeAttributes
			if value, exist := attributes[localtype.ParamSnapshotName]; exist && value != "" {
				isSnapshot = true
			}
			if value, exist := attributes[localtype.ParamSnapshotReadonly]; exist && value == "true" {
				isSnapshotReadOnly = true
			}
		}
		if isSnapshot {
			if isSnapshotReadOnly {
				log.Infof("DeleteVolume: volume %s is ro snapshot volume, skip delete lv...", volumeID)
				// break switch
				break
			} else {
				log.Errorf("DeleteVolume: only support readonly snapshot now, you must set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
				return nil, status.Errorf(codes.Unimplemented, "DeleteVolume: only support readonly snapshot now, you must set %s parameter in volumesnapshotclass", localtype.ParamSnapshotReadonly)
			}
		}

		if nodeName != "" {
			conn, err := server.getNodeConn(nodeName)
			if err != nil {
				log.Errorf("DeleteVolume: New lvm %s Connection at node %s with error: %s", req.GetVolumeId(), nodeName, err.Error())
				return nil, err
			}
			defer conn.Close()
			if lvmName, err := conn.GetLvm(ctx, vgName, volumeID); err == nil && lvmName != "" {
				if err := conn.DeleteLvm(ctx, vgName, volumeID); err != nil {
					log.Errorf("DeleteVolume: Remove lvm %s/%s at node %s with error: %s", vgName, volumeID, nodeName, err.Error())
					return nil, errors.New("DeleteVolume: Remove Lvm " + volumeID + " with error " + err.Error())
				}
				log.Infof("DeleteLvm: Successful Delete lvm %s/%s at node %s", vgName, volumeID, nodeName)
			} else if err == nil && lvmName == "" {
				log.Infof("DeleteVolume: get lvm empty, skip deleting %s", volumeID)
			} else if err != nil && strings.Contains(err.Error(), "Failed to find logical volume") {
				log.Infof("DeleteVolume: lvm volume not found, skip deleting %s", volumeID)
			} else if err != nil && strings.Contains(err.Error(), "Volume group \""+vgName+"\" not found") {
				log.Infof("DeleteVolume: Volume group not found, skip deleting %s", volumeID)
			} else {
				log.Errorf("DeleteVolume: Get lvm for %s with error: %s", req.GetVolumeId(), err.Error())
				return nil, err
			}
		} else {
			log.Infof("DeleteVolume: delete local volume %s with node empty", volumeID)
		}

	case MountPointType:
		if pvObj.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimDelete {
			if pvObj.Spec.NodeAffinity == nil {
				log.Errorf("DeleteVolume: Get Lvm Spec for volume %s, with nil nodeAffinity", volumeID)
				return nil, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil nodeAffinity")
			}
			if pvObj.Spec.NodeAffinity.Required == nil || len(pvObj.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
				log.Errorf("DeleteVolume: Get Lvm Spec for volume %s, with nil Required", volumeID)
				return nil, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil Required")
			}
			if len(pvObj.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions) == 0 {
				log.Errorf("DeleteVolume: Get Lvm Spec for volume %s, with nil MatchExpressions", volumeID)
				return nil, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil MatchExpressions")
			}
			key := pvObj.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key
			if key != TopologyNodeKey && key != TopologyYodaNodeKey {
				log.Errorf("DeleteVolume: Get Lvm Spec for volume %s, with key %s", volumeID, key)
				return nil, errors.New("Get Lvm Spec for volume " + volumeID + ", with key" + key)
			}
			nodes := pvObj.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values
			if len(nodes) == 0 {
				log.Errorf("DeleteVolume: Get MountPoint Spec for volume %s, with empty nodes", volumeID)
				return nil, errors.New("MountPoint Pv is illegal, No node info")
			}
			nodeName := nodes[0]
			conn, err := server.getNodeConn(nodeName)
			if err != nil {
				log.Errorf("DeleteVolume: New mountpoint %s Connection error: %s", req.GetVolumeId(), err.Error())
				return nil, err
			}
			defer conn.Close()
			path := ""
			if value, ok := pvObj.Spec.CSI.VolumeAttributes[MountPointType]; ok {
				path = value
			}
			if path == "" {
				log.Errorf("DeleteVolume: Get MountPoint Path for volume %s, with empty", volumeID)
				return nil, errors.New("MountPoint Path is empty")
			}
			if err := conn.CleanPath(ctx, path); err != nil {
				log.Errorf("DeleteVolume: Remove mountpoint for %s with error: %s", req.GetVolumeId(), err.Error())
				return nil, errors.New("DeleteVolume: Delete mountpoint Failed: " + err.Error())
			}
		}
		log.Infof("DeleteVolume: successful delete MountPoint volume(%s)...", volumeID)
	case DeviceVolumeType:
		log.Infof("DeleteVolume: successful delete Device volume(%s)...", volumeID)
	default:
		log.Errorf("DeleteVolume: volumeType %s not supported %s", volumeType, volumeID)
		return nil, status.Error(codes.InvalidArgument, "Local driver only support LVM volume type, no "+volumeType)
	}
	delete(createdVolumeMap, req.VolumeId)
	log.Infof("DeleteVolume: successful delete local volume %s", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// CreateSnapshot create lvm snapshot
func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log.Debugf("Starting Create Snapshot %s with response: %v", req.Name, req)
	// Step 1: check request
	snapshotName := req.GetName()
	if len(snapshotName) == 0 {
		log.Error("CreateSnapshot: snapshot name not provided")
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot: snapshot name not provided")
	}
	volumeID := req.GetSourceVolumeId()
	if len(volumeID) == 0 {
		log.Error("CreateSnapshot: snapshot volume source ID not provided")
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot: snapshot volume source ID not provided")
	}

	// Step 2: get snapshot initial size from parameter
	initialSize, _, _, err := getSnapshotInitialInfo(req.Parameters)
	if err != nil {
		log.Errorf("CreateSnapshot: get snapshot %s initial info error: %s", req.Name, err.Error())
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: get snapshot %s initial info error: %s", req.Name, err.Error())
	}

	// Step 3: get nodeName and vgName
	nodeName, vgName, pv, err := getPvSpec(cs.client, volumeID, cs.driverName)
	if err != nil {
		log.Errorf("CreateSnapshot: get pv %s error: %s", volumeID, err.Error())
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: get pv %s error: %s", volumeID, err.Error())
	}
	log.Infof("CreateSnapshot: snapshot %s is in %s, whose vg is %s", snapshotName, nodeName, vgName)

	// Step 4: update initialSize if initialSize is bigger than pv request size
	pvSize, _ := pv.Spec.Capacity.Storage().AsInt64()
	if pvSize < int64(initialSize) {
		initialSize = uint64(pvSize)
	}

	// Step 5: get grpc client
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		log.Errorf("CreateSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// Step 6: create lvm snapshot
	var lvmName string
	if lvmName, err = conn.GetLvm(ctx, vgName, snapshotName); err != nil {
		log.Errorf("CreateSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
	}
	if lvmName == "" {
		_, err := conn.CreateSnapshot(ctx, vgName, snapshotName, volumeID, initialSize)
		if err != nil {
			log.Errorf("CreateSnapshot: create lvm snapshot %s failed: %s", snapshotName, err.Error())
			return nil, status.Errorf(codes.Internal, "CreateSnapshot: create lvm snapshot %s failed: %s", snapshotName, err.Error())
		}
		log.Infof("CreateSnapshot: create snapshot %s successfully", snapshotName)
	} else {
		log.Infof("CreateSnapshot: lvm snapshot %s in node %s already exists", snapshotName, nodeName)
	}
	return cs.newCreateSnapshotResponse(req)
}

// DeleteSnapshot delete lvm snapshot
func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	log.Infof("Starting Delete Snapshot %s with response: %v", req.SnapshotId, req)
	// Step 1: check req
	// snapshotName is name of snapshot lv
	snapshotName := req.GetSnapshotId()
	if len(snapshotName) == 0 {
		log.Error("DeleteSnapshot: Snapshot ID not provided")
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot: Snapshot ID not provided")
	}

	// Step 2: get volumeID from snapshot
	snapContent, err := getVolumeSnapshotContent(cs.snapclient, snapshotName)
	if err != nil {
		log.Errorf("DeleteSnapshot: get snapContent %s error: %s", snapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get snapContent %s error: %s", snapshotName, err.Error())
	}
	volumeID := *snapContent.Spec.Source.VolumeHandle

	// Step 3: get nodeName and vgName
	nodeName, vgName, _, err := getPvSpec(cs.client, volumeID, cs.driverName)
	if err != nil {
		log.Errorf("DeleteSnapshot: get pv %s error: %s", volumeID, err.Error())
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get pv %s error: %s", volumeID, err.Error())
	}
	log.Infof("DeleteSnapshot: snapshot %s is in %s, whose vg is %s", snapshotName, nodeName, vgName)

	// Step 4: get grpc client
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		log.Errorf("DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// Step 5: delete lvm snapshot
	var lvmName string
	if lvmName, err = conn.GetLvm(ctx, vgName, snapshotName); err != nil {
		log.Errorf("DeleteSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
	}
	if lvmName != "" {
		err := conn.DeleteSnapshot(ctx, vgName, snapshotName)
		if err != nil {
			log.Errorf("DeleteSnapshot: delete lvm snapshot %s failed: %s", snapshotName, err.Error())
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: delete lvm snapshot %s failed: %s", snapshotName, err.Error())
		}
	} else {
		log.Infof("DeleteSnapshot: lvm snapshot %s in node %s not found, skip...", snapshotName, nodeName)
		// return immediately
		return &csi.DeleteSnapshotResponse{}, nil
	}

	log.Infof("DeleteSnapshot: delete snapshot %s successfully", snapshotName)
	return &csi.DeleteSnapshotResponse{}, nil
}

// ControllerExpandVolume expand volume
func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	log.Infof("ControllerExpandVolume: Starting Expand Volume %s with response: %v", req.VolumeId, req)

	// Step 1: get nodeName
	volumeID := req.GetVolumeId()
	nodeName, vgName, pvObj, err := getPvSpec(cs.client, volumeID, cs.driverName)
	if err != nil {
		log.Errorf("ControllerExpandVolume: get pv %s error: %s", volumeID, err.Error())
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: get pv %s error: %s", volumeID, err.Error())
	}

	// Step 2: check whether the volume can be expanded
	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	volSizeGB := int((volSizeBytes + 1024*1024*1024 - 1) / (1024 * 1024 * 1024))
	attributes := pvObj.Spec.CSI.VolumeAttributes
	pvcName, pvcNameSpace := "", ""
	if value, ok := attributes[PvcNameTag]; ok {
		pvcName = value
	}
	if value, ok := attributes[PvcNsTag]; ok {
		pvcNameSpace = value
	}
	if err := adapter.ExpandVolume(pvcNameSpace, pvcName, volSizeGB); err != nil {
		log.Errorf("ControllerExpandVolume: expand volume %s to size %d meet error: %v", volumeID, volSizeGB, err)
		return nil, errors.New("ControllerExpandVolume: expand volume error " + err.Error())
	}

	// Step 3: get grpc client
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		log.Errorf("ControllerExpandVolume: get grpc client at node %s error: %s", nodeName, err.Error())
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: get grpc client at node %s error: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// Step 4: expand volume
	if err := conn.ExpandLvm(ctx, vgName, volumeID, uint64(volSizeBytes)); err != nil {
		log.Errorf("ControllerExpandVolume: expand lvm %s/%s with error: %s", vgName, volumeID, err.Error())
		return nil, errors.New("Create Lvm with error " + err.Error())
	}

	log.Infof("ControllerExpandVolume: Successful expand lvm %s/%s in node %s", vgName, volumeID, nodeName)
	return &csi.ControllerExpandVolumeResponse{CapacityBytes: volSizeBytes, NodeExpansionRequired: true}, nil
}

// ControllerPublishVolume csi interface
func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Infof("ControllerPublishVolume is called, do nothing by now: %s", req.VolumeId)
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// ControllerUnpublishVolume csi interface
func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Infof("ControllerUnpublishVolume is called, do nothing by now: %s", req.VolumeId)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *controllerServer) newCreateSnapshotResponse(req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	_, _, pv, err := getPvSpec(cs.client, req.GetSourceVolumeId(), cs.driverName)
	if err != nil {
		log.Errorf("newCreateSnapshotResponse: get pv %s error: %s", req.GetSourceVolumeId(), err.Error())
		return nil, status.Errorf(codes.Internal, "newCreateSnapshotResponse: get pv %s error: %s", req.GetSourceVolumeId(), err.Error())
	}

	ts := &timestamppb.Timestamp{
		Seconds: time.Now().Unix(),
		Nanos:   int32(time.Now().Nanosecond()),
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     req.Name,
			SourceVolumeId: req.SourceVolumeId,
			SizeBytes:      int64(pv.Size()),
			ReadyToUse:     true,
			CreationTime:   ts,
		},
	}, nil
}

func (cs *controllerServer) getNodeConn(nodeSelected string) (client.Connection, error) {
	addr, err := getNodeAddr(cs.client, nodeSelected)
	if err != nil {
		log.Errorf("CreateVolume: Get node %s address with error: %s", nodeSelected, err.Error())
		return nil, err
	}
	conn, err := client.NewGrpcConnection(addr, connectTimeout)
	return conn, err
}

func getNodeName(client kubernetes.Interface, pvcName string, pvcNameSpace string) (nodeName string, err error) {
	pvc, err := client.CoreV1().PersistentVolumeClaims(pvcNameSpace).Get(context.Background(), pvcName, metav1.GetOptions{})

	if err != nil {
		return "", err
	}

	nodeName, exist := pvc.Annotations[NodeSchedueTag]
	if !exist {
		return "", fmt.Errorf("no annotation %s found in pvc %s/%s", NodeSchedueTag, pvcNameSpace, pvcName)
	}

	return nodeName, nil
}

func getVolumeSnapshotClass(snapclient snapshot.Interface, className string) (*snapshotapi.VolumeSnapshotClass, error) {
	return snapclient.SnapshotV1beta1().VolumeSnapshotClasses().Get(context.Background(), className, metav1.GetOptions{})
}

func getVolumeSnapshotContent(snapclient snapshot.Interface, snapshotContentName string) (*snapshotapi.VolumeSnapshotContent, error) {
	// Step 1: get yoda snapshot prefix
	prefix := os.Getenv(localtype.EnvSnapshotPrefix)
	if prefix == "" {
		prefix = localtype.DefaultSnapshotPrefix
	}
	// Step 2: get snapshot content api
	return snapclient.SnapshotV1beta1().VolumeSnapshotContents().Get(context.TODO(), strings.Replace(snapshotContentName, prefix, "snapcontent", 1), metav1.GetOptions{})
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

func getNodeAddr(client kubernetes.Interface, node string) (string, error) {
	ip, err := GetNodeIP(client, node)
	if err != nil {
		return "", err
	}
	return ip.String() + ":" + server.GetLvmdPort(), nil
}

// GetNodeIP get node address
func GetNodeIP(client kubernetes.Interface, nodeID string) (net.IP, error) {
	node, err := client.CoreV1().Nodes().Get(context.Background(), nodeID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
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

func getPvSpec(client kubernetes.Interface, volumeID, driverName string) (string, string, *v1.PersistentVolume, error) {
	pv, err := getPvObj(client, volumeID)
	if err != nil {
		log.Errorf("Get Lvm Spec for volume %s, error with %v", volumeID, err)
		return "", "", nil, err
	}
	if pv.Spec.NodeAffinity == nil {
		log.Errorf("Get Lvm Spec for volume %s, with nil nodeAffinity", volumeID)
		return "", "", pv, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil nodeAffinity")
	}
	if pv.Spec.NodeAffinity.Required == nil || len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
		log.Errorf("Get Lvm Spec for volume %s, with nil Required", volumeID)
		return "", "", pv, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil Required")
	}
	if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions) == 0 {
		log.Errorf("Get Lvm Spec for volume %s, with nil MatchExpressions", volumeID)
		return "", "", pv, errors.New("Get Lvm Spec for volume " + volumeID + ", with nil MatchExpressions")
	}
	key := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key
	if key != TopologyNodeKey && key != TopologyYodaNodeKey {
		log.Errorf("Get Lvm Spec for volume %s, with key %s", volumeID, key)
		return "", "", pv, errors.New("Get Lvm Spec for volume " + volumeID + ", with key" + key)
	}
	nodes := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values
	if len(nodes) == 0 {
		log.Errorf("Get Lvm Spec for volume %s, with empty nodes", volumeID)
		return "", "", pv, errors.New("Get Lvm Spec for volume " + volumeID + ", with empty nodes")
	}
	vgName := ""
	if value, ok := pv.Spec.CSI.VolumeAttributes["vgName"]; ok {
		vgName = value
	}

	log.Debugf("Get Lvm Spec for volume %s, with VgName %s, Node %s", volumeID, pv.Spec.CSI.VolumeAttributes["vgName"], nodes[0])
	return nodes[0], vgName, pv, nil
}

func lvmScheduled(storageSelected string, parameters map[string]string) (map[string]string, error) {
	vgName := ""
	paraList := map[string]string{}
	if storageSelected != "" {
		storageMap := map[string]string{}
		err := json.Unmarshal([]byte(storageSelected), &storageMap)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "Scheduler provide error storage format: "+err.Error())
		}
		if value, ok := storageMap["VolumeGroup"]; ok {
			paraList[VgNameTag] = value
			vgName = value
		}
	}
	if value, ok := parameters[VgNameTag]; ok && value != "" {
		if vgName != "" && value != vgName {
			return nil, status.Error(codes.InvalidArgument, "Storage Schedule is not expected "+value+vgName)
		}
	}
	if vgName == "" {
		return nil, status.Error(codes.InvalidArgument, "Node/Storage Schedule failed "+vgName)
	}
	return paraList, nil
}

func lvmPartScheduled(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	vgName := ""
	paraList := map[string]string{}
	if value, ok := parameters[VgNameTag]; ok {
		vgName = value
	}
	if vgName == "" {
		volumeInfo, err := adapter.ScheduleVolume(LvmVolumeType, pvcName, pvcNameSpace, vgName, nodeSelected)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "lvm schedule with error "+err.Error())
		}
		if volumeInfo.VgName == "" || volumeInfo.Node == "" {
			log.Errorf("Lvm Schedule finished, but get empty: %v", volumeInfo)
			return nil, status.Error(codes.InvalidArgument, "lvm schedule finish but vgName/Node empty")
		}
		vgName = volumeInfo.VgName
	}
	paraList[VgNameTag] = vgName
	return paraList, nil
}

func lvmNoScheduled(parameters map[string]string) (string, map[string]string, error) {
	paraList := map[string]string{}
	return "", paraList, nil
}

func mountpointScheduled(storageSelected string, parameters map[string]string) (map[string]string, error) {
	mountpoint := ""
	paraList := map[string]string{}
	if storageSelected != "" {
		storageMap := map[string]string{}
		err := json.Unmarshal([]byte(storageSelected), &storageMap)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "Scheduler provide error storage format: "+err.Error())
		}
		if value, ok := storageMap[MountPointType]; ok {
			paraList[MountPointType] = value
			mountpoint = value
		}
	}
	if mountpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "Mountpoint Schedule failed "+mountpoint)
	}
	return paraList, nil
}

func mountpointPartScheduled(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	paraList := map[string]string{}
	volumeInfo, err := adapter.ScheduleVolume(MountPointType, pvcName, pvcNameSpace, "", nodeSelected)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "lvm schedule with error "+err.Error())
	}
	if volumeInfo.Disk == "" {
		log.Errorf("mountpoint Schedule finished, but get empty Disk: %v", volumeInfo)
		return nil, status.Error(codes.InvalidArgument, "mountpoint schedule finish but Disk empty")
	}
	paraList[MountPointType] = volumeInfo.Disk
	return paraList, nil
}

func mountpointNoScheduled(parameters map[string]string) (string, map[string]string, error) {
	paraList := map[string]string{}
	return "", paraList, nil
}

func deviceScheduled(storageSelected string, parameters map[string]string) (map[string]string, error) {
	device := ""
	paraList := map[string]string{}
	if storageSelected != "" {
		storageMap := map[string]string{}
		err := json.Unmarshal([]byte(storageSelected), &storageMap)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "Scheduler provide error storage format: "+err.Error())
		}
		if value, ok := storageMap[DeviceVolumeType]; ok {
			paraList[DeviceVolumeType] = value
			device = value
		}
	}
	if device == "" {
		return nil, status.Error(codes.InvalidArgument, "Device Schedule failed "+device)
	}
	return paraList, nil
}

func devicePartScheduled(nodeSelected, pvcName, pvcNameSpace string, parameters map[string]string) (map[string]string, error) {
	paraList := map[string]string{}
	volumeInfo, err := adapter.ScheduleVolume(DeviceVolumeType, pvcName, pvcNameSpace, "", nodeSelected)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "device schedule with error "+err.Error())
	}
	if volumeInfo.Disk == "" {
		log.Errorf("Device Schedule finished, but get empty Disk: %v", volumeInfo)
		return nil, status.Error(codes.InvalidArgument, "Device schedule finish but Disk empty")
	}
	paraList[DeviceVolumeType] = volumeInfo.Disk
	return paraList, nil
}

func deviceNoScheduled(parameters map[string]string) (string, map[string]string, error) {
	paraList := map[string]string{}
	return "", paraList, nil
}

func getPvObj(client kubernetes.Interface, volumeID string) (*v1.PersistentVolume, error) {
	return client.CoreV1().PersistentVolumes().Get(context.Background(), volumeID, metav1.GetOptions{})
}
