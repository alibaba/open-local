/*
Copyright 2022/8/23 Alibaba Cloud.

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
	"github.com/alibaba/open-local/pkg/utils"
	"k8s.io/klog/v2"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
)

type Scorer interface {
	ScoreByCapacity(nodeAllocate *cache.NodeAllocateState) (score int64)
	ScoreByCount(nodeAllocate *cache.NodeAllocateState) (score int64)
	ScoreByNodeAntiAffinity(nodeAllocate *cache.NodeAllocateState) (score int64)
}

type ScoreCalculator struct {
	scorers []Scorer
}

func NewScoreCalculator(strategy localtype.StrategyType, nodeAntiAffinityWeight *localtype.NodeAntiAffinityWeight) *ScoreCalculator {
	return &ScoreCalculator{
		scorers: []Scorer{NewVolumeGroupScorer(strategy), NewDeviceScorer(strategy, nodeAntiAffinityWeight)},
	}
}

func (scorer *ScoreCalculator) Score(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scorer.ScoreByCapacity(nodeAllocate) + scorer.ScoreByCount(nodeAllocate) + scorer.ScoreByNodeAntiAffinity(nodeAllocate)
}

func (scorer *ScoreCalculator) ScoreByCapacity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	score = 0
	for _, scorer := range scorer.scorers {
		score += scorer.ScoreByCapacity(nodeAllocate)
	}
	return score
}

func (scorer *ScoreCalculator) ScoreByCount(nodeAllocate *cache.NodeAllocateState) (score int64) {
	score = 0
	for _, scorer := range scorer.scorers {
		score += scorer.ScoreByCount(nodeAllocate)
	}
	return score
}

func (scorer *ScoreCalculator) ScoreByNodeAntiAffinity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	score = 0
	for _, scorer := range scorer.scorers {
		score += scorer.ScoreByNodeAntiAffinity(nodeAllocate)
	}
	return score
}

type VolumeGroupScorer struct {
	scoreStrategy cache.VGScheduleStrategy
}

func NewVolumeGroupScorer(strategy localtype.StrategyType) *VolumeGroupScorer {
	return &VolumeGroupScorer{
		scoreStrategy: cache.GetVGScheduleStrategy(strategy),
	}
}

func (scorer *VolumeGroupScorer) ScoreByCapacity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scorer.scoreStrategy.ScoreByCapacity(nodeAllocate)
}

func (scorer *VolumeGroupScorer) ScoreByCount(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return int64(utils.MinScore)
}

func (scorer *VolumeGroupScorer) ScoreByNodeAntiAffinity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return int64(utils.MinScore)
}

type DeviceScorer struct {
	scoreStrategy          cache.DeviceScheduleStrategy
	nodeAntiAffinityWeight *localtype.NodeAntiAffinityWeight
}

func NewDeviceScorer(strategy localtype.StrategyType, nodeAntiAffinityWeight *localtype.NodeAntiAffinityWeight) *DeviceScorer {
	return &DeviceScorer{
		scoreStrategy:          cache.GetDeviceScheduleStrategy(strategy),
		nodeAntiAffinityWeight: nodeAntiAffinityWeight,
	}
}
func (scorer *DeviceScorer) ScoreByCapacity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	units := nodeAllocate.Units.DevicePVCAllocateUnits
	if len(units) <= 0 {
		return int64(utils.MinScore)
	}

	var scoref float64 = 0
	for _, unit := range units {
		scoref += float64(unit.Requested) / float64(unit.Allocated)
	}
	score = int64(scoref / float64(len(units)) * float64(utils.MaxScore))
	return
}

func (scorer *DeviceScorer) ScoreByCount(nodeAllocate *cache.NodeAllocateState) (score int64) {
	return scorer.scoreStrategy.ScoreByCount(nodeAllocate)
}

func (scorer *DeviceScorer) ScoreByNodeAntiAffinity(nodeAllocate *cache.NodeAllocateState) (score int64) {
	var freeDeviceCount = 0
	for _, state := range nodeAllocate.NodeStorageAllocatedByUnits.DeviceStates {
		if !state.IsAllocated {
			freeDeviceCount += 1
		}
	}
	deviceWeight := scorer.nodeAntiAffinityWeight.Get(localtype.VolumeTypeDevice)
	if deviceWeight <= 0 {
		return int64(utils.MinScore)
	}

	if len(nodeAllocate.Units.DevicePVCAllocateUnits) <= 0 && (!nodeAllocate.NodeStorageAllocatedByUnits.IsLocal() || (freeDeviceCount <= 0)) {
		score = int64(deviceWeight * 1)
		klog.Infof("[NodeAntiAffinity]node %s got %d out of %d", nodeAllocate.NodeName, score, utils.MaxScore)
		return score
	}
	return int64(utils.MinScore)
}
