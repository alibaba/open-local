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

func (plugin *LocalPlugin) scoreByCapacity(nodeAllocate *cache.NodeAllocateState) int64 {

	scoreByVgCapacity := plugin.scoreByVgCapacity(nodeAllocate)
	scoreByDeviceCapacity := plugin.scoreByDeviceCapacity(nodeAllocate)

	return scoreByVgCapacity + scoreByDeviceCapacity
}

func (plugin *LocalPlugin) scoreByVgCapacity(nodeAllocate *cache.NodeAllocateState) int64 {

	vgStates := nodeAllocate.NodeStorageAllocatedByUnits.VGStates
	if len(vgStates) <= 0 {
		return int64(utils.MinScore)
	}

	return plugin.allocateStrategy.ScoreLVM(nodeAllocate)
}

func (plugin *LocalPlugin) scoreByDeviceCapacity(nodeAllocate *cache.NodeAllocateState) (score int64) {

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

func (plugin *LocalPlugin) scoreByCount(nodeAllocate *cache.NodeAllocateState) (score int64) {

	deviceStates := nodeAllocate.NodeStorageAllocatedByUnits.DeviceStates
	if len(deviceStates) <= 0 {
		return int64(utils.MinScore)
	}

	return plugin.allocateStrategy.ScoreDeviceCount(nodeAllocate)
}

func (plugin *LocalPlugin) scoreByNodeAntiAffinity(nodeAllocate *cache.NodeAllocateState) (score int64) {

	var freeDeviceCount = 0
	for _, state := range nodeAllocate.NodeStorageAllocatedByUnits.DeviceStates {
		if !state.IsAllocated {
			freeDeviceCount += 1
		}
	}
	deviceWeight := plugin.nodeAntiAffinityWeight.Get(localtype.VolumeTypeDevice)
	if deviceWeight <= 0 {
		return int64(utils.MinScore)
	}

	if len(nodeAllocate.Units.DevicePVCAllocateUnits) <= 0 && (!plugin.cache.IsLocalNode(nodeAllocate.NodeName) || (freeDeviceCount <= 0)) {
		score = int64(deviceWeight * 1)
		klog.Infof("[NodeAntiAffinity]node %s got %d out of %d", nodeAllocate.NodeName, score, utils.MaxScore)
		return score
	}
	return 0
}
