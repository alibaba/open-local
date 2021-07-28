package bind

import (
	"k8s.io/apimachinery/pkg/types"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

type Bind struct {
	Func func(podName string, podNamespace string, podUID types.UID, node string) error
}

func (b Bind) Handler(args schedulerapi.ExtenderBindingArgs) *schedulerapi.ExtenderBindingResult {
	_ = b.Func(args.PodName, args.PodNamespace, args.PodUID, args.Node)
	return &schedulerapi.ExtenderBindingResult{
		Error: "Bind not implemented",
	}
}
