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

package priorities

import (
	"fmt"
	"github.com/alibaba/open-local/pkg/utils"
	"time"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
	utiltrace "k8s.io/utils/trace"
)

// CountMatch picks the node whose amount of device/mount point best fulfill the amount of pvc requests
func CountMatch(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[CountMatch] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)
	containReadonlySnapshot := false
	err, _, mpPVCs, devicePVCs := algorithm.GetPodPvcs(pod, ctx, true, containReadonlySnapshot)
	if err != nil {
		return utils.MinScore, err
	}
	nc := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nc == nil {
		return 0, fmt.Errorf("failed to get node cache by name %s", node.Name)
	}
	var scoreMP, scoreDevice int
	freeMPCount, err := freeMountPoints(nc)
	if len(mpPVCs) > 0 && freeMPCount > 0 {
		if err != nil {
			return 0, err
		}
		scoreMP = int(float64(len(mpPVCs)) * float64(utils.MaxScore) / float64(freeMPCount))
		log.Infof("[CountMatch]node %s got %d out of %d", node.Name, scoreMP, utils.MaxScore)
	}
	freeDeviceCount, err := freeDevices(nc)
	if len(devicePVCs) > 0 && freeDeviceCount > 0 {
		if err != nil {
			return 0, err
		}
		scoreDevice = int(float64(len(devicePVCs)) * float64(utils.MaxScore) / float64(freeDeviceCount))
		log.Infof("[CountMatch]node %s got %d out of %d", node.Name, scoreDevice, utils.MaxScore)
	}
	return (scoreMP + scoreDevice) / 2.0, nil
}

func freeMountPoints(nc *cache.NodeCache) (int, error) {

	freeMPs := make([]cache.ExclusiveResource, 0)
	for _, mp := range nc.MountPoints {
		if !mp.IsAllocated {
			freeMPs = append(freeMPs, mp)
		}
	}
	return len(freeMPs), nil
}

func freeDevices(nc *cache.NodeCache) (int, error) {
	freeDevices := make([]cache.ExclusiveResource, 0)
	for _, device := range nc.Devices {
		if !device.IsAllocated {
			freeDevices = append(freeDevices, device)
		}
	}
	return len(freeDevices), nil

}
