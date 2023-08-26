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

package predicates

import (
	"fmt"
	"time"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	log "k8s.io/klog/v2"
	utiltrace "k8s.io/utils/trace"
)

// CapacityPredicate checks if local storage on a node matches the persistent volume claims, follow rules are applied:
//  1. pvc contains vg or mount point or device claim
//  2. node free size must larger or equal to pvcs
//  3. for pvc of type mount point/device:
//     a. must contains more mount points than pvc count
func CapacityPredicate(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (bool, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[CapacityPredicate] %s", utils.GetName(pod.ObjectMeta)))
	defer trace.LogIfLong(50 * time.Millisecond)

	// 包含了快照卷
	err, lvmPVCs, mpPVCs, devicePVCs := algorithm.GetPodPvcs(pod, ctx, true)
	if err != nil {
		return false, err
	}

	containInlineVolume, _ := utils.ContainInlineVolumes(pod)
	if containInlineVolume {
		fits, _, err := algo.HandleInlineLVMVolume(ctx, node, pod)
		if err != nil {
			log.Error(err)
			return false, err
		} else if !fits {
			return false, nil
		}
	}

	var fits bool
	if len(lvmPVCs) > 0 {
		trace.Step("Computing AllocateLVMVolume")

		// 如果是只读快照卷，需要去掉
		// 预测时不需要考虑这部分容量申请
		fits, _, err = algo.AllocateLVMVolume(pod, lvmPVCs, node, ctx)
		if err != nil {
			log.Error(err)
			return false, err
		} else if !fits {
			return false, nil
		}
	}

	if len(mpPVCs) > 0 {
		trace.Step("Computing AllocateMountPointVolume")

		fits, _, err = algo.AllocateMountPointVolume(pod, mpPVCs, node, ctx)
		if err != nil {
			log.Error(err)
			return false, err
		} else if !fits {
			return false, nil
		}
	}

	if len(devicePVCs) > 0 {
		trace.Step("Computing AllocateDeviceVolume")

		fits, _, err = algo.AllocateDeviceVolume(pod, devicePVCs, node, ctx)
		if err != nil {
			log.Error(err)
			return false, err
		} else if !fits {
			return false, nil
		}
	}

	// 处理下只读snapshot
	if fits, err = algo.ProcessSnapshotPVC(lvmPVCs, node.Name, ctx.CoreV1Informers, ctx.SnapshotInformers); err != nil {
		return false, err
	}
	if !fits {
		klog.Infof("pod %s not fit node %s readonly snapshot", utils.GetName(pod.ObjectMeta), node.Name)
		return false, errors.NewSnapshotError(pkg.VolumeTypeLVM)
	} else {
		klog.Infof("pod %s fit node %s readonly snapshot!", utils.GetName(pod.ObjectMeta), node.Name)
	}

	if len(lvmPVCs) <= 0 && len(mpPVCs) <= 0 && len(devicePVCs) <= 0 && !containInlineVolume {
		log.Infof("no open-local volume request on pod %s, skipped", pod.Name)
		return true, nil
	}

	return true, nil
}
