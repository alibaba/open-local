package predicates

import (
	"fmt"
	"time"

	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm/algo"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog"
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
		log.V(4).Infof("[SnapshotPredicate]no open-local-storage-service volume request on pod %s, skipped", pod.Name)
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
