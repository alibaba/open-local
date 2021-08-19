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

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	utiltrace "k8s.io/utils/trace"
)

// SnapshotPredicate checks if node of the source pv is source node
func SnapshotPredicate(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (bool, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[SnapshotPredicate] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)

	containReadonlySnapshot := true
	err, lvmPVCs, _, _ := algorithm.GetPodPvcs(pod, ctx, true, containReadonlySnapshot)

	if err != nil {
		return false, err
	}
	if len(lvmPVCs) <= 0 {
		log.Infof("[SnapshotPredicate]no open-local volume request on pod %s, skipped", pod.Name)
		return true, nil
	}

	var fits bool
	if len(lvmPVCs) > 0 {
		trace.Step("Computing AllocateLVMVolume")

		fits, err = algo.ProcessSnapshotPVC(lvmPVCs, node, ctx)
		if err != nil {
			log.Error(err)
			return false, err
		} else if fits == false {
			return false, nil
		}
	}

	return true, nil
}
