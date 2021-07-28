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
