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
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
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
	if len(p.Ctx.NodeAntiAffinityWeight.Items(false)) == 0 && utils.NeedSkip(args) {
		log.Infof("priorities: skip pod %s/%s scheduling", pod.Namespace, pod.Name)
		return &hostPriorityList, nil
	}

	for _, pri := range p.PrioritizeFuncs {
		for i, nodeName := range nodeNames {
			log.Infof("prioritizing pod %s/%s with node %s", pod.Namespace, pod.Name, nodeName)
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
			log.Infof("pod %s/%s on node %q , score=%d", pod.Name, pod.Namespace, node.Name, score)
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
	return &Prioritize{"open-local-prioritize", ctx, DefaultPrioritizeFuncs}
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
