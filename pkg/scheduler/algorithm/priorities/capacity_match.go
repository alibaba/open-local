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
	"time"

	utiltrace "k8s.io/utils/trace"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
)

func CapacityMatch(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[CapacityMatch] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)
	containReadonlySnapshot := true
	err, lvmPVCs, mpPVCs, devicePVCs := algorithm.GetPodPvcs(pod, ctx, true, containReadonlySnapshot)
	if err != nil {
		return utils.MinScore, err
	}
	containInlineVolume, _ := utils.ContainInlineVolumes(pod)
	// if pod has no open-local pvc, it should be scheduled to non Open-Local nodes
	if len(lvmPVCs) <= 0 && len(mpPVCs) <= 0 && len(devicePVCs) <= 0 && !containInlineVolume {
		log.Infof("no open-local volume request on pod %s, skipped", pod.Name)
		if algorithm.IsLocalNode(node.Name, ctx) {
			log.Infof("node %s is open-local node, so pod %s gets minimal score %d", node.Name, pod.Name, utils.MinScore)
			return utils.MinScore, nil
		}
		log.Infof("node %s is not open-local node, so pod %s gets max score %d", node.Name, pod.Name, utils.MaxScore)
		return utils.MaxScore, nil
	}

	// if pod has snapshot pv, return MaxScore
	if utils.ContainsSnapshotPVC(lvmPVCs) {
		return utils.MaxScore, nil
	}

	trace.Step("Computing ScoreLVMVolume")
	lvmScore, _, err := algo.ScoreLVMVolume(pod, lvmPVCs, node, ctx)
	if err != nil {
		return utils.MinScore, err
	}
	trace.Step("Computing ScoreMountPointVolume")
	mpScore, _, err := algo.ScoreMountPointVolume(pod, mpPVCs, node, ctx)
	if err != nil {
		return utils.MinScore, err
	}
	trace.Step("Computing ScoreDeviceVolume")
	deviceScore, _, err := algo.ScoreDeviceVolume(pod, devicePVCs, node, ctx)
	if err != nil {
		return utils.MinScore, err
	}
	trace.Step("Computing ScoreDeviceVolume")
	inlineScore, _, err := algo.ScoreInlineLVMVolume(pod, node, ctx)
	if err != nil {
		return utils.MinScore, err
	}

	score := lvmScore + mpScore + deviceScore + inlineScore
	return score, nil
}
