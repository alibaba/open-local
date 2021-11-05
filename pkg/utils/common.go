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

package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	csilib "github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	storagev1informers "k8s.io/client-go/informers/storage/v1"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
	k8svol "k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/util/fs"
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

// GetVGNameFromCsiPV extracts vgName from open-local csi PV via
// VolumeAttributes
func GetVGNameFromCsiPV(pv *corev1.PersistentVolume) string {
	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes["vgName"]; ok {
		return v
	}
	log.Debugf("PV %s has no csi volumeAttributes /%q", pv.Name, "vgName")

	return ""
}

// GetDeviceNameFromCsiPV extracts Device Name from open-local csi PV via
// VolumeAttributes
func GetDeviceNameFromCsiPV(pv *corev1.PersistentVolume) string {
	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes[string(localtype.VolumeTypeDevice)]; ok {
		return v
	}
	log.Debugf("PV %s has no csi volumeAttributes %q", pv.Name, "device")

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
	log.Debugf("PV %s has no csi volumeAttributes %q", pv.Name, "mountpoint")

	return ""
}

func GetVGRequested(localPVs map[string]corev1.PersistentVolume, vgName string) (requested int64) {
	requested = 0
	for _, pv := range localPVs {
		vgNameFromPV := GetVGNameFromCsiPV(&pv)
		if vgNameFromPV == vgName {
			v := pv.Spec.Capacity[corev1.ResourceStorage]
			requested += v.Value()
			log.Debugf("size of pv(%s) from VG(%s) is %d", pv.Name, vgNameFromPV, requested)
		} else {
			log.Debugf("pv(%s) is from VG(%s), not VG(%s)", pv.Name, vgNameFromPV, vgName)
		}
	}
	log.Debugf("requested size of VG %s is %d", vgName, requested)
	return requested
}

//CheckDiskOptions excludes mp which is readyonly or with unsupported fs type
func CheckMountPointOptions(mp *nodelocalstorage.MountPoint) bool {
	if mp == nil {
		return false
	}

	if StringsContains(localtype.SupportedFS, mp.FsType) == -1 {
		log.Debugf("mount point fstype is %q, valid fstype is %s, skipped", mp.FsType, localtype.SupportedFS)
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
	log.Debugf("json output: %s", response)
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
		log.Warningf("pv name is empty for pvc %s/%s", pvc.Namespace, pvc.Name)
	}
	return
}

func GetStorageClassFromPVC(pvc *corev1.PersistentVolumeClaim, p storagev1informers.Interface) *storagev1.StorageClass {
	var scName string
	if pvc.Spec.StorageClassName == nil {
		log.Infof("pvc %s/%s has no associated storage class", pvc.Namespace, pvc.Name)
		return nil
	}
	scName = *pvc.Spec.StorageClassName
	sc, err := p.StorageClasses().Lister().Get(scName)
	if err != nil {
		log.Errorf("failed to fetch storage class %s with pvc %s/%s: %s", scName, pvc.Namespace, pvc.Name, err.Error())
		return nil
	}
	return sc
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

func GetVGNameFromPVC(pvc *corev1.PersistentVolumeClaim, p storagev1informers.Interface) string {
	sc := GetStorageClassFromPVC(pvc, p)
	if sc == nil {
		return ""
	}
	vgName, ok := sc.Parameters["vgName"]
	if !ok {
		log.Debugf("storage class %s has no parameter %q set", sc.Name, "vgName")
		return ""
	}
	return vgName
}

func GetMediaTypeFromPVC(pvc *corev1.PersistentVolumeClaim, p storagev1informers.Interface) localtype.MediaType {
	sc := GetStorageClassFromPVC(pvc, p)
	if sc == nil {
		return ""
	}
	mediaType, ok := sc.Parameters["mediaType"]
	if !ok {
		log.Debugf("storage class %s has no parameter %q set", sc.Name, "mediaType")
		return ""
	}
	return localtype.MediaType(mediaType)
}

func IsLocalPVC(claim *corev1.PersistentVolumeClaim, p storagev1informers.Interface, containReadonlySnapshot bool) (bool, localtype.VolumeType) {
	sc := GetStorageClassFromPVC(claim, p)
	if sc == nil {
		return false, ""
	}
	if !ContainsProvisioner(sc.Provisioner) {
		return false, ""
	}
	if IsLocalSnapshotPVC(claim) && !containReadonlySnapshot {
		return false, ""
	}
	return true, LocalPVType(sc)
}

func IsOpenLocalPV(pv *corev1.PersistentVolume, p storagev1informers.Interface, c corev1informers.Interface, containReadonlySnapshot bool) (bool, localtype.VolumeType) {
	var isSnapshot, isSnapshotReadOnly bool = false, false

	if pv.Spec.CSI != nil && ContainsProvisioner(pv.Spec.CSI.Driver) {
		attributes := pv.Spec.CSI.VolumeAttributes
		// check if is snapshot pv according to pvc
		if value, exist := attributes[localtype.TagSnapshot]; exist && value != "" {
			isSnapshot = true
		}
		if value, exist := attributes[localtype.ParamSnapshotReadonly]; exist && value == "true" {
			isSnapshotReadOnly = true
		}
		if isSnapshot && !isSnapshotReadOnly {
			log.Errorf("[IsOpenLocalPV]only support ro snapshot pv!")
			return false, ""
		}
		if isSnapshot && !containReadonlySnapshot {
			return false, ""
		}
		// check open-local type
		if value, exist := attributes[localtype.VolumeTypeKey]; exist {
			if lsstype, err := localtype.VolumeTypeFromString(value); err == nil {
				return true, lsstype
			}
		}
	}
	return false, localtype.VolumeTypeUnknown
}

func IsLocalSnapshotPVC(claim *corev1.PersistentVolumeClaim) bool {
	return IsSnapshotPVC(claim)
}

func IsSnapshotPVC(claim *corev1.PersistentVolumeClaim) bool {
	// check if kind of datasource is "VolumeSnapshot"
	if claim.Spec.DataSource != nil && claim.Spec.DataSource.Kind == "VolumeSnapshot" {
		return true
	}
	return false
}

func ContainsSnapshotPVC(claims []*corev1.PersistentVolumeClaim) (contain bool) {
	contain = false
	for _, claim := range claims {
		if IsSnapshotPVC(claim) {
			contain = true
			break
		}
	}
	return
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
		return fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
	case corev1.PersistentVolume:
		ref := t.Spec.ClaimRef
		return fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
	case *corev1.PersistentVolumeClaim:
		return fmt.Sprintf("%s/%s", t.Namespace, t.Name)
	case corev1.PersistentVolumeClaim:
		return fmt.Sprintf("%s/%s", t.Namespace, t.Name)
	}
	return ""

}

func PvcContainsSelectedNode(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}
	if len(pvc.Annotations) <= 0 {
		return false
	}
	if v, ok := pvc.Annotations[localtype.AnnSelectedNode]; ok {
		// according to
		return len(v) > 0
	}
	return false
}

func PodName(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
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

// IsFormatted checks whether the source device is formatted or not. It
// returns true if the source device is already formatted.
func IsFormatted(source string) (bool, error) {
	if source == "" {
		return false, errors.New("source is not specified")
	}

	fileCmd := "file"
	_, err := exec.LookPath(fileCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable not found in $PATH", fileCmd)
		}
		return false, err
	}

	args := []string{"-sL", source}

	out, err := exec.Command(fileCmd, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking formatting failed: %v cmd: %q output: %q",
			err, fileCmd, string(out))
	}

	output := strings.TrimPrefix(string(out), fmt.Sprintf("%s:", source))
	if strings.TrimSpace(output) == "data" {
		return false, nil
	}

	return true, nil
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

	log.Debugf("Format %s with fsType %s, the command is %s %v", source, fsType, mkfsCmd, mkfsArgs)
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
	available, capacity, usage, inodes, inodesFree, inodesUsed, err := fs.FsInfo(path)
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
