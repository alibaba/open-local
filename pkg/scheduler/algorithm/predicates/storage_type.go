/*
Copyright © 2021 Alibaba Group Holding Ltd.
Copyright © 2022 Intel Corporation

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
	"strconv"
	"time"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
	utiltrace "k8s.io/utils/trace"
)

// StorageTypePredicate
// 1. if SPDK support is required, filter out the nodes without SPDK support
// 2. if SPDK support is unnecessary, filter out the nodes with SPDK support
// 3. if no volume is required, do nothing
func StorageTypePredicate(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (bool, error) {
	var scName *string
	trace := utiltrace.New(fmt.Sprintf("Scheduling[StorageTypePredicate] %s/%s", pod.Namespace, pod.Name))
	defer trace.LogIfLong(50 * time.Millisecond)

	if len(pod.Spec.Volumes) == 0 {
		return true, nil
	}

	nc := ctx.ClusterNodeCache.GetNodeCache(node.Name)
	if nc == nil {
		return false, fmt.Errorf("failed to get node cache by name %s", node.Name)
	}

	ns := pod.Namespace
	for _, v := range pod.Spec.Volumes {
		if v.CSI != nil && utils.ContainsProvisioner(v.CSI.Driver) {
			sv := false
			var err error
			if val, ok := v.CSI.VolumeAttributes["spdk"]; ok {
				sv, err = strconv.ParseBool(val)
				if err != nil {
					sv = false
				}
			}

			if sv != nc.NodeInfo.SupportSPDK {
				return false, nil
			}
		}

		scName = nil
		if v.PersistentVolumeClaim != nil {
			name := v.PersistentVolumeClaim.ClaimName
			pvc, err := ctx.CoreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(ns).Get(name)
			if err != nil {
				log.Errorf("failed to get pvc by name %s/%s: %s", ns, name, err.Error())
				return false, err
			}

			scName = pvc.Spec.StorageClassName
		}

		if v.Ephemeral != nil {
			scName = v.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName
		}

		if scName != nil {
			sc, err := ctx.StorageV1Informers.StorageClasses().Lister().Get(*scName)
			if err != nil {
				log.Errorf("failed to get storage class by name %s: %s", *scName, err.Error())
				return false, err
			}

			if utils.ContainsProvisioner(sc.Provisioner) {
				sv := false
				if val, ok := sc.Parameters["spdk"]; ok {
					var err error
					sv, err = strconv.ParseBool(val)
					if err != nil {
						sv = false
					}
				}

				if sv != nc.NodeInfo.SupportSPDK {
					return false, nil
				}
			}
		}
	}

	return true, nil
}
