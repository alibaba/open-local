package priorities

import (
	"fmt"
	"time"

	utiltrace "k8s.io/utils/trace"

	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm/algo"
	"github.com/oecp/open-local-storage-service/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog"
)

const MinScore int = 0
const MaxScore int = 10

func CapacityMatch(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[CapacityMatch] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)
	containReadonlySnapshot := true
	err, lvmPVCs, mpPVCs, devicePVCs := algorithm.GetPodPvcs(pod, ctx, true, containReadonlySnapshot)
	if err != nil {
		return MinScore, err
	}
	// if pod has no open-local-storage-service pvc, it should be scheduled to non LSS nodes
	if len(lvmPVCs) <= 0 && len(mpPVCs) <= 0 && len(devicePVCs) <= 0 {
		log.V(4).Infof("no open-local-storage-service volume request on pod %s, skipped", pod.Name)
		if algorithm.IsLSSNode(node.Name, ctx) == true {
			log.V(4).Infof("node %s is open-local-storage-service node, so pod %s gets minimal score %d", node.Name, pod.Name, MinScore)
			return MinScore, nil
		}
		log.V(4).Infof("node %s is not open-local-storage-service node, so pod %s gets max score %d", node.Name, pod.Name, MaxScore)
		return MaxScore, nil
	}

	// if pod has snapshot pv, return MaxScore
	if utils.ContainsSnapshotPVC(lvmPVCs) == true {
		return MaxScore, nil
	}

	trace.Step("Computing ScoreLVMVolume")
	lvmScore, _, err := algo.ScoreLVMVolume(pod, lvmPVCs, node, ctx)
	if err != nil {
		return MinScore, err
	}
	trace.Step("Computing ScoreMountPointVolume")
	mpScore, _, err := algo.ScoreMountPointVolume(pod, mpPVCs, node, ctx)
	if err != nil {
		return MinScore, err
	}
	trace.Step("Computing ScoreDeviceVolume")

	deviceScore, _, err := algo.ScoreDeviceVolume(pod, devicePVCs, node, ctx)
	if err != nil {
		return MinScore, err
	}
	score := lvmScore + mpScore + deviceScore
	return score, nil
}
