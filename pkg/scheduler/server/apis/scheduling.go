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
	"time"

	utiltrace "k8s.io/utils/trace"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
)

// Scheduling is the interface for provisioner to request storage info
// reports error if failed to reserve storage for pvc
func SchedulingPVC(ctx *algorithm.SchedulingContext, pvc *corev1.PersistentVolumeClaim, node *corev1.Node) (*pkg.BindingInfo, error) {
	if node == nil {
		log.Infof("scheduling pvc %s/%s without node", pvc.Namespace, pvc.Name)
	} else {
		log.Infof("scheduling pvc %s/%s on node %s", pvc.Namespace, pvc.Name, node.Name)
	}
	trace := utiltrace.New(fmt.Sprintf("Scheduling[SchedulingPVC] %s/%s", pvc.Namespace, pvc.Name))
	defer trace.LogIfLong(50 * time.Millisecond)

	// serialize the api request from provisioner to keep cache consistency
	ctx.CtxLock.Lock()
	defer ctx.CtxLock.Unlock()

	pvcName := utils.PVCName(pvc)

	if node == nil {
		panic("cluster level(nodeName is nil) pvc allocation is not implemented yet")
	}

	if ctx.ClusterNodeCache.BindingInfo.IsPVCExists(pvcName) {
		log.Infof("%s is already allocated, returning existing", pvcName)
		return unitsToBinding(
			[]*corev1.PersistentVolumeClaim{pvc},
			[]cache.AllocatedUnit{*ctx.ClusterNodeCache.BindingInfo[pvcName]}), nil
	}
	if !ctx.ClusterNodeCache.PvcMapping.IsPodPvcReady(pvc) {
		msg := fmt.Sprintf("pvc %s is not eligible for provisioning as related pvcs are still pending", pvcName)
		log.Info(msg)
		return nil, fmt.Errorf(msg)
	}
	containReadonlySnapshot := false
	err, lvmPVCs, mpPVCs, devicePVCs := algorithm.GetPodUnboundPvcs(pvc, ctx, containReadonlySnapshot)
	if err != nil {
		log.Errorf("failed to get pod unbound pvcs: %s", err.Error())
		return nil, err
	}

	if len(lvmPVCs)+len(mpPVCs)+len(devicePVCs) == 0 {
		msg := "unexpected schedulering request for all pvcs are bounded"
		log.Info(msg)
		return nil, fmt.Errorf(msg)
	}
	var allocatedUnits []cache.AllocatedUnit
	trace.Step("Computing ScoreLVMVolume")
	if _, lvmUnits, err := algo.ScoreLVMVolume(nil /*do we need pod here*/, lvmPVCs, node, ctx); err != nil {
		err = fmt.Errorf("failed to allocate local storage for pvc %s/%s: %s", pvc.Namespace, pvc.Name, err.Error())
		log.Errorf(err.Error())
		return nil, err
	} else {
		allocatedUnits = append(allocatedUnits, lvmUnits...)
	}
	trace.Step("Computing ScoreMountPointVolume")
	if _, mpUnits, err := algo.ScoreMountPointVolume(nil /*do we need pod here*/, mpPVCs, node, ctx); err != nil {
		err = fmt.Errorf("failed to allocate local storage for pvc %s/%s: %s", pvc.Namespace, pvc.Name, err.Error())
		log.Errorf(err.Error())
		return nil, err
	} else {
		allocatedUnits = append(allocatedUnits, mpUnits...)
	}
	trace.Step("Computing ScoreDeviceVolume")
	if _, deviceUnits, err := algo.ScoreDeviceVolume(nil /*do we need pod here*/, devicePVCs, node, ctx); err != nil {
		err = fmt.Errorf("failed to allocate local storage for pvc %s/%s: %s", pvc.Namespace, pvc.Name, err.Error())
		log.Errorf(err.Error())
		return nil, err
	} else {
		allocatedUnits = append(allocatedUnits, deviceUnits...)
	}

	if (allocatedUnits == nil || len(allocatedUnits) <= 0) || len(allocatedUnits) != (len(lvmPVCs)+len(mpPVCs)+len(devicePVCs)) {
		log.Errorf("unexpected allocated unit number: %d", len(allocatedUnits))
		return nil, err
	}
	trace.Step("Computing Reserve")

	err = ctx.ClusterNodeCache.Assume(allocatedUnits)

	if err != nil {
		err = fmt.Errorf("failed to assume local storage for pvc %s/%s: %s", pvc.Namespace, pvc.Name, err.Error())
		log.Error(err.Error())
		return nil, err
	}
	var targetAllocateUnits []cache.AllocatedUnit
	log.V(6).Infof("allocatedUnits of pvc %s: %+v", pvcName, allocatedUnits)
	for _, unit := range allocatedUnits {
		newUnit := unit
		ctx.ClusterNodeCache.BindingInfo[newUnit.PVCName] = &newUnit
		if unit.PVCName == utils.PVCName(pvc) {
			targetAllocateUnits = append(targetAllocateUnits, unit)
		}
	}
	bindingInfo := unitsToBinding([]*corev1.PersistentVolumeClaim{pvc}, targetAllocateUnits)
	log.Infof("successfully schedule pvc %s/%s", pvc.Namespace, pvc.Name)
	return bindingInfo, nil
}

func unitsToBinding(pvcs []*corev1.PersistentVolumeClaim, units []cache.AllocatedUnit) *pkg.BindingInfo {
	if len(units) <= 0 || len(units) > 1 {
		log.Errorf("unexpected allocated unit number: %d", len(units))
		return nil
	}
	unit0 := units[0]
	pvc0 := pvcs[0]

	return &pkg.BindingInfo{
		Node:                  unit0.NodeName,
		Disk:                  unit0.MountPoint,
		VgName:                unit0.VgName,
		Device:                unit0.Device,
		VolumeType:            string(unit0.VolumeType),
		PersistentVolumeClaim: fmt.Sprintf("%s/%s", pvc0.Namespace, pvc0.Name),
	}
}
