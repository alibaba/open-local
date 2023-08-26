/*
Copyright © 2021 Alibaba Group Holding Ltd.
Copyright 2014 The Kubernetes Authors.

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

package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	csilib "github.com/container-storage-interface/spec/lib/go/csi"
	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions/volumesnapshot/v1"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	storagev1informers "k8s.io/client-go/informers/storage/v1"
	"k8s.io/client-go/kubernetes"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	log "k8s.io/klog/v2"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
	k8svol "k8s.io/kubernetes/pkg/volume"
)

// WordSepNormalizeFunc changes all flags that contain "_" separators
func WordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}

// Contains method for a slice
func ContainsString(s []string, e string) bool {
	for _, a := range s {
		if strings.Compare(a, e) == 0 {
			return true
		}
	}
	return false
}

func IsLocalPV(pv *corev1.PersistentVolume) (isLocalPV bool, node string) {
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return isLocalPV, node
	}

	terms := pv.Spec.NodeAffinity.Required.NodeSelectorTerms
	if len(terms) <= 0 {
		return isLocalPV, node
	}

	term0 := terms[0]
	if len(term0.MatchExpressions) <= 0 {
		return isLocalPV, node
	}
	for _, ex := range term0.MatchExpressions {
		if len(ex.Values) <= 0 {
			continue
		}
		if ex.Key == localtype.KubernetesNodeIdentityKey &&
			ex.Values[0] != "" &&
			ex.Operator == corev1.NodeSelectorOpIn {
			isLocalPV = true
			return isLocalPV, ex.Values[0]
		}
	}

	return isLocalPV, node
}

func IsPodNeedAllocate(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return false
	}
	if pod.Spec.NodeName != "" {
		return true
	}
	return false
}

func ContainInlineVolumes(pod *corev1.Pod) (contain bool, node string) {
	if pod == nil {
		return false, ""
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && ContainsProvisioner(volume.CSI.Driver) {
			return true, pod.Spec.NodeName
		}
	}
	return false, ""
}

func GetInlineVolumeInfoFromParam(attributes map[string]string) (vgName string, size int64) {
	vgName, exist := attributes[localtype.VGName]
	if !exist {
		return "", 0
	}
	sizeStr, exist := attributes[localtype.ParamLVSize]
	if !exist {
		return vgName, 1024 * 1024 * 1024
	}

	quan, err := resource.ParseQuantity(sizeStr)
	if err != nil {
		return "", 0
	}
	size = quan.Value()
	return vgName, size
}

// GetVGNameFromCsiPV extracts vgName from open-local csi PV via
// VolumeAttributes
func GetVGNameFromCsiPV(pv *corev1.PersistentVolume) string {
	allocateInfo, err := localtype.GetAllocatedInfoFromPVAnnotation(pv)
	if err != nil {
		log.Warningf("Parse allocate info from PV %s error: %s", pv.Name, err.Error())
	} else if allocateInfo != nil && allocateInfo.VGName != "" {
		return allocateInfo.VGName
	}

	log.V(2).Infof("allocateInfo of pv %s is %v", pv.Name, allocateInfo)

	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes[localtype.VGName]; ok {
		return v
	}
	log.V(6).Infof("PV %s has no csi volumeAttributes /%q", pv.Name, localtype.VGName)

	return ""
}

// GetDeviceNameFromCsiPV extracts Device Name from open-local csi PV via
// VolumeAttributes
func GetDeviceNameFromCsiPV(pv *corev1.PersistentVolume) string {

	allocateInfo, err := localtype.GetAllocatedInfoFromPVAnnotation(pv)
	if err != nil {
		log.Warningf("Parse allocate info from PV %s error: %s", pv.Name, err.Error())
	} else if allocateInfo != nil && allocateInfo.DeviceName != "" {
		return allocateInfo.DeviceName
	}

	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes[string(localtype.DeviceName)]; ok {
		return v
	}
	log.V(6).Infof("PV %s has no csi volumeAttributes %q", pv.Name, "device")

	return ""
}

// GetMountPointFromCsiPV extracts MountPoint from open-local csi PV via
// VolumeAttributes
func GetMountPointFromCsiPV(pv *corev1.PersistentVolume) string {
	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes[string(localtype.VolumeTypeMountPoint)]; ok {
		return v
	}
	log.V(6).Infof("PV %s has no csi volumeAttributes %q", pv.Name, "mountpoint")

	return ""
}

func GetNodeNameFromCsiPV(pv *corev1.PersistentVolume) string {
	if pv.Spec.NodeAffinity == nil {
		log.Errorf("pv %s with nil nodeAffinity", pv.Name)
		return ""
	}
	if pv.Spec.NodeAffinity.Required == nil || len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
		log.Errorf("pv %s with nil Required or nil required.nodeSelectorTerms", pv.Name)
		return ""
	}
	if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions) == 0 {
		log.Errorf("pv %s with nil MatchExpressions", pv.Name)
		return ""
	}
	key := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key
	if key != localtype.KubernetesNodeIdentityKey {
		log.Errorf("pv %s with MatchExpressions %s, must be %s", pv.Name, key, localtype.KubernetesNodeIdentityKey)
		return ""
	}

	nodes := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values
	if len(nodes) == 0 {
		log.Errorf("pv %s with empty nodes", pv.Name)
		return ""
	}
	nodeName := nodes[0]
	return nodeName
}

func GetVGRequested(localPVs map[string]corev1.PersistentVolume, vgName string) (requested int64) {
	requested = 0
	for _, pv := range localPVs {
		vgNameFromPV := GetVGNameFromCsiPV(&pv)
		if vgNameFromPV == vgName {
			v := pv.Spec.Capacity[corev1.ResourceStorage]
			requested += v.Value()
			log.V(6).Infof("Size of pv(%s) from VG(%s) is %d", pv.Name, vgNameFromPV, requested)
		} else {
			log.V(6).Infof("pv(%s) is from VG(%s), not VG(%s)", pv.Name, vgNameFromPV, vgName)
		}
	}
	log.V(6).Infof("requested Size of VG %s is %d", vgName, requested)
	return requested
}

// CheckDiskOptions excludes mp which is readyonly or with unsupported fs type
func CheckMountPointOptions(mp *nodelocalstorage.MountPoint) bool {
	if mp == nil {
		return false
	}

	if StringsContains(localtype.SupportedFS, mp.FsType) == -1 {
		log.V(6).Infof("mount point fstype is %q, valid fstype is %s, skipped", mp.FsType, localtype.SupportedFS)
		return false
	}

	return true
}

// StringsContains check val exist in array
func StringsContains(array []string, val string) (index int) {
	index = -1
	for i := 0; i < len(array); i++ {
		if array[i] == val {
			index = i
			return
		}
	}
	return
}
func IsEmpty(r string) bool {
	return len(r) == 0
}

func HttpResponse(w http.ResponseWriter, code int, msg []byte) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

func HttpJSON(w http.ResponseWriter, code int, result interface{}) {
	response, err := json.Marshal(result)
	log.V(6).Infof("json output: %s", response)
	if err != nil {
		HttpResponse(w, 500, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(response)
	if err != nil {
		log.Warningf("noted http write failure met: %s", err.Error())
	}
}

func SliceEquals(src interface{}, dst interface{}) bool {
	return reflect.DeepEqual(src, dst)
}

func GetAddedAndRemovedItems(newList, oldList []string) (addedItems, unchangedItems, removedItems []string) {
	// get items that will be added
	for _, item := range newList {
		if StringsContains(oldList, item) == -1 {
			addedItems = append(addedItems, item)
		}
	}
	// get items that will be removed
	for _, item := range oldList {
		if StringsContains(newList, item) == -1 {
			removedItems = append(removedItems, item)
		} else {
			unchangedItems = append(unchangedItems, item)
		}
	}

	return
}

func ContainsProvisioner(name string) bool {
	for _, x := range localtype.ValidProvisionerNames {
		if x == name {
			return true
		}
	}
	return false
}

func HashSpec(storage *nodelocalstorage.NodeLocalStorage) uint64 {
	hash := fnv.New32a()
	hashutil.DeepHashObject(hash, storage.Spec.ListConfig)
	return uint64(hash.Sum32())
}

func HashStorageSpec(obj interface{}) uint64 {
	storage, ok := obj.(nodelocalstorage.NodeLocalStorage)
	if ok {
		return HashWithoutState(&storage)
	}
	storageSpec, ok := obj.(nodelocalstorage.NodeLocalStorageSpec)
	if ok {
		return HashWithoutState(&nodelocalstorage.NodeLocalStorage{
			Spec: storageSpec,
		})
	}
	log.Errorf("invalid type %#v", obj)
	return 0
}

// HashWithoutState remove the state field then compare
func HashWithoutState(storage *nodelocalstorage.NodeLocalStorage) uint64 {
	if storage == nil {
		return 0
	}
	cloned := storage.DeepCopy().Status
	cloned.NodeStorageInfo.State = nodelocalstorage.StorageState{}
	cloned.FilteredStorageInfo.UpdateStatus = nodelocalstorage.UpdateStatusInfo{}
	hash := fnv.New32a()

	hashutil.DeepHashObject(hash, cloned)
	return uint64(hash.Sum32())
}

func GetPVStorageSize(pv *corev1.PersistentVolume) int64 {
	if v, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
		return v.Value()
	} else {
		return 0
	}
}

func GetPVFromBoundPVC(pvc *corev1.PersistentVolumeClaim) (name string) {
	name = pvc.Spec.VolumeName
	if len(name) <= 0 {
		log.Warningf("pv name is empty for pvc %s", GetName(pvc.ObjectMeta))
	}
	return
}

func GetStorageClassFromPVC(pvc *corev1.PersistentVolumeClaim, scLister storagelisters.StorageClassLister) (*storagev1.StorageClass, error) {
	var scName string
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		err := fmt.Errorf("failed to fetch storage class %s with pvc %s: scName is nil", scName, GetName(pvc.ObjectMeta))
		log.Errorf(err.Error())
		return nil, err
	}
	scName = *pvc.Spec.StorageClassName
	sc, err := scLister.Get(scName)

	if err != nil {
		err := fmt.Errorf("failed to fetch storage class %s with pvc %s: %s", scName, GetName(pvc.ObjectMeta), err.Error())
		log.Errorf(err.Error())
		return nil, err
	}
	return sc, nil
}

func GetStorageClassFromPV(pv *corev1.PersistentVolume, p storagev1informers.Interface) *storagev1.StorageClass {
	var scName string
	if pv.Spec.StorageClassName == "" {
		log.Infof("pv %s has no associated storage class", pv.Name)
		return nil
	}
	scName = pv.Spec.StorageClassName
	sc, err := p.StorageClasses().Lister().Get(scName)
	if err != nil {
		log.Errorf("failed to fetch storage class %s with pv %s: %s", scName, pv.Name, err.Error())
		return nil
	}
	return sc
}

func GetVGNameFromPVC(pvc *corev1.PersistentVolumeClaim, scLister storagelisters.StorageClassLister) (string, error) {
	sc, err := GetStorageClassFromPVC(pvc, scLister)
	if err != nil {
		return "", err
	}
	if sc == nil {
		return "", nil
	}
	vgName, ok := sc.Parameters["vgName"]
	if !ok {
		log.V(6).Infof("storage class %s has no parameter %q set", sc.Name, "vgName")
		return "", nil
	}
	return vgName, nil
}

func GetMediaTypeFromPVC(pvc *corev1.PersistentVolumeClaim, scLister storagelisters.StorageClassLister) (localtype.MediaType, error) {
	sc, err := GetStorageClassFromPVC(pvc, scLister)
	if err != nil {
		return "", err
	}
	if sc == nil {
		return "", nil
	}
	mediaType, ok := sc.Parameters["mediaType"]
	if !ok {
		log.V(6).Infof("storage class %s has no parameter %q set", sc.Name, "mediaType")
		return "", nil
	}
	return localtype.MediaType(mediaType), nil
}

func IsLocalPVC(claim *corev1.PersistentVolumeClaim, scLister storagelisters.StorageClassLister) (bool, localtype.VolumeType) {
	sc, err := GetStorageClassFromPVC(claim, scLister)
	if err != nil {
		return false, ""
	}

	if sc == nil {
		return false, ""
	}
	if !ContainsProvisioner(sc.Provisioner) {
		return false, ""
	}
	return true, LocalPVType(sc)
}

func IsOpenLocalPV(pv *corev1.PersistentVolume) (bool, localtype.VolumeType) {
	if pv.Spec.CSI != nil && ContainsProvisioner(pv.Spec.CSI.Driver) {
		attributes := pv.Spec.CSI.VolumeAttributes
		// check open-local type
		if value, exist := attributes[localtype.VolumeTypeKey]; exist {
			if localtype, err := localtype.VolumeTypeFromString(value); err == nil {
				return true, localtype
			}
		}
	}
	return false, localtype.VolumeTypeUnknown
}

func LocalPVType(sc *storagev1.StorageClass) localtype.VolumeType {
	if t, ok := sc.Parameters[localtype.VolumeTypeKey]; ok {
		switch localtype.VolumeType(t) {
		case localtype.VolumeTypeMountPoint, localtype.VolumeTypeDevice, localtype.VolumeTypeLVM, localtype.VolumeTypeQuota:
			return localtype.VolumeType(t)
		default:
			return localtype.VolumeTypeUnknown
		}
	}
	return localtype.VolumeTypeUnknown
}

func GetPVCRequested(pvc *corev1.PersistentVolumeClaim) int64 {
	var value int64
	if v, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		value = v.Value()
	}
	if value != 0 {
		return value
	}
	if v, ok := pvc.Spec.Resources.Limits[corev1.ResourceStorage]; ok {
		value = v.Value()
	}
	return value
}

func GetPVSize(pv *corev1.PersistentVolume) int64 {
	capacity := pv.Spec.Capacity
	for name, c := range capacity {
		if name == corev1.ResourceStorage {
			return c.Value()
		}
	}
	return 0
}

func PVCName(storageObj interface{}) string {
	if storageObj == nil {
		return ""
	}
	switch t := storageObj.(type) {
	case *corev1.PersistentVolume:
		ref := t.Spec.ClaimRef
		return GetNameKey(ref.Namespace, ref.Name)
	case corev1.PersistentVolume:
		ref := t.Spec.ClaimRef
		return GetNameKey(ref.Namespace, ref.Name)
	case *corev1.PersistentVolumeClaim:
		return GetName(t.ObjectMeta)
	case corev1.PersistentVolumeClaim:
		return GetName(t.ObjectMeta)
	}
	return ""

}

func PVCNameFromPV(pv *corev1.PersistentVolume) (pvcName, pvcNamespace string) {
	if pv == nil || pv.Spec.ClaimRef == nil {
		return "", ""
	}
	return pv.Spec.ClaimRef.Name, pv.Spec.ClaimRef.Namespace
}

func NodeNameFromPV(pv *corev1.PersistentVolume) string {
	b, nodeName := IsLocalPV(pv)
	if b && nodeName != "" {
		return nodeName
	}
	return ""
}

func NodeNameFromPVC(pvc *corev1.PersistentVolumeClaim) string {
	if pvc.Annotations == nil {
		return ""
	}
	return pvc.Annotations[localtype.AnnoSelectedNode]
}

func PvcContainsSelectedNode(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}
	if len(pvc.Annotations) <= 0 {
		return false
	}
	if v, ok := pvc.Annotations[localtype.AnnoSelectedNode]; ok {
		// according to
		return len(v) > 0
	}
	return false
}

// PodPvcAllowReschedule returns true if any of pvcs has in pending status
// for more than 5 minutes
// the fakeNow is parameter for unit testing
func PodPvcAllowReschedule(pvcs []*corev1.PersistentVolumeClaim, fakeNow *time.Time) bool {
	if fakeNow == nil {
		now := time.Now()
		fakeNow = &now
	}
	for _, pvc := range pvcs {
		deadLine := pvc.CreationTimestamp.Add(5 * time.Minute)
		if fakeNow.After(deadLine) {
			log.Infof("pvc %s has pending for %s", PVCName(pvc), fakeNow.Sub(pvc.CreationTimestamp.Time))
			return true
		}
	}
	return false
}

func GeneratePodPatch(oldPod, newPod *corev1.Pod) ([]byte, error) {
	oldData, err := json.Marshal(oldPod)
	if err != nil {
		return nil, err
	}

	newData, err := json.Marshal(newPod)
	if err != nil {
		return nil, err
	}
	return strategicpatch.CreateTwoWayMergePatch(oldData, newData, &corev1.Pod{})
}

func GeneratePVPatch(oldPV, newPV *corev1.PersistentVolume) ([]byte, error) {
	oldData, err := json.Marshal(oldPV)
	if err != nil {
		return nil, err
	}

	newData, err := json.Marshal(newPV)
	if err != nil {
		return nil, err
	}
	return strategicpatch.CreateTwoWayMergePatch(oldData, newData, &corev1.PersistentVolume{})
}

// Format formats the source with the given filesystem type
func Format(source, fsType string) error {
	mkfsCmd := fmt.Sprintf("mkfs.%s", fsType)

	_, err := exec.LookPath(mkfsCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return fmt.Errorf("%q executable not found in $PATH", mkfsCmd)
		}
		return err
	}

	mkfsArgs := []string{}
	if fsType == "" {
		return errors.New("fs type is not specified for formatting the volume")
	}
	if source == "" {
		return errors.New("source is not specified for formatting the volume")
	}
	mkfsArgs = append(mkfsArgs, source)
	if fsType == "ext4" || fsType == "ext3" {
		mkfsArgs = []string{"-F", source}
	}

	log.V(6).Infof("Format %s with fsType %s, the command is %s %v", source, fsType, mkfsCmd, mkfsArgs)
	out, err := exec.Command(mkfsCmd, mkfsArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("formatting disk failed: %v cmd: '%s %s' output: %q",
			err, mkfsCmd, strings.Join(mkfsArgs, " "), string(out))
	}

	return nil
}

// CommandRunFunc define the run function in utils for ut
type CommandRunFunc func(cmd string) (string, error)

// Run run shell command
func Run(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with out: " + string(out) + ", with error: " + err.Error())
	}
	return string(out), nil
}

// GetMetrics get path metric
func GetMetrics(path string) (*csilib.NodeGetVolumeStatsResponse, error) {
	if path == "" {
		return nil, fmt.Errorf("getMetrics No path given")
	}
	available, capacity, usage, inodes, inodesFree, inodesUsed, err := FsInfo(path)
	if err != nil {
		return nil, err
	}

	metrics := &k8svol.Metrics{Time: metav1.Now()}
	metrics.Available = resource.NewQuantity(available, resource.BinarySI)
	metrics.Capacity = resource.NewQuantity(capacity, resource.BinarySI)
	metrics.Used = resource.NewQuantity(usage, resource.BinarySI)
	metrics.Inodes = resource.NewQuantity(inodes, resource.BinarySI)
	metrics.InodesFree = resource.NewQuantity(inodesFree, resource.BinarySI)
	metrics.InodesUsed = resource.NewQuantity(inodesUsed, resource.BinarySI)

	metricAvailable, ok := (*(metrics.Available)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch available bytes for target: %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch available bytes")
	}
	metricCapacity, ok := (*(metrics.Capacity)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch capacity bytes for target: %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch capacity bytes")
	}
	metricUsed, ok := (*(metrics.Used)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch used bytes for target %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch used bytes")
	}
	metricInodes, ok := (*(metrics.Inodes)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch available inodes for target %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch available inodes")
	}
	metricInodesFree, ok := (*(metrics.InodesFree)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch free inodes for target: %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch free inodes")
	}
	metricInodesUsed, ok := (*(metrics.InodesUsed)).AsInt64()
	if !ok {
		log.Errorf("failed to fetch used inodes for target: %s", path)
		return nil, status.Error(codes.Unknown, "failed to fetch used inodes")
	}

	return &csilib.NodeGetVolumeStatsResponse{
		Usage: []*csilib.VolumeUsage{
			{
				Available: metricAvailable,
				Total:     metricCapacity,
				Used:      metricUsed,
				Unit:      csilib.VolumeUsage_BYTES,
			},
			{
				Available: metricInodesFree,
				Total:     metricInodes,
				Used:      metricInodesUsed,
				Unit:      csilib.VolumeUsage_INODES,
			},
		},
	}, nil
}

func NeedSkip(args schedulerapi.ExtenderArgs) bool {
	pod := args.Pod
	// no volume, skipped
	if len(pod.Spec.Volumes) <= 0 {
		log.Infof("skip pod %s scheduling, reason: no volume", GetName(pod.ObjectMeta))
		return true
	}
	// no volume contains PVC
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil || v.CSI != nil {
			return false
		}
	}
	log.Infof("skip pod %s scheduling, reason: no pv", GetName(pod.ObjectMeta))

	return true
}

// GetAccessModes returns a slice containing all of the access modes defined
// in the passed in VolumeCapabilities.
func GetAccessModes(caps []*csilib.VolumeCapability) *[]string {
	modes := []string{}
	for _, c := range caps {
		modes = append(modes, c.AccessMode.GetMode().String())
	}
	return &modes
}

func EnsureBlock(target string) error {
	fi, err := os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && fi.IsDir() {
		os.Remove(target)
	}
	targetPathFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, 0750)
	if err != nil {
		return fmt.Errorf("failed to create block:%s with error: %v", target, err)
	}
	if err := targetPathFile.Close(); err != nil {
		return fmt.Errorf("failed to close targetPath:%s with error: %v", target, err)
	}
	return nil
}

func MountBlock(source, target string, opts ...string) error {
	mountCmd := "mount"
	mountArgs := []string{}

	if source == "" {
		return errors.New("source is not specified for mounting the volume")
	}
	if target == "" {
		return errors.New("target is not specified for mounting the volume")
	}

	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}
	mountArgs = append(mountArgs, source)
	mountArgs = append(mountArgs, target)
	// create target, os.Mkdirall is noop if it exists
	_, err := os.Create(target)
	if err != nil {
		return err
	}

	log.V(6).Infof("Mount %s to %s, the command is %s %v", source, target, mountCmd, mountArgs)
	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}
	return nil
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

func IsReadOnlyPV(pv *corev1.PersistentVolume) bool {
	if pv.Spec.CSI != nil {
		attributes := pv.Spec.CSI.VolumeAttributes
		if value, exist := attributes[localtype.ParamReadonly]; exist && value == "true" {
			return true
		}
	}
	return false
}

func IsReadOnlySnapshotPVC(claim *corev1.PersistentVolumeClaim, snapInformer volumesnapshotinformers.Interface) bool {
	// 数据源为快照
	// 快照类表示为只读快照
	if claim.Spec.DataSource != nil && claim.Spec.DataSource.Kind == "VolumeSnapshot" {
		snasphotName := claim.Spec.DataSource.Name
		snasphotNamespace := claim.Namespace
		snapshot, err := snapInformer.VolumeSnapshots().Lister().VolumeSnapshots(snasphotNamespace).Get(snasphotName)
		if err != nil {
			log.Warningf("fail to get snapshot %s(may have been deleted): %s", GetNameKey(snasphotNamespace, snasphotName), err.Error())
			return false
		}
		return IsSnapshotClassReadOnly(*snapshot.Spec.VolumeSnapshotClassName, snapInformer)
	}
	return false
}

func IsReadOnlySnapshotPVC2(claim *corev1.PersistentVolumeClaim, snapClient snapshot.Interface) bool {
	// 数据源为快照
	// 快照类表示为只读快照
	if claim.Spec.DataSource != nil && claim.Spec.DataSource.Kind == "VolumeSnapshot" {
		snasphotName := claim.Spec.DataSource.Name
		snasphotNamespace := claim.Namespace
		snapshot, err := snapClient.SnapshotV1().VolumeSnapshots(snasphotNamespace).Get(context.TODO(), snasphotName, metav1.GetOptions{})
		if err != nil {
			log.Warningf("fail to get snapshot %s(may have been deleted): %s", GetNameKey(snasphotNamespace, snasphotName), err.Error())
			return false
		}
		return IsSnapshotClassReadOnly2(*snapshot.Spec.VolumeSnapshotClassName, snapClient)
	}
	return false
}

func IsSnapshotClassReadOnly(className string, snapInformer volumesnapshotinformers.Interface) bool {
	snapshotClass, err := snapInformer.VolumeSnapshotClasses().Lister().Get(className)
	if err != nil {
		log.Warningf("fail to get snapshotClass %s(may have been deleted): %s", className, err.Error())
		return false
	}
	if value, exist := snapshotClass.Parameters[localtype.ParamReadonly]; exist && value == "true" {
		return true
	}
	return false
}

func IsSnapshotClassReadOnly2(className string, snapClient snapshot.Interface) bool {
	snapshotClass, err := snapClient.SnapshotV1().VolumeSnapshotClasses().Get(context.TODO(), className, metav1.GetOptions{})
	if err != nil {
		log.Warningf("fail to get snapshotClass %s(may have been deleted): %s", className, err.Error())
		return false
	}
	if value, exist := snapshotClass.Parameters[localtype.ParamReadonly]; exist && value == "true" {
		return true
	}
	return false
}

// FsInfo linux returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func FsInfo(path string) (int64, int64, int64, int64, int64, int64, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(path, statfs)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}

	// Available is blocks available * fragment size
	available := int64(statfs.Bavail) * int64(statfs.Bsize)

	// Capacity is total block count * fragment size
	capacity := int64(statfs.Blocks) * int64(statfs.Bsize)

	// Usage is block being used * fragment size (aka block size).
	usage := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	inodes := int64(statfs.Files)
	inodesFree := int64(statfs.Ffree)
	inodesUsed := inodes - inodesFree

	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

func TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Infof("%s took %s", name, elapsed)
}

func GetClusterID(kubeclient kubernetes.Interface) (string, error) {
	ns, err := kubeclient.CoreV1().Namespaces().Get(context.Background(), "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(ns.UID), nil
}

func GetVolumeSnapshotContent(snapclient snapshot.Interface, snapshotContentID string) (*snapshotapi.VolumeSnapshotContent, error) {
	// Step 1: get yoda snapshot prefix
	prefix := os.Getenv(localtype.EnvSnapshotPrefix)
	if prefix == "" {
		prefix = localtype.DefaultSnapshotPrefix
	}
	// Step 2: get snapshot content api
	return snapclient.SnapshotV1().VolumeSnapshotContents().Get(context.TODO(), strings.Replace(snapshotContentID, prefix, "snapcontent", 1), metav1.GetOptions{})
}

func GetNameKey(nameSpace, name string) string {
	return fmt.Sprintf("%s/%s", nameSpace, name)
}

func GetName(meta metav1.ObjectMeta) string {
	return GetNameKey(meta.Namespace, meta.Name)
}

// GetFuncName returns funcname in the form of 'github.com/alibaba/open-local/pkg/scheduler/algorithm/predicates.CapacityPredicate'
// if `full=true`, otherwise 'predicates.CapacityPredicate'
func GetFuncName(funcName interface{}, full bool) string {
	origin := runtime.FuncForPC(reflect.ValueOf(funcName).Pointer()).Name()
	if !full {
		funcs := strings.Split(origin, "/")
		if len(funcs) > 1 {
			return funcs[len(funcs)-1]
		}
		return funcs[0]
	}
	return origin
}