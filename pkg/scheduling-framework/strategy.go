/*
Copyright 2022/8/25 Alibaba Cloud.

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
package plugin

import (
	"fmt"
	"sort"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"

	"k8s.io/klog/v2"
)

var _ AllocateStrategy = &BinPackStrategy{}
var _ AllocateStrategy = &SpreadStrategy{}

func GetAllocateStrategy(c *OpenLocalArg) AllocateStrategy {
	if c == nil {
		return NewBinPackStrategy()
	}
	switch c.SchedulerStrategy {
	case string(localtype.StrategyBinpack):
		return NewBinPackStrategy()
	case string(localtype.StrategySpread):
		return NewSpreadStrategy()
	default:
		return NewBinPackStrategy()
	}
}

type AllocateStrategy interface {
	AllocatePVCWithoutVgName(nodeName string, vgStates *[]*cache.VGStoragePool, pvcInfo *LVMPVCInfo) (*cache.LVMPVAllocated, error)
	ScoreLVM(nodeAllocate *cache.NodeAllocateState) (score int64)
	ScoreDeviceCount(nodeAllocate *cache.NodeAllocateState) (score int64)
}

type vgSortFunc func(vgStateList []*cache.VGStoragePool)
type scoreWeightByVgCapacityFunc func(allocatedVG cache.VGStoragePool) float64
type scoreByCountFunc func(allocateCount, freeCount int) (score int64)

type BinPackStrategy struct {
	vgSortFunc      vgSortFunc
	scoreWeightFunc scoreWeightByVgCapacityFunc
	scoreCountFunc  scoreByCountFunc
}

func NewBinPackStrategy() *BinPackStrategy {
	return &BinPackStrategy{
		vgSortFunc: func(vgStateList []*cache.VGStoragePool) {
			sort.Slice(vgStateList, func(i, j int) bool {
				return (vgStateList[i].Allocatable - vgStateList[i].Requested) < (vgStateList[j].Allocatable - vgStateList[j].Requested)
			})
		},
		scoreWeightFunc: func(allocatedVG cache.VGStoragePool) float64 {
			return float64(allocatedVG.Requested) / float64(allocatedVG.Allocatable)
		},
		scoreCountFunc: func(allocateCount, freeCount int) (score int64) {
			return int64(float64(allocateCount) * float64(utils.MaxScore) / float64(freeCount))
		},
	}
}

func (s *BinPackStrategy) AllocatePVCWithoutVgName(nodeName string, vgStates *[]*cache.VGStoragePool, pvcInfo *LVMPVCInfo) (*cache.LVMPVAllocated, error) {
	return allocatePVCWithoutVgName(nodeName, vgStates, pvcInfo, s.vgSortFunc)
}

func (s *BinPackStrategy) ScoreLVM(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scoreLVM(nodeAllocate, s.scoreWeightFunc)
}

func (s *BinPackStrategy) ScoreDeviceCount(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scoreDeviceCount(nodeAllocate, s.scoreCountFunc)

}

type SpreadStrategy struct {
	vgSortFunc      vgSortFunc
	scoreWeightFunc scoreWeightByVgCapacityFunc
	scoreCountFunc  scoreByCountFunc
}

func NewSpreadStrategy() *SpreadStrategy {
	return &SpreadStrategy{
		vgSortFunc: func(vgStateList []*cache.VGStoragePool) {
			sort.Slice(vgStateList, func(i, j int) bool {
				return (vgStateList[i].Allocatable - vgStateList[i].Requested) > (vgStateList[j].Allocatable - vgStateList[j].Requested)
			})
		},
		scoreWeightFunc: func(allocatedVG cache.VGStoragePool) float64 {
			return 1.0 - float64(allocatedVG.Requested)/float64(allocatedVG.Allocatable)
		},
		scoreCountFunc: func(allocateCount, freeCount int) (score int64) {
			score = int64((1.0 - float64(allocateCount)/float64(freeCount)) * float64(utils.MaxScore))
			return
		},
	}
}

func (s *SpreadStrategy) AllocatePVCWithoutVgName(nodeName string, vgStates *[]*cache.VGStoragePool, pvcInfo *LVMPVCInfo) (*cache.LVMPVAllocated, error) {
	return allocatePVCWithoutVgName(nodeName, vgStates, pvcInfo, s.vgSortFunc)
}

func (s *SpreadStrategy) ScoreLVM(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scoreLVM(nodeAllocate, s.scoreWeightFunc)
}

func (s *SpreadStrategy) ScoreDeviceCount(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scoreDeviceCount(nodeAllocate, s.scoreCountFunc)

}

func allocatePVCWithoutVgName(nodeName string, vgStates *[]*cache.VGStoragePool, pvcInfo *LVMPVCInfo, vgSortFunc vgSortFunc) (*cache.LVMPVAllocated, error) {
	if vgStates == nil {
		err := fmt.Errorf("allocate for pvc(%s) fail, no vg found on node(%s)", utils.PVCName(pvcInfo.pvc), nodeName)
		klog.Error(err)
		return nil, err
	}
	vgStateList := *vgStates

	// sort by free size
	vgSortFunc(vgStateList)

	for j, vg := range vgStateList {
		poolFreeSize := vg.Allocatable - vg.Requested
		klog.V(6).Infof("validating node(%s) vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", nodeName, vg.Name, poolFreeSize, utils.PVCName(pvcInfo.pvc), pvcInfo.request)

		if poolFreeSize < pvcInfo.request {
			continue
		}
		vgStateList[j].Requested += pvcInfo.request
		return &cache.LVMPVAllocated{
			BasePVAllocated: cache.BasePVAllocated{
				PVCName:      pvcInfo.pvc.Name,
				PVCNamespace: pvcInfo.pvc.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.request,
				Allocated:    pvcInfo.request,
			},
			VGName: vg.Name,
		}, nil
	}
	return nil, fmt.Errorf("allocate for pvc(%s) fail, all vg(%+v) allocate fail on node(%s)", utils.PVCName(pvcInfo.pvc), vgStateList, nodeName)
}

func scoreLVM(nodeAllocate *cache.NodeAllocateState, scoreWeightFunc scoreWeightByVgCapacityFunc) (score int64) {
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

	allocateVGs := cache.VGStates{}
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

func scoreDeviceCount(nodeAllocate *cache.NodeAllocateState, scoreCountFunc scoreByCountFunc) (score int64) {

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
