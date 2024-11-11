/*
Copyright 2022/9/13 Alibaba Cloud.

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
package cache

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	"k8s.io/klog/v2"
)

type VGScheduleStrategy interface {
	AllocateForPVCWithoutVgName(nodeName string, vgStates *[]*VGStoragePool, pvcInfo *LVMPVCInfo) (*LVMPVAllocated, error)
	ScoreByCapacity(nodeAllocate *NodeAllocateState) (score int64)
}

type vgSortFunc func(vgStateList []*VGStoragePool)
type vgScoreWeightFunc func(allocatedVG VGStoragePool) float64

var _ VGScheduleStrategy = &vgScheduleBinpackStrategy{}
var _ VGScheduleStrategy = &vgScheduleSpreadStrategy{}

func GetVGScheduleStrategy(strategy pkg.StrategyType) VGScheduleStrategy {
	switch strategy {
	case pkg.StrategyBinpack:
		return NewVGScheduleBinpackStrategy()
	case pkg.StrategySpread:
		return NewVGScheduleSpreadStrategy()
	default:
		return NewVGScheduleBinpackStrategy()
	}
}

type vgScheduleBinpackStrategy struct {
	vgSortFunc      vgSortFunc
	scoreWeightFunc vgScoreWeightFunc
}

func NewVGScheduleBinpackStrategy() *vgScheduleBinpackStrategy {
	return &vgScheduleBinpackStrategy{
		vgSortFunc: func(vgStateList []*VGStoragePool) {
			sort.Slice(vgStateList, func(i, j int) bool {
				return (vgStateList[i].Allocatable - vgStateList[i].Requested) < (vgStateList[j].Allocatable - vgStateList[j].Requested)
			})
		},
		scoreWeightFunc: func(allocatedVG VGStoragePool) float64 {
			return float64(allocatedVG.Requested) / float64(allocatedVG.Allocatable)
		},
	}
}

func (s *vgScheduleBinpackStrategy) AllocateForPVCWithoutVgName(nodeName string, vgStates *[]*VGStoragePool, pvcInfo *LVMPVCInfo) (*LVMPVAllocated, error) {
	return allocatePVCWithoutVgName(nodeName, vgStates, pvcInfo, s.vgSortFunc)
}

func (s *vgScheduleBinpackStrategy) ScoreByCapacity(nodeAllocate *NodeAllocateState) (score int64) {
	return scoreByCapacity(nodeAllocate, s.scoreWeightFunc)
}

type vgScheduleSpreadStrategy struct {
	vgSortFunc      vgSortFunc
	scoreWeightFunc vgScoreWeightFunc
}

func NewVGScheduleSpreadStrategy() *vgScheduleSpreadStrategy {
	return &vgScheduleSpreadStrategy{
		vgSortFunc: func(vgStateList []*VGStoragePool) {
			sort.Slice(vgStateList, func(i, j int) bool {
				return (vgStateList[i].Allocatable - vgStateList[i].Requested) > (vgStateList[j].Allocatable - vgStateList[j].Requested)
			})
		},
		scoreWeightFunc: func(allocatedVG VGStoragePool) float64 {
			return 1.0 - float64(allocatedVG.Requested)/float64(allocatedVG.Allocatable)
		},
	}
}

func (s *vgScheduleSpreadStrategy) AllocateForPVCWithoutVgName(nodeName string, vgStates *[]*VGStoragePool, pvcInfo *LVMPVCInfo) (*LVMPVAllocated, error) {
	return allocatePVCWithoutVgName(nodeName, vgStates, pvcInfo, s.vgSortFunc)
}

func (s *vgScheduleSpreadStrategy) ScoreByCapacity(nodeAllocate *NodeAllocateState) (score int64) {
	return scoreByCapacity(nodeAllocate, s.scoreWeightFunc)
}

func allocatePVCWithoutVgName(nodeName string, vgStates *[]*VGStoragePool, pvcInfo *LVMPVCInfo, vgSortFunc vgSortFunc) (*LVMPVAllocated, error) {
	if vgStates == nil {
		err := fmt.Errorf("allocate for pvc(%s) fail, no vg found on node(%s)", utils.PVCName(pvcInfo.PVC), nodeName)
		klog.Error(err)
		return nil, err
	}
	vgStateList := *vgStates

	// sort by free size
	vgSortFunc(vgStateList)

	for j, vg := range vgStateList {
		poolFreeSize := vg.Allocatable - vg.Requested
		klog.V(6).Infof("validating node(%s) vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", nodeName, vg.Name, poolFreeSize, utils.PVCName(pvcInfo.PVC), pvcInfo.Request)

		if poolFreeSize < pvcInfo.Request {
			continue
		}
		vgStateList[j].Requested += pvcInfo.Request
		return &LVMPVAllocated{
			BasePVAllocated: BasePVAllocated{
				PVCName:      pvcInfo.PVC.Name,
				PVCNamespace: pvcInfo.PVC.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.Request,
				Allocated:    pvcInfo.Request,
			},
			VGName: vg.Name,
		}, nil
	}
	return nil, fmt.Errorf("allocate for pvc(%s) fail, all vg allocate fail on node(%s): %s", utils.PVCName(pvcInfo.PVC), nodeName, vgStateListToString(vgStateList))
}

func vgStateListToString(vgStates []*VGStoragePool) string {
	var info string
	for _, vgState := range vgStates {
		info = strings.Join([]string{info, fmt.Sprintf("%v", vgState)}, ",")
	}
	return info
}

func scoreByCapacity(nodeAllocate *NodeAllocateState, scoreWeightFunc vgScoreWeightFunc) (score int64) {
	if nodeAllocate == nil || nodeAllocate.Units == nil || nodeAllocate.NodeStorageAllocatedByUnits == nil {
		return int64(utils.MinScore)
	}

	if len(nodeAllocate.Units.InlineVolumeAllocateUnits)+len(nodeAllocate.Units.LVMPVCAllocateUnits) <= 0 {
		return int64(utils.MinScore)
	}

	allVGs := nodeAllocate.NodeStorageAllocatedByUnits.VGStates
	if len(allVGs) <= 0 {
		return int64(utils.MinScore)
	}

	allocateVGs := VGStates{}
	for _, inlineUnit := range nodeAllocate.Units.InlineVolumeAllocateUnits {
		if _, ok := allVGs[inlineUnit.VgName]; ok {
			allocateVGs[inlineUnit.VgName] = allVGs[inlineUnit.VgName]
		}
	}

	for _, pvcUnit := range nodeAllocate.Units.LVMPVCAllocateUnits {
		if _, ok := allVGs[pvcUnit.VGName]; ok {
			allocateVGs[pvcUnit.VGName] = allVGs[pvcUnit.VGName]
		}
	}

	// score
	var scoref float64 = 0
	count := 0
	for _, allocatedVG := range allocateVGs {
		scoref += scoreWeightFunc(*allocatedVG)
		count++
	}
	score = int64(scoref / float64(count) * float64(utils.MaxScore))
	return
}

func GetDeviceScheduleStrategy(strategy pkg.StrategyType) DeviceScheduleStrategy {
	switch strategy {
	case pkg.StrategyBinpack:
		return NewDeviceScheduleBinpackStrategy()
	case pkg.StrategySpread:
		return NewDeviceScheduleSpreadStrategy()
	default:
		return NewDeviceScheduleBinpackStrategy()
	}
}

type DeviceScheduleStrategy interface {
	ScoreByCount(nodeAllocate *NodeAllocateState) (score int64)
}

var _ DeviceScheduleStrategy = &DeviceScheduleBinpackStrategy{}
var _ DeviceScheduleStrategy = &DeviceScheduleSpreadStrategy{}

type scoreByCountFunc func(allocateCount, freeCount int) (score int64)

type DeviceScheduleBinpackStrategy struct {
	scoreCountFunc scoreByCountFunc
}

func NewDeviceScheduleBinpackStrategy() *DeviceScheduleBinpackStrategy {
	return &DeviceScheduleBinpackStrategy{
		scoreCountFunc: func(allocateCount, freeCount int) (score int64) {
			return int64(float64(allocateCount) * float64(utils.MaxScore) / float64(freeCount))
		},
	}
}

func (s *DeviceScheduleBinpackStrategy) ScoreByCount(nodeAllocate *NodeAllocateState) (score int64) {
	return scoreDeviceCount(nodeAllocate, s.scoreCountFunc)

}

type DeviceScheduleSpreadStrategy struct {
	scoreCountFunc scoreByCountFunc
}

func NewDeviceScheduleSpreadStrategy() *DeviceScheduleBinpackStrategy {
	return &DeviceScheduleBinpackStrategy{
		scoreCountFunc: func(allocateCount, freeCount int) (score int64) {
			score = int64((1.0 - float64(allocateCount)/float64(freeCount)) * float64(utils.MaxScore))
			return
		},
	}
}

func (s *DeviceScheduleSpreadStrategy) ScoreByCount(nodeAllocate *NodeAllocateState) (score int64) {
	return scoreDeviceCount(nodeAllocate, s.scoreCountFunc)

}

func scoreDeviceCount(nodeAllocate *NodeAllocateState, scoreCountFunc scoreByCountFunc) (score int64) {

	var deviceFreeCountBeforeAllocate = 0
	for _, state := range nodeAllocate.NodeStorageAllocatedByUnits.DeviceStates {
		if !state.IsAllocated {
			deviceFreeCountBeforeAllocate += 1
		}
	}
	deviceAllocatedCountThisTime := len(nodeAllocate.Units.DevicePVCAllocateUnits)
	deviceFreeCountBeforeAllocate += deviceAllocatedCountThisTime

	if deviceAllocatedCountThisTime <= 0 || deviceFreeCountBeforeAllocate <= 0 {
		return int64(utils.MinScore)
	}
	score = scoreCountFunc(deviceAllocatedCountThisTime, deviceFreeCountBeforeAllocate)
	return

}
