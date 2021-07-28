/*
Copyright 2021 OECP Authors.

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

package preemptions

import (
	v1 "k8s.io/api/core/v1"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

type Preemption struct {
	Func func(
		pod v1.Pod,
		nodeNameToVictims map[string]*schedulerapi.Victims,
		nodeNameToMetaVictims map[string]*schedulerapi.MetaVictims,
	) map[string]*schedulerapi.MetaVictims
}

func (b Preemption) Handler(
	args schedulerapi.ExtenderPreemptionArgs,
) *schedulerapi.ExtenderPreemptionResult {
	nodeNameToMetaVictims := b.Func(*args.Pod, args.NodeNameToVictims, args.NodeNameToMetaVictims)
	return &schedulerapi.ExtenderPreemptionResult{
		NodeNameToMetaVictims: nodeNameToMetaVictims,
	}
}
