package priorities

import (
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

var (
	// Newly added predicates should be placed here
	DefaultPrioritizeFuncs = []PrioritizeFunc{
		//LuckyPredicate,
		CapacityMatch,
		CountMatch,
		NodeAntiAffinity,
	}
)

type PrioritizeFunc func(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (int, error)

type Prioritize struct {
	Name            string
	Ctx             *algorithm.SchedulingContext
	PrioritizeFuncs []PrioritizeFunc
}

func (p Prioritize) Handler(args schedulerapi.ExtenderArgs) (*schedulerapi.HostPriorityList, error) {
	pod := args.Pod
	nodeNames := []string{}

	if args.NodeNames != nil {
		nodeNames = *args.NodeNames
	} else if args.Nodes != nil {
		for _, n := range args.Nodes.Items {
			nodeNames = append(nodeNames, n.Name)
		}
	}
	hostPriorityList := InitHostPriorityList(nodeNames)

	for _, pri := range p.PrioritizeFuncs {
		for i, nodeName := range nodeNames {
			log.V(5).Infof("prioritizing pod %s/%s with node %s", pod.Namespace, pod.Name, nodeName)
			node, err := p.Ctx.CoreV1Informers.Nodes().Lister().Get(nodeName)
			if err != nil {
				log.Errorf("unable to fetch node cache %s from informer: %s", nodeName, err.Error())
				continue
			}
			score, err := pri(p.Ctx, pod, node)
			if err != nil {
				log.Errorf("error when prioritize pod %s, for node %s: %s", pod.Name, node.Name,
					err.Error())
				continue
			}
			log.V(3).Infof("pod %s/%s on node %q , score=%d", pod.Name, pod.Namespace, node.Name, score)
			hostPriorityList[i] = schedulerapi.HostPriority{
				Host:  node.Name,
				Score: int64(score) + hostPriorityList[i].Score,
			}
		}
	}
	return &hostPriorityList, nil
}

func NewPrioritize(ctx *algorithm.SchedulingContext) *Prioritize {
	if ctx == nil {
		panic("scheduling context must not be nil")
	}
	return &Prioritize{"open-local-storage-service-prioritize", ctx, DefaultPrioritizeFuncs}
}

func InitHostPriorityList(nodeNames []string) schedulerapi.HostPriorityList {
	hostPriorityList := make(schedulerapi.HostPriorityList, len(nodeNames))
	for idx, name := range nodeNames {
		hostPriorityList[idx] = schedulerapi.HostPriority{
			Host:  name,
			Score: 0,
		}
	}
	return hostPriorityList
}
