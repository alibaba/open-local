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

package algo

import (
	"fmt"
	"sort"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/utils"

	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	corev1informers "k8s.io/client-go/informers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

const MinScore int = 0
const MaxScore int = 10

// AllocateLVMVolume contains two policy: BINPACK/SPREAD
func AllocateLVMVolume(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	if len(pvcs) <= 0 {
		return
	}
	if pod == nil {
		return fits, units, fmt.Errorf("pod is nil")
	}
	klog.Infof("allocating lvm volume for pod %s/%s", pod.Namespace, pod.Name)

	fits, units, err = ProcessLVMPVCPredicate(pvcs, node, ctx)
	if err != nil {
		return
	}

	if len(units) <= 0 {
		return false, units, nil
	}
	klog.Infof("node %s is capable of lvm %d pvcs", node.Name, len(pvcs))
	klog.V(6).Infof("pod=%s/%s, node=%s, units:%#v", pod.Namespace, pod.Name, node.Name, units)
	return true, units, nil
}

func ProcessLVMPVCPredicate(pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	pvcsWithVG, pvcsWithoutVG := DivideLVMPVCs(pvcs, ctx)
	cacheVGsMap, err := GetNodeVGMap(node, ctx)
	if err != nil {
		return false, units, err
	}

	// process pvcsWithVG first
	for _, pvc := range pvcsWithVG {
		vgName, err := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers.StorageClasses().Lister())
		if err != nil {
			return false, units, err
		}
		requestedSize := utils.GetPVCRequested(pvc)

		vg, ok := cacheVGsMap[cache.ResourceName(vgName)]
		if !ok {
			return false, units, errors.NewNoSuchVGError(vgName, node.GetName())
		}

		freeSize := vg.Capacity - vg.Requested
		klog.V(6).Infof("validating vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", vgName, freeSize, pvc.Name, requestedSize)

		if freeSize < requestedSize {
			return false, units, errors.NewInsufficientLVMError(requestedSize, vg.Requested, vg.Capacity, vg.Name, node.GetName())
		}
		tmp := cacheVGsMap[cache.ResourceName(vgName)]
		tmp.Requested += requestedSize
		cacheVGsMap[cache.ResourceName(vgName)] = tmp
		u := cache.AllocatedUnit{
			NodeName:   node.Name,
			VolumeType: localtype.VolumeTypeLVM,
			Requested:  requestedSize,
			Allocated:  requestedSize, // for LVM requested is always equal to allocated
			VgName:     string(vgName),
			Device:     "",
			MountPoint: "",
			PVCName:    utils.PVCName(pvc),
		}
		units = append(units, u)
	}

	// make a copy slice of cacheVGsMap
	cacheVGsSlice := make([]cache.SharedResource, len(cacheVGsMap))
	for _, vg := range cacheVGsMap {
		cacheVGsSlice = append(cacheVGsSlice, vg)
	}

	if len(cacheVGsSlice) <= 0 {
		return false, units, errors.NewNoAvailableVGError(node.Name)
	}
	// process pvcsWithoutVG
	for _, pvc := range pvcsWithoutVG {
		requestedSize := utils.GetPVCRequested(pvc)

		// sort by available size
		sort.Slice(cacheVGsSlice, func(i, j int) bool {
			return (cacheVGsSlice[i].Capacity - cacheVGsSlice[i].Requested) < (cacheVGsSlice[j].Capacity - cacheVGsSlice[j].Requested)
		})

		for i, vg := range cacheVGsSlice {
			freeSize := vg.Capacity - vg.Requested
			klog.V(6).Infof("validating vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", vg.Name, freeSize, pvc.Name, requestedSize)

			if freeSize < requestedSize {
				if i == len(cacheVGsSlice)-1 {
					return false, units, errors.NewInsufficientLVMError(requestedSize, vg.Requested, vg.Capacity, vg.Name, node.GetName())
				}
				continue
			}
			cacheVGsSlice[i].Requested += requestedSize
			u := cache.AllocatedUnit{
				NodeName:   node.Name,
				VolumeType: localtype.VolumeTypeLVM,
				Requested:  requestedSize,
				Allocated:  requestedSize, // for LVM requested is always equal to allocated
				VgName:     string(vg.Name),
				Device:     "",
				MountPoint: "",
				PVCName:    utils.PVCName(pvc),
			}
			units = append(units, u)
			break
		}
	}

	return true, units, nil
}

func HandleInlineLVMVolume(ctx *algorithm.SchedulingContext, node *corev1.Node, pod *corev1.Pod) (fits bool, units []cache.AllocatedUnit, err error) {
	cacheVGsMap, err := GetNodeVGMap(node, ctx)
	if err != nil {
		return false, units, err
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, requestedSize := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				return false, units, fmt.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
			vg, ok := cacheVGsMap[cache.ResourceName(vgName)]
			if !ok {
				return false, units, errors.NewNoSuchVGError(vgName, node.GetName())
			}

			freeSize := vg.Capacity - vg.Requested

			if freeSize < requestedSize {
				return false, units, errors.NewInsufficientLVMError(requestedSize, vg.Requested, vg.Capacity, vg.Name, node.GetName())
			}
			tmp := cacheVGsMap[cache.ResourceName(vgName)]
			tmp.Requested += requestedSize
			cacheVGsMap[cache.ResourceName(vgName)] = tmp
			u := cache.AllocatedUnit{
				NodeName:   node.Name,
				VolumeType: localtype.VolumeTypeLVM,
				Requested:  requestedSize,
				Allocated:  requestedSize, // for LVM requested is always equal to allocated
				VgName:     string(vgName),
				Device:     "",
				MountPoint: "",
				PVCName:    "",
			}
			units = append(units, u)
		}
	}

	return true, units, nil
}

// DivideLVMPVCs divide pvcs into pvcsWithVG and pvcsWithoutVG
func DivideLVMPVCs(pvcs []*corev1.PersistentVolumeClaim, ctx *algorithm.SchedulingContext) (pvcsWithVG, pvcsWithoutVG []*corev1.PersistentVolumeClaim) {
	for _, pvc := range pvcs {
		vgName, err := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers.StorageClasses().Lister())
		if err != nil {
			return
		}
		if vgName == "" {
			pvcsWithoutVG = append(pvcsWithoutVG, pvc)
		} else {
			pvcsWithVG = append(pvcsWithVG, pvc)
		}
	}
	return
}

// GetNodeVGMap make a copy map of NodeCache VGs
func GetNodeVGMap(node *corev1.Node, ctx *algorithm.SchedulingContext) (cacheVGsMap map[cache.ResourceName]cache.SharedResource, err error) {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nodeCache == nil {
		return nil, fmt.Errorf("node %s not found from cache", node.Name)
	}

	cacheVGsMap = make(map[cache.ResourceName]cache.SharedResource, len(nodeCache.VGs))
	for k, v := range nodeCache.VGs {
		cacheVGsMap[k] = v
	}

	return
}

func AllocateMountPointVolume(
	pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node,
	ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {

	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		klog.Infof("allocating mount point volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	klog.V(6).Infof("pvcs: %#v, node: %#v", pvcs, node)
	fits, units, err = ProcessMPPVC(pod, pvcs, node, ctx)

	return fits, units, err
}

func ProcessMPPVC(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	pvcsWithTypeSSD, pvcsWithTypeHDD, err := DividePVCAccordingToMediaType(pvcs, ctx.StorageV1Informers.StorageClasses().Lister())
	if err != nil {
		return false, units, err
	}

	freeMPSSD, freeMPHDD, err := GetFreeMP(node, ctx)
	if err != nil {
		return false, units, err
	}

	totalCount, err := GetCacheMPCount(node, ctx)
	if err != nil {
		return false, units, err
	}
	// ssd
	freeMPSSDCount := int64(len(freeMPSSD))
	requestedSSDCount := int64(len(pvcsWithTypeSSD))
	// hdd
	freeMPHDDCount := int64(len(freeMPHDD))
	requestedHDDCount := int64(len(pvcsWithTypeHDD))

	// process pvcsWithTypeSSD first.
	if freeMPSSDCount < requestedSSDCount {
		return false, units, errors.NewInsufficientMountPointCountError(
			requestedSSDCount,
			freeMPSSDCount,
			totalCount,
			localtype.MediaTypeSSD,
			node.GetName(),
		)
	}
	fits, rstUnits, err := CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeMountPoint, freeMPSSD, pvcsWithTypeSSD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	// process pvcsWithTypeHDD second.
	if freeMPHDDCount < requestedHDDCount {
		klog.Infof("there is no enough mount point volume on node %s, want(cnt) %d, actual(cnt) %d, type hdd", node.Name, requestedHDDCount, freeMPHDDCount)
		return false, units, errors.NewInsufficientMountPointCountError(
			requestedHDDCount,
			freeMPHDDCount,
			totalCount,
			localtype.MediaTypeHDD,
			node.GetName(),
		)
	}
	fits, rstUnits, err = CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeMountPoint, freeMPHDD, pvcsWithTypeHDD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	klog.V(6).Infof("node %s is capable of mount point %d pvcs", node.Name, len(pvcs))
	return true, units, nil
}

// DividePVCAccordingToMediaType divide pvcs into pvcsWithSSD and pvcsWithHDD
func DividePVCAccordingToMediaType(pvcs []*corev1.PersistentVolumeClaim, scLister storagelisters.StorageClassLister) (pvcsWithTypeSSD, pvcsWithTypeHDD []*corev1.PersistentVolumeClaim, err error) {
	for _, pvc := range pvcs {
		var mediaType localtype.MediaType
		mediaType, err = utils.GetMediaTypeFromPVC(pvc, scLister)
		if err != nil {
			return
		}
		switch mediaType {
		case localtype.MediaTypeSSD:
			pvcsWithTypeSSD = append(pvcsWithTypeSSD, pvc)
		case localtype.MediaTypeHDD:
			pvcsWithTypeHDD = append(pvcsWithTypeHDD, pvc)
		default:
			err = fmt.Errorf("mediaType %s not support! pvc: %s/%s", mediaType, pvc.Namespace, pvc.Name)
			klog.Errorf(err.Error())
			return
		}
	}
	return
}

// GetFreeMP divide nodeCache.MountPoints into freeMPSSD and freeMPHDD
func GetFreeMP(node *corev1.Node, ctx *algorithm.SchedulingContext) (freeMPSSD, freeMPHDD []cache.ExclusiveResource, err error) {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nodeCache == nil {
		return nil, nil, fmt.Errorf("node %s not found from cache", node.Name)
	}

	for _, mp := range nodeCache.MountPoints {
		if mp.MediaType == localtype.MediaTypeSSD && !mp.IsAllocated {
			freeMPSSD = append(freeMPSSD, mp)
		} else if mp.MediaType == localtype.MediaTypeHDD && !mp.IsAllocated {
			freeMPHDD = append(freeMPHDD, mp)
		}
	}

	return
}

// GetCacheMPCount get MP count from ClusterNodeCache
func GetCacheMPCount(node *corev1.Node, ctx *algorithm.SchedulingContext) (count int64, err error) {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nodeCache == nil {
		return 0, fmt.Errorf("node %s not found from cache", node.Name)
	}

	return int64(len(nodeCache.MountPoints)), nil
}

func CheckExclusiveResourceMeetsPVCSize(resource localtype.VolumeType, ers []cache.ExclusiveResource, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	// sort from small to large
	sort.Slice(pvcs, func(i, j int) bool {
		return utils.GetPVCRequested(pvcs[i]) < utils.GetPVCRequested(pvcs[j])
	})
	sort.Slice(ers, func(i, j int) bool {
		return ers[i].Capacity < ers[j].Capacity
	})

	if err != nil {
		return false, units, err
	}
	disksCount := int64(len(ers))
	pvcsCount := int64(len(pvcs))

	i := 0
	var maxSize int64 = 0
	for j, er := range ers {
		if pvcsCount == 0 {
			break
		}
		pvc := pvcs[i]
		requestedSize := utils.GetPVCRequested(pvc)
		if requestedSize > int64(maxSize) {
			maxSize = requestedSize
		}
		klog.V(6).Infof("[CheckExclusiveResourceMeetsPVCSize]%s(%s) capacity=%d, pvc requestedSize=%d", er.Name, string(er.MediaType), er.Capacity, requestedSize)
		if er.Capacity < requestedSize {
			if j == int(disksCount)-1 {
				return false, units, errors.NewInsufficientExclusiveResourceError(
					resource,
					requestedSize,
					maxSize)
			}
			continue
		}

		u := cache.AllocatedUnit{
			NodeName:   node.Name,
			VolumeType: resource,
			Requested:  requestedSize,
			Allocated:  er.Capacity,
			VgName:     "",
			Device:     er.Name,
			MountPoint: er.Name,
			PVCName:    utils.PVCName(pvc),
		}

		klog.V(6).Infof("found unit: %#v for pvc %#v", u, pvc)
		units = append(units, u)
		i++
		if i == int(pvcsCount) {
			break
		}
	}

	return true, units, nil
}

func AllocateDeviceVolume(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node,
	ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		klog.Infof("allocating device volume for pod %s/%s", pod.Namespace, pod.Name)
	}
	klog.V(6).Infof("pvcs: %#v, node: %#v", pvcs, node)
	fits, units, err = ProcessDevicePVC(pod, pvcs, node, ctx)

	return fits, units, err
}

// GetFreeDevice divide nodeCache.Devices into freeDeviceSSD and freeDeviceHDD
func GetFreeDevice(node *corev1.Node, ctx *algorithm.SchedulingContext) (freeDeviceSSD, freeDeviceHDD []cache.ExclusiveResource, err error) {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nodeCache == nil {
		return nil, nil, fmt.Errorf("node %s not found from cache", node.Name)
	}

	for _, device := range nodeCache.Devices {
		if device.MediaType == localtype.MediaTypeSSD && !device.IsAllocated {
			freeDeviceSSD = append(freeDeviceSSD, device)
		} else if device.MediaType == localtype.MediaTypeHDD && !device.IsAllocated {
			freeDeviceHDD = append(freeDeviceHDD, device)
		}
	}

	return
}

// GetCacheDeviceCount get Device count from ClusterNodeCache
func GetCacheDeviceCount(node *corev1.Node, ctx *algorithm.SchedulingContext) (count int64, err error) {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nodeCache == nil {
		return 0, fmt.Errorf("node %s not found from cache", node.Name)
	}

	return int64(len(nodeCache.Devices)), nil
}

func ProcessDevicePVC(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	pvcsWithTypeSSD, pvcsWithTypeHDD, err := DividePVCAccordingToMediaType(pvcs, ctx.StorageV1Informers.StorageClasses().Lister())
	if err != nil {
		return false, units, err
	}
	freeDeviceSSD, freeDeviceHDD, err := GetFreeDevice(node, ctx)
	if err != nil {
		return false, units, err
	}

	totalCount, err := GetCacheDeviceCount(node, ctx)
	if err != nil {
		return false, units, err
	}
	// ssd
	freeDeviceSSDCount := int64(len(freeDeviceSSD))
	requestedSSDCount := int64(len(pvcsWithTypeSSD))
	// hdd
	freeDeviceHDDCount := int64(len(freeDeviceHDD))
	requestedHDDCount := int64(len(pvcsWithTypeHDD))

	// process pvcsWithTypeSSD first.
	if freeDeviceSSDCount < requestedSSDCount {
		return false, units, errors.NewInsufficientDeviceCountError(
			requestedSSDCount,
			freeDeviceSSDCount,
			totalCount,
			localtype.MediaTypeSSD,
			node.GetName(),
		)
	}
	fits, rstUnits, err := CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeDevice, freeDeviceSSD, pvcsWithTypeSSD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	// process pvcsWithTypeHDD second.
	if freeDeviceHDDCount < requestedHDDCount {
		return false, units, errors.NewInsufficientDeviceCountError(
			requestedHDDCount,
			freeDeviceHDDCount,
			totalCount,
			localtype.MediaTypeHDD,
			node.GetName(),
		)
	}
	fits, rstUnits, err = CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeDevice, freeDeviceHDD, pvcsWithTypeHDD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	klog.V(6).Infof("node %s is capable of mount point %d pvcs", node.Name, len(pvcs))
	return true, units, nil
}

// If there is no snapshot pvc, just return true
func ProcessSnapshotPVC(pvcs []*corev1.PersistentVolumeClaim, nodeName string, coreV1Informers corev1informers.Interface, snapshotInformers volumesnapshotinformers.Interface) (fits bool, err error) {
	for _, pvc := range pvcs {
		// step 0: check if is snapshot pvc
		if !utils.IsSnapshotPVC(pvc) {
			continue
		}
		klog.Infof("[ProcessSnapshotPVC]data source of pvc %s/%s is snapshot", pvc.Namespace, pvc.Name)
		// step 1: get snapshot api
		snapName := pvc.Spec.DataSource.Name
		snapNamespace := pvc.Namespace
		snapshot, err := snapshotInformers.VolumeSnapshots().Lister().VolumeSnapshots(snapNamespace).Get(snapName)
		if err != nil {
			return false, fmt.Errorf("[ProcessSnapshotPVC]get snapshot %s/%s failed: %s", snapNamespace, snapName, err.Error())
		}
		klog.Infof("[ProcessSnapshotPVC]snapshot is %s", snapshot.Name)
		// step 2: get src pvc
		srcPVCName := *snapshot.Spec.Source.PersistentVolumeClaimName
		srcPVCNamespace := snapNamespace
		srcPVC, err := coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(srcPVCNamespace).Get(srcPVCName)
		if err != nil {
			return false, fmt.Errorf("[ProcessSnapshotPVC]get src pvc %s/%s failed: %s", snapNamespace, snapName, err.Error())
		}
		klog.Infof("[ProcessSnapshotPVC]source pvc is %s/%s", srcPVC.Namespace, srcPVC.Name)
		// step 3: get src node name
		srcNodeName := srcPVC.Annotations[localtype.AnnoSelectedNode]
		klog.Infof("[ProcessSnapshotPVC]source node is %s", srcNodeName)
		// step 4: check
		if srcNodeName != nodeName {
			return false, nil
		}
	}

	return true, nil
}

func ScoreInlineLVMVolume(pod *corev1.Pod, node *corev1.Node, ctx *algorithm.SchedulingContext) (score int, units []cache.AllocatedUnit, err error) {
	if pod != nil {
		klog.Infof("allocating lvm volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	fits, units, err := HandleInlineLVMVolume(ctx, node, pod)
	if err != nil {
		return MinScore, units, err
	}
	if !fits {
		return MinScore, units, nil
	}

	cacheVGsMap, err := GetNodeVGMap(node, ctx)
	if err != nil {
		return MinScore, units, err
	}
	score = ScoreLVM(units, cacheVGsMap)
	return score, units, nil
}

func ScoreLVMVolume(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (score int, units []cache.AllocatedUnit, err error) {
	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		klog.Infof("allocating lvm volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	fits, units, err := ProcessLVMPVCPriority(pod, pvcs, node, ctx)
	if err != nil {
		return MinScore, units, err
	}
	if !fits {
		return MinScore, units, nil
	}

	cacheVGsMap, err := GetNodeVGMap(node, ctx)
	if err != nil {
		return MinScore, units, err
	}
	score = ScoreLVM(units, cacheVGsMap)
	return score, units, nil
}

func ProcessLVMPVCPriority(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	pvcsWithVG, pvcsWithoutVG := DivideLVMPVCs(pvcs, ctx)
	cacheVGsMap, err := GetNodeVGMap(node, ctx)
	if err != nil {
		return false, units, err
	}

	// process pvcsWithVG first
	for _, pvc := range pvcsWithVG {
		vgName, err := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers.StorageClasses().Lister())
		if err != nil {
			return false, units, err
		}
		if _, ok := cacheVGsMap[cache.ResourceName(vgName)]; !ok {
			return false, units, fmt.Errorf("no vg named %s on node %s", vgName, node.Name)
		}

		requestedSize := utils.GetPVCRequested(pvc)
		freeSize := cacheVGsMap[cache.ResourceName(vgName)].Capacity - cacheVGsMap[cache.ResourceName(vgName)].Requested
		quanFree := resource.NewQuantity(freeSize, resource.BinarySI)
		quanReq := resource.NewQuantity(requestedSize, resource.BinarySI)
		if freeSize < requestedSize {
			if pod == nil {
				return false, units, fmt.Errorf("not enough lv storage on %s/%s, requested size %s,  free size %s",
					node.Name, vgName, quanReq.String(), quanFree.String())
			}
			return false, units, fmt.Errorf("not enough lv storage on %s/%s for pod %s/%s, requested size %s,  free size %s",
				node.Name, vgName, pod.Namespace, pod.Name, quanReq.String(), quanFree.String())
		}
		tmp := cacheVGsMap[cache.ResourceName(vgName)]
		tmp.Requested += requestedSize
		cacheVGsMap[cache.ResourceName(vgName)] = tmp
		u := cache.AllocatedUnit{
			NodeName:   node.Name,
			VolumeType: localtype.VolumeTypeLVM,
			Requested:  requestedSize,
			Allocated:  requestedSize, // for LVM requested is always equal to allocated
			VgName:     string(vgName),
			Device:     "",
			MountPoint: "",
			PVCName:    utils.PVCName(pvc),
		}
		units = append(units, u)
	}

	// process pvcsWithoutVG(default strategy: Binpack)
	for _, pvc := range pvcsWithoutVG {
		switch localtype.SchedulerStrategy {
		case localtype.StrategyBinpack:
			fits, tmpunits, err := Binpack(pod, pvc, node, cacheVGsMap)
			if !fits {
				return false, units, err
			}
			units = append(units, tmpunits...)
		case localtype.StrategySpread:
			fits, tmpunits, err := Spread(pod, pvc, node, cacheVGsMap)
			if !fits {
				return false, units, err
			}
			units = append(units, tmpunits...)
		}
	}
	if len(units) <= 0 {
		return false, units, nil
	}
	return true, units, nil
}

func Binpack(pod *corev1.Pod, pvc *corev1.PersistentVolumeClaim, node *corev1.Node, cacheVGsMap map[cache.ResourceName]cache.SharedResource) (fits bool, units []cache.AllocatedUnit, err error) {
	if len(cacheVGsMap) == 0 {
		return false, units, fmt.Errorf("no vg on node %s,", node.Name)
	}
	requestedSize := utils.GetPVCRequested(pvc)

	// make a copy slice of cacheVGsMap
	cacheVGsSlice := make([]cache.SharedResource, len(cacheVGsMap))
	for _, vg := range cacheVGsMap {
		cacheVGsSlice = append(cacheVGsSlice, vg)
	}

	// sort from small to large according to free size
	sort.Slice(cacheVGsSlice, func(i, j int) bool {
		return (cacheVGsSlice[i].Capacity - cacheVGsSlice[i].Requested) < (cacheVGsSlice[j].Capacity - cacheVGsSlice[j].Requested)
	})
	for i, vg := range cacheVGsSlice {
		freeSize := vg.Capacity - vg.Requested
		quanFree := resource.NewQuantity(freeSize, resource.BinarySI)
		quanReq := resource.NewQuantity(requestedSize, resource.BinarySI)
		if freeSize < requestedSize {
			if i == len(cacheVGsSlice)-1 {
				if pod == nil {
					return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s, requested size %s, max free size[VG: %s] %s, strategiy %s. you need to expand the vg",
						node.Name, quanReq.String(), vg.Name, quanFree.String(), localtype.SchedulerStrategy)

				}
				return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s for pod %s/%s, requested size %s, max free size[VG: %s] %s, strategiy %s. you need to expand the vg",
					node.Name, pod.Namespace, pod.Name, quanReq.String(), vg.Name, quanFree.String(), localtype.SchedulerStrategy)
			}
			continue
		}
		tmp := cacheVGsMap[cache.ResourceName(vg.Name)]
		tmp.Requested += requestedSize
		cacheVGsMap[cache.ResourceName(vg.Name)] = tmp
		u := cache.AllocatedUnit{
			NodeName:   node.Name,
			VolumeType: localtype.VolumeTypeLVM,
			Requested:  requestedSize,
			Allocated:  requestedSize, // for LVM requested is always equal to allocated
			VgName:     string(vg.Name),
			Device:     "",
			MountPoint: "",
			PVCName:    utils.PVCName(pvc),
		}
		units = append(units, u)
		break
	}

	return true, units, nil
}

func Spread(pod *corev1.Pod, pvc *corev1.PersistentVolumeClaim, node *corev1.Node, cacheVGsMap map[cache.ResourceName]cache.SharedResource) (fits bool, units []cache.AllocatedUnit, err error) {
	if len(cacheVGsMap) == 0 {
		return false, units, fmt.Errorf("no vg on node %s,", node.Name)
	}
	requestedSize := utils.GetPVCRequested(pvc)

	// make a copy slice of cacheVGsMap
	cacheVGsSlice := make([]cache.SharedResource, len(cacheVGsMap))
	for _, vg := range cacheVGsMap {
		cacheVGsSlice = append(cacheVGsSlice, vg)
	}

	// sort from large to small according to free size
	sort.Slice(cacheVGsSlice, func(i, j int) bool {
		return (cacheVGsSlice[i].Capacity - cacheVGsSlice[i].Requested) > (cacheVGsSlice[j].Capacity - cacheVGsSlice[j].Requested)
	})
	// the free size of cacheVGsSlice[0] is largest
	freeSize := cacheVGsSlice[0].Capacity - cacheVGsSlice[0].Requested
	quanFree := resource.NewQuantity(freeSize, resource.BinarySI)
	quanReq := resource.NewQuantity(requestedSize, resource.BinarySI)
	if freeSize < requestedSize {
		if pod == nil {
			return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s/%s, requested size %s,  free size %s, strategiy %s. you need to expand the vg",
				node.Name, cacheVGsSlice[0].Name, quanReq.String(), quanFree.String(), localtype.SchedulerStrategy)
		}
		return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s/%s for pod %s/%s, requested size %s,  free size %s, strategiy %s. you need to expand the vg",
			node.Name, cacheVGsSlice[0].Name, pod.Namespace, pod.Name, quanReq.String(), quanFree.String(), localtype.SchedulerStrategy)
	}
	cacheVGsSlice[0].Requested += requestedSize
	u := cache.AllocatedUnit{
		NodeName:   node.Name,
		VolumeType: localtype.VolumeTypeLVM,
		Requested:  requestedSize,
		Allocated:  requestedSize, // for LVM requested is always equal to allocated
		VgName:     string(cacheVGsSlice[0].Name),
		Device:     "",
		MountPoint: "",
		PVCName:    utils.PVCName(pvc),
	}
	units = append(units, u)

	return true, units, nil
}

func ScoreLVM(units []cache.AllocatedUnit, cacheVGsMap map[cache.ResourceName]cache.SharedResource) (score int) {
	if len(units) == 0 {
		return MinScore
	}
	// make a map store VG size pvcs used
	// key: VG name
	// value: used size
	scoreMap := make(map[string]int64)
	for _, unit := range units {
		if size, ok := scoreMap[unit.VgName]; ok {
			size += unit.Allocated
			scoreMap[unit.VgName] = size
		} else {
			scoreMap[unit.VgName] = unit.Allocated
		}
	}

	// score
	var scoref float64 = 0
	count := 0
	switch localtype.SchedulerStrategy {
	case localtype.StrategyBinpack:
		for vg, used := range scoreMap {
			scoref += float64(used) / float64(cacheVGsMap[cache.ResourceName(vg)].Capacity)
			count++
		}
	case localtype.StrategySpread:
		for vg, used := range scoreMap {
			scoref += (1.0 - float64(used)/float64(cacheVGsMap[cache.ResourceName(vg)].Capacity))
			count++
		}
	}
	score = int(scoref / float64(count) * float64(MaxScore))

	return
}

func ScoreMountPointVolume(
	pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node,
	ctx *algorithm.SchedulingContext) (score int, units []cache.AllocatedUnit, err error) {

	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		klog.Infof("allocating mount point volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	klog.V(6).Infof("pvcs: %#v, node: %#v", pvcs, node)
	fits, units, err := ProcessMPPVC(pod, pvcs, node, ctx)
	if err != nil {
		return MinScore, units, err
	}
	if !fits {
		return MinScore, units, nil
	}
	score = ScoreMP(units)

	return score, units, nil
}

func ScoreMP(units []cache.AllocatedUnit) (score int) {
	if len(units) == 0 {
		return MinScore
	}
	// score
	var scoref float64 = 0
	for _, unit := range units {
		scoref += float64(unit.Requested) / float64(unit.Allocated)
	}
	score = int(scoref / float64(len(units)) * float64(MaxScore))
	return
}

func ScoreDeviceVolume(
	pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node,
	ctx *algorithm.SchedulingContext) (score int, units []cache.AllocatedUnit, err error) {
	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		klog.Infof("allocating device volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	klog.V(6).Infof("pvcs: %#v, node: %#v", pvcs, node)

	fits, units, err := ProcessDevicePVC(pod, pvcs, node, ctx)
	if err != nil {
		return MinScore, units, err
	}
	if !fits {
		return MinScore, units, nil
	}

	score = ScoreDevice(units)

	return score, units, nil
}

func ScoreDevice(units []cache.AllocatedUnit) (score int) {
	if len(units) == 0 {
		return MinScore
	}
	// score
	var scoref float64 = 0
	for _, unit := range units {
		scoref += float64(unit.Requested) / float64(unit.Allocated)
	}

	score = int(scoref / float64(len(units)) * float64(MaxScore))
	return score
}
