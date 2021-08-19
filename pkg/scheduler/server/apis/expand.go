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

package apis

import (
	"fmt"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// ExpandPVC tries to reserve new size for PV regarding to the PVC
func ExpandPVC(ctx *algorithm.SchedulingContext, pvc *corev1.PersistentVolumeClaim) error {
	log.Infof("expanding pvc %s/%s", pvc.Namespace, pvc.Name)
	isBound := pvc.Status.Phase == corev1.ClaimBound

	if !isBound {
		err := fmt.Errorf("pvc phase is not as expected: %s != %s", pvc.Status.Phase, corev1.ClaimBound)
		log.Error(err.Error())
		return err
	}
	name := utils.GetPVFromBoundPVC(pvc)
	if len(name) == 0 {
		err := fmt.Errorf("failed to get PV for pvc %s/%s", pvc.Namespace, pvc.Name)
		log.Errorf(err.Error())
		return err
	}
	pv, err := ctx.CoreV1Informers.PersistentVolumes().Lister().Get(name)
	if err != nil {
		log.Errorf(err.Error())
		return err
	}
	newSize := utils.GetPVCRequested(pvc)
	oldSize := utils.GetPVSize(pv)
	if newSize <= oldSize {
		log.Infof("pvc %s/%s size and PV size %s is equal, both are %d", pvc.Namespace, pvc.Name, pv.Name, oldSize)
		return nil
	}

	log.Infof("pvc %s/%s old size is %d, new size %d", pvc.Namespace, pvc.Name, oldSize, newSize)
	nodeName := ctx.ClusterNodeCache.GetNodeNameFromPV(pv)
	nc := ctx.ClusterNodeCache.GetNodeCache(nodeName)
	if nc == nil {
		err := fmt.Errorf("error getting node cache via node %s", nodeName)
		log.Errorf(err.Error())
		return err
	}
	containReadonlySnapshot := false
	isLSS, lssType := utils.IsLSSPV(pv, ctx.StorageV1Informers, ctx.CoreV1Informers, containReadonlySnapshot)
	if !isLSS {
		err := fmt.Errorf("unable to expand non-open-local PV %s", pv.Name)
		log.Errorf(err.Error())
		return err
	}
	switch pkg.VolumeType(lssType) {
	case pkg.VolumeTypeLVM:
		vg := utils.GetVGNameFromCsiPV(pv)
		if vg == "" {
			return fmt.Errorf("vgName is empty for pv %s", pv.Name)
		}
		if vgCache, ok := nc.VGs[cache.ResourceName(vg)]; ok {
			newRequested := vgCache.Requested + int64(newSize-oldSize)
			log.Infof("matching pvc %s/%s on vg %s(left=%d bytes), ", pvc.Namespace, pvc.Name, vg, vgCache.Capacity-vgCache.Requested)
			if newRequested > vgCache.Capacity {
				err := fmt.Errorf("failed to extend pvc, vg %s is not enough, requested total %d, capacity %d", vg, newRequested, vgCache.Capacity)
				return err
			}
			vgCache.Requested += newSize - oldSize
			nc.VGs[cache.ResourceName(vg)] = vgCache
			return nil
		} else {
			err := fmt.Errorf("vg cache is not found for VG %s", vg)
			return err
		}
	case pkg.VolumeTypeMountPoint:
		return fmt.Errorf("expansion on mountpoint volume is not supported")
	case pkg.VolumeTypeDevice:
		return fmt.Errorf("expansion on Device volume is not supported")
	case pkg.VolumeTypeQuota:
		return fmt.Errorf("expansion on Quota volume is not supported")
	}
	return fmt.Errorf("unhandled error during volume expansion")

}
