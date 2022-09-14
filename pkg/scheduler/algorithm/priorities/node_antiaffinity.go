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

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
	utiltrace "k8s.io/utils/trace"
)

// NodeAntiAffinity picks the node whose amount of device/mount point best fulfill the amount of pvc requests
func NodeAntiAffinity(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error) {
	trace := utiltrace.New(fmt.Sprintf("Scheduling[NodeAntiAffinity] %s/%s", pod.Namespace, pod.Name))
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

	// currently ,we only take FREE mount point and devices into account
	isLocal := algorithm.IsLocalNode(node.Name, ctx)
	freeMPCount, err := freeMountPoints(nc)
	if err != nil {
		return 0, err
	}
	freeDeviceCount, err := freeDevices(nc)
	if err != nil {
		return 0, err
	}
	volumeTypeAntiFound := make(map[pkg.VolumeType]bool)
	for volumeType, weight := range ctx.NodeAntiAffinityWeight.Items(false) {
		if weight <= 0 {
			continue
		}
		switch volumeType {
		case pkg.VolumeTypeMountPoint:
			log.Infof("node=%s,isLocal: %t, nc.MountPoints: %d, freeMPCount: %d", node.Name, isLocal, len(nc.MountPoints), freeMPCount)
			if len(mpPVCs) <= 0 && (!isLocal || (freeMPCount <= 0)) {
				// non-open-local Pod and Node is not enough open-local volume of this type
				scoreMP = weight * 1
				log.Infof("[NodeAntiAffinity]node %s got %d out of %d", node.Name, scoreMP, utils.MaxScore)
				volumeTypeAntiFound[volumeType] = true
			}
		case pkg.VolumeTypeDevice:
			log.Infof("node=%s,isLocal: %t, nc.Devices: %d, freeDeviceCount: %d", node.Name, isLocal, len(nc.Devices), freeDeviceCount)
			if len(devicePVCs) <= 0 && (!isLocal || (freeDeviceCount <= 0)) {
				scoreDevice = weight * 1

				log.Infof("[NodeAntiAffinity]node %s got %d out of %d", node.Name, scoreDevice, utils.MaxScore)
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
