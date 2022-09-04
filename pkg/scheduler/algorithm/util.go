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

package algorithm

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	storagev1informers "k8s.io/client-go/informers/storage/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	log "k8s.io/klog/v2"
)

//GetPodPvcs returns the pending pvcs which are needed for scheduling
func GetPodPvcs(pod *corev1.Pod, ctx *SchedulingContext, skipBound bool, containReadonlySnapshot bool) (
	err error,
	lvmPVCs []*corev1.PersistentVolumeClaim,
	mpPVCs []*corev1.PersistentVolumeClaim,
	devicePVCs []*corev1.PersistentVolumeClaim) {
	return GetPodPvcsByLister(pod, ctx.CoreV1Informers.PersistentVolumeClaims().Lister(), ctx.StorageV1Informers.StorageClasses().Lister(), skipBound, containReadonlySnapshot)
}

func GetPodUnboundPvcs(pvc *corev1.PersistentVolumeClaim, ctx *SchedulingContext, containReadonlySnapshot bool) (
	err error,
	lvmPVCs []*corev1.PersistentVolumeClaim,
	mpPVCs []*corev1.PersistentVolumeClaim,
	devicePVCs []*corev1.PersistentVolumeClaim) {
	pvcName := utils.PVCName(pvc)
	podName := ctx.ClusterNodeCache.PvcMapping.PvcPod[pvcName]
	if podName == "" {
		return fmt.Errorf("pod associated with pvc %s is not yet in PvcPod mapping", pvcName), lvmPVCs, mpPVCs, devicePVCs
	}
	var pod *corev1.Pod
	pod, err = ctx.CoreV1Informers.Pods().Lister().Pods(strings.Split(podName, "/")[0]).Get(strings.Split(podName, "/")[1])
	if err != nil {
		log.Errorf("failed to get pod by name %s: %s", podName, err.Error())
		return
	}
	return GetPodPvcs(pod, ctx, true, containReadonlySnapshot)
}

func GetAllPodPvcs(pod *corev1.Pod, ctx *SchedulingContext, containReadonlySnapshot bool) ([]*corev1.PersistentVolumeClaim, error) {
	err, pvc1, pvc2, pvc3 := GetPodPvcs(pod, ctx, false, containReadonlySnapshot)
	if err != nil {
		log.Errorf("failed to get pod pvcs: %s", err.Error())
		return nil, err
	}
	pvcs := make([]*corev1.PersistentVolumeClaim, 0)
	pvcs = append(pvcs, pvc1...)
	pvcs = append(pvcs, pvc2...)
	pvcs = append(pvcs, pvc3...)
	return pvcs, err
}

func GetPodPvcsByLister(pod *corev1.Pod, pvcLister corelisters.PersistentVolumeClaimLister, scLister storagelisters.StorageClassLister, skipBound bool, containReadonlySnapshot bool) (
	err error,
	lvmPVCs []*corev1.PersistentVolumeClaim,
	mpPVCs []*corev1.PersistentVolumeClaim,
	devicePVCs []*corev1.PersistentVolumeClaim) {

	ns := pod.Namespace
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			name := v.PersistentVolumeClaim.ClaimName
			pvc, err := pvcLister.PersistentVolumeClaims(ns).Get(name)
			if err != nil {
				log.Errorf("failed to get pvc by name %s/%s: %s", ns, name, err.Error())
				return err, lvmPVCs, mpPVCs, devicePVCs
			}
			if pvc.Status.Phase == corev1.ClaimBound && skipBound {
				log.Infof("skip scheduling bound pvc %s/%s", pvc.Namespace, pvc.Name)
				continue
			}
			scName := pvc.Spec.StorageClassName
			if scName == nil {
				continue
			}
			_, err = scLister.Get(*scName)
			if err != nil {
				log.Errorf("failed to get storage class by name %s: %s", *scName, err.Error())
				return err, lvmPVCs, mpPVCs, devicePVCs
			}
			var isLocalPV bool
			var pvType pkg.VolumeType
			if isLocalPV, pvType = utils.IsLocalPVC(pvc, scLister, containReadonlySnapshot); isLocalPV {
				switch pvType {
				case pkg.VolumeTypeLVM:
					log.V(4).Infof("got pvc %s/%s as lvm pvc", pvc.Namespace, pvc.Name)
					lvmPVCs = append(lvmPVCs, pvc)
				case pkg.VolumeTypeMountPoint:
					log.V(4).Infof("got pvc %s/%s as mount point pvc", pvc.Namespace, pvc.Name)
					mpPVCs = append(mpPVCs, pvc)
				case pkg.VolumeTypeDevice:
					log.V(4).Infof("got pvc %s/%s as device pvc", pvc.Namespace, pvc.Name)
					devicePVCs = append(devicePVCs, pvc)
				default:
					log.Infof("not a open-local pvc %s/%s, should handled by other provisioner", pvc.Namespace, pvc.Name)
				}
			}
		}
	}
	return
}

func IsLocalNode(nodeName string, ctx *SchedulingContext) bool {
	nodeCache := ctx.ClusterNodeCache.GetNodeCache(nodeName)

	if nodeCache == nil {
		return false
	}

	if len(nodeCache.VGs) != 0 || len(nodeCache.MountPoints) != 0 || len(nodeCache.Devices) != 0 {
		return true
	}

	return false
}

func ExtractPVCKey(pv *corev1.PersistentVolume) (string, error) {
	if pv.Spec.ClaimRef == nil {
		return "", fmt.Errorf("nil ClaimRef for pv %s", pv.Name)
	}
	return fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name), nil
}

/*
	ConvertAUFromPV convert an AllocatedUnit from an bound open-local PV
	we assure the pv is a valid open-local pv
*/
func ConvertAUFromPV(pv *corev1.PersistentVolume, scInformer storagev1informers.Interface, coreInformer corev1informers.Interface) (*cache.AllocatedUnit, error) {
	_, nodeName := utils.IsLocalPV(pv)
	containReadonlySnapshot := false
	_, volumeType := utils.IsOpenLocalPV(pv, containReadonlySnapshot)
	requested := utils.GetPVSize(pv)
	allocated := requested
	vgName := utils.GetVGNameFromCsiPV(pv)
	device := utils.GetDeviceNameFromCsiPV(pv)
	mountPoint := utils.GetMountPointFromCsiPV(pv)
	return &cache.AllocatedUnit{
		NodeName: nodeName,
		// currently we do not care abort the volume type of au
		VolumeType: volumeType,
		Requested:  requested,
		Allocated:  allocated,
		VgName:     vgName,
		Device:     device,
		MountPoint: mountPoint,
		PVCName:    utils.PVCName(pv),
	}, nil
}
