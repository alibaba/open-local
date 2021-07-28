package priorities

import (
	"fmt"
	"time"

	"github.com/oecp/open-local-storage-service/pkg"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog"
	utiltrace "k8s.io/utils/trace"
)

// NodeAntiAffinity picks the node whose amount of device/mount point best fulfill the amount of pvc requests
func NodeAntiAffinity(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[NodeAntiAffinity] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)
	containReadonlySnapshot := false
	err, _, mpPVCs, devicePVCs := algorithm.GetPodPvcs(pod, ctx, true, containReadonlySnapshot)
	if err != nil {
		return MinScore, err
	}
	nc := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nc == nil {
		return 0, fmt.Errorf("failed to get node cache by name %s", node.Name)
	}
	var scoreMP, scoreDevice int

	// currently ,we only take FREE mount point and devices into account
	isLSS := algorithm.IsLSSNode(node.Name, ctx)
	freeMPCount, err := freeMountPoints(nc)
	freeDeviceCount, err := freeDevices(nc)
	volumeTypeAntiFound := make(map[pkg.VolumeType]bool)
	for volumeType, weight := range ctx.NodeAntiAffinityWeight.Items(false) {
		if weight <= 0 {
			continue
		}
		switch volumeType {
		case pkg.VolumeTypeMountPoint:
			log.V(6).Infof("node=%s,isLSS: %t, nc.MountPoints: %d, freeMPCount: %d", node.Name, isLSS, len(nc.MountPoints), freeMPCount)
			if len(mpPVCs) <= 0 && (!isLSS || (freeMPCount <= 0)) {
				// non-open-local-storage-service Pod and Node is not enough open-local-storage-service volume of this type
				scoreMP = weight * 1
				log.V(5).Infof("[NodeAntiAffinity]node %s got %d out of %d", node.Name, scoreMP, MaxScore)
				volumeTypeAntiFound[volumeType] = true
			}
		case pkg.VolumeTypeDevice:
			log.V(6).Infof("node=%s,isLSS: %t, nc.Devices: %d, freeDeviceCount: %d", node.Name, isLSS, len(nc.Devices), freeDeviceCount)
			if len(devicePVCs) <= 0 && (!isLSS || (freeDeviceCount <= 0)) {
				scoreDevice = weight * 1

				log.V(5).Infof("[NodeAntiAffinity]node %s got %d out of %d", node.Name, scoreDevice, MaxScore)
				volumeTypeAntiFound[volumeType] = true
			}
		case pkg.VolumeTypeLVM:
		}
	}
	if len(volumeTypeAntiFound) <= 0 {
		return 0, nil
	}
	return int(float64(scoreMP+scoreDevice) / float64(len(volumeTypeAntiFound))), nil
}
