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

	"github.com/alibaba/open-local/pkg/scheduler/errors"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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
	log.Infof("allocating lvm volume for pod %s/%s", pod.Namespace, pod.Name)

	fits, units, err = ProcessLVMPVCPredicate(pvcs, node, ctx)
	if err != nil {
		return
	}

	if len(units) <= 0 {
		return false, units, nil
	}
	log.Infof("node %s is capable of lvm %d pvcs", node.Name, len(pvcs))
	log.Debugf("pod=%s/%s, node=%s, units:%#v", pod.Namespace, pod.Name, node.Name, units)
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
		vgName := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers)
		requestedSize := utils.GetPVCRequested(pvc)

		vg, ok := cacheVGsMap[cache.ResourceName(vgName)]
		if !ok {
			return false, units, errors.NewNotSuchVGError(vgName)
		}

		freeSize := vg.Capacity - vg.Requested
		log.Debugf("validating vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", vgName, freeSize, pvc.Name, requestedSize)

		if freeSize < requestedSize {
			return false, units, errors.NewInsufficientLVMError(requestedSize, vg.Requested, vg.Capacity)
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
			log.Debugf("validating vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", vg.Name, freeSize, pvc.Name, requestedSize)

			if freeSize < requestedSize {
				if i == len(cacheVGsSlice)-1 {
					return false, units, errors.NewInsufficientLVMError(requestedSize, vg.Requested, vg.Capacity)
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

// DivideLVMPVCs divide pvcs into pvcsWithVG and pvcsWithoutVG
func DivideLVMPVCs(pvcs []*corev1.PersistentVolumeClaim, ctx *algorithm.SchedulingContext) (pvcsWithVG, pvcsWithoutVG []*corev1.PersistentVolumeClaim) {
	for _, pvc := range pvcs {
		if vgName := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers); vgName == "" {
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
		log.Infof("allocating mount point volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	log.Debugf("pvcs: %#v, node: %#v", pvcs, node)
	fits, units, err = ProcessMPPVC(pod, pvcs, node, ctx)

	return fits, units, err
}

func ProcessMPPVC(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, units []cache.AllocatedUnit, err error) {
	pvcsWithTypeSSD, pvcsWithTypeHDD := DividePVCAccordingToMediaType(pvcs, ctx)
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
		return false, units, errors.NewInsufficientMountPointError(
			requestedSSDCount,
			freeMPSSDCount,
			totalCount,
			localtype.MediaTypeSSD)
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
		log.Infof("there is no enough mount point volume on node %s, want(cnt) %d, actual(cnt) %d, type hdd", node.Name, requestedHDDCount, freeMPHDDCount)
		return false, units, errors.NewInsufficientMountPointError(
			requestedHDDCount,
			freeMPHDDCount,
			totalCount,
			localtype.MediaTypeHHD)
	}
	fits, rstUnits, err = CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeMountPoint, freeMPHDD, pvcsWithTypeHDD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	log.Debugf("node %s is capable of mount point %d pvcs", node.Name, len(pvcs))
	return true, units, nil
}

// DividePVCAccordingToMediaType divide pvcs into pvcsWithSSD and pvcsWithHDD
func DividePVCAccordingToMediaType(pvcs []*corev1.PersistentVolumeClaim, ctx *algorithm.SchedulingContext) (pvcsWithTypeSSD, pvcsWithTypeHDD []*corev1.PersistentVolumeClaim) {
	for _, pvc := range pvcs {
		if mediaType := utils.GetMediaTypeFromPVC(pvc, ctx.StorageV1Informers); mediaType == localtype.MediaTypeSSD {
			pvcsWithTypeSSD = append(pvcsWithTypeSSD, pvc)
		} else if mediaType == localtype.MediaTypeHHD {
			pvcsWithTypeHDD = append(pvcsWithTypeHDD, pvc)
		} else if mediaType == "" {
			log.Infof("empty mediaType in storageClass! pvc: %s/%s", pvc.Namespace, pvc.Name)
		} else {
			log.Infof("mediaType %s not support! pvc: %s/%s", mediaType, pvc.Namespace, pvc.Name)
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
		} else if mp.MediaType == localtype.MediaTypeHHD && !mp.IsAllocated {
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

	var totalCount int64
	if resource == localtype.VolumeTypeDevice {
		totalCount, err = GetCacheDeviceCount(node, ctx)
	} else if resource == localtype.VolumeTypeMountPoint {
		totalCount, err = GetCacheMPCount(node, ctx)
	}
	if err != nil {
		return false, units, err
	}
	disksCount := int64(len(ers))
	pvcsCount := int64(len(pvcs))

	i := 0
	for j, er := range ers {
		if pvcsCount == 0 {
			break
		}
		pvc := pvcs[i]
		requestedSize := utils.GetPVCRequested(pvc)
		log.Debugf("%s: %s capacity=%d, requestedSize=%d", string(er.MediaType), er.Name, er.Capacity, requestedSize)
		if er.Capacity < requestedSize {
			if j == int(disksCount)-1 {
				return false, units, errors.NewInsufficientExclusiveResourceError(
					resource,
					pvcsCount,
					disksCount,
					totalCount)
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

		log.Debugf("found unit: %#v for pvc %#v", u, pvc)
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
		log.Infof("allocating device volume for pod %s/%s", pod.Namespace, pod.Name)
	}
	log.Debugf("pvcs: %#v, node: %#v", pvcs, node)
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
		} else if device.MediaType == localtype.MediaTypeHHD && !device.IsAllocated {
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
	pvcsWithTypeSSD, pvcsWithTypeHDD := DividePVCAccordingToMediaType(pvcs, ctx)
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
		return false, units, errors.NewInsufficientExclusiveResourceError(
			localtype.VolumeTypeDevice,
			requestedSSDCount,
			freeDeviceSSDCount,
			totalCount)
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
		return false, units, errors.NewInsufficientExclusiveResourceError(
			localtype.VolumeTypeDevice,
			requestedSSDCount,
			freeDeviceSSDCount,
			totalCount)
	}
	fits, rstUnits, err = CheckExclusiveResourceMeetsPVCSize(localtype.VolumeTypeDevice, freeDeviceHDD, pvcsWithTypeHDD, node, ctx)
	if err != nil {
		return false, rstUnits, err
	}
	if !fits {
		return false, rstUnits, nil
	}
	units = append(units, rstUnits...)

	log.Debugf("node %s is capable of mount point %d pvcs", node.Name, len(pvcs))
	return true, units, nil
}

// If there is no snapshot pvc, just return true
func ProcessSnapshotPVC(pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (fits bool, err error) {
	nodeName := node.Name
	for _, pvc := range pvcs {
		// step 0: check if is snapshot pvc
		if !utils.IsSnapshotPVC(pvc) {
			continue
		}
		log.Infof("[ProcessSnapshotPVC]data source of pvc %s/%s is snapshot", pvc.Namespace, pvc.Name)
		// step 1: get snapshot api
		snapName := pvc.Spec.DataSource.Name
		snapNamespace := pvc.Namespace
		snapshot, err := ctx.SnapshotInformers.VolumeSnapshots().Lister().VolumeSnapshots(snapNamespace).Get(snapName)
		if err != nil {
			return false, fmt.Errorf("[ProcessSnapshotPVC]get snapshot %s/%s failed: %s", snapNamespace, snapName, err.Error())
		}
		log.Infof("[ProcessSnapshotPVC]snapshot is %s", snapshot.Name)
		// step 2: get src pvc
		srcPVCName := *snapshot.Spec.Source.PersistentVolumeClaimName
		srcPVCNamespace := snapNamespace
		srcPVC, err := ctx.CoreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(srcPVCNamespace).Get(srcPVCName)
		if err != nil {
			return false, fmt.Errorf("[ProcessSnapshotPVC]get src pvc %s/%s failed: %s", snapNamespace, snapName, err.Error())
		}
		log.Infof("[ProcessSnapshotPVC]source pvc is %s/%s", srcPVC.Namespace, srcPVC.Name)
		// step 3: get src node name
		srcNodeName := srcPVC.Annotations[localtype.AnnSelectedNode]
		log.Infof("[ProcessSnapshotPVC]source node is %s", srcNodeName)
		// step 4: check
		if srcNodeName != nodeName {
			return false, nil
		}
	}

	return true, nil
}

func ScoreLVMVolume(pod *corev1.Pod, pvcs []*corev1.PersistentVolumeClaim, node *corev1.Node, ctx *algorithm.SchedulingContext) (score int, units []cache.AllocatedUnit, err error) {
	if len(pvcs) <= 0 {
		return
	}
	if pod != nil {
		log.Infof("allocating lvm volume for pod %s/%s", pod.Namespace, pod.Name)
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
		vgName := utils.GetVGNameFromPVC(pvc, ctx.StorageV1Informers)
		if _, ok := cacheVGsMap[cache.ResourceName(vgName)]; !ok {
			return false, units, fmt.Errorf("no vg named %q on node %q", vgName, node.Name)
		}

		requestedSize := utils.GetPVCRequested(pvc)
		freeSize := cacheVGsMap[cache.ResourceName(vgName)].Capacity - cacheVGsMap[cache.ResourceName(vgName)].Requested
		if freeSize < requestedSize {
			if pod == nil {
				return false, units, fmt.Errorf("not enough lv storage on %s/%s, requested size %d,  free size %d",
					node.Name, vgName, requestedSize, freeSize)
			}
			return false, units, fmt.Errorf("not enough lv storage on %s/%s for pod %s/%s, requested size %d,  free size %d",
				node.Name, vgName, pod.Namespace, pod.Name, requestedSize, freeSize)
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
		if freeSize < requestedSize {
			if i == len(cacheVGsSlice)-1 {
				if pod == nil {
					return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s, requested size %d, max free size[VG: %s] %d, strategiy %s",
						node.Name, requestedSize, vg.Name, freeSize, localtype.SchedulerStrategy)

				}
				return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s for pod %s/%s, requested size %d, max free size[VG: %s] %d, strategiy %s",
					node.Name, pod.Namespace, pod.Name, requestedSize, vg.Name, freeSize, localtype.SchedulerStrategy)
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
	if freeSize < requestedSize {
		if pod == nil {
			return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s/%s, requested size %d,  free size %d, strategiy %s",
				node.Name, cacheVGsSlice[0].Name, requestedSize, freeSize, localtype.SchedulerStrategy)
		}
		return false, units, fmt.Errorf("[multipleVGs]not enough lv storage on %s/%s for pod %s/%s, requested size %d,  free size %d, strategiy %s",
			node.Name, cacheVGsSlice[0].Name, pod.Namespace, pod.Name, requestedSize, freeSize, localtype.SchedulerStrategy)
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
		log.Infof("allocating mount point volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	log.Debugf("pvcs: %#v, node: %#v", pvcs, node)
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
		log.Infof("allocating device volume for pod %s/%s", pod.Namespace, pod.Name)
	}

	log.Debugf("pvcs: %#v, node: %#v", pvcs, node)

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
	// score
	var scoref float64 = 0
	for _, unit := range units {
		scoref += float64(unit.Requested) / float64(unit.Allocated)
	}

	score = int(scoref / float64(len(units)) * float64(MaxScore))
	return score
}
