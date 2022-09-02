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

package predicates

import (
	"math/rand"
	"strings"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

type Predicate struct {
	Name           string
	Ctx            *algorithm.SchedulingContext
	PredicateFuncs []PredicateFunc
}

// PredicateFunc a single predicate implementation for any algorithm
// it should return 2 kinds of error if failed to predicate:
// 1: a PredicateError if pod does not fit a node, PredicateError will be translated as failed reason.
// 2: an Error if any unexpected error met, and will terminate the scheduling process.

type PredicateFunc func(ctx *algorithm.SchedulingContext, pod *corev1.Pod, node *corev1.Node) (bool, error)

var (
	// Newly added predicates should be placed here
	DefaultPredicateFuncs = []PredicateFunc{
		//LuckyPredicate,
		StorageTypePredicate,
		CapacityPredicate,
	}
)

func (p Predicate) Handler(args schedulerapi.ExtenderArgs) (*schedulerapi.ExtenderFilterResult, error) {
	pod := args.Pod

	if utils.NeedSkip(args) {
		log.Infof("predicate: skip pod %s/%s scheduling", pod.Namespace, pod.Name)
		return &schedulerapi.ExtenderFilterResult{
			Nodes:       args.Nodes,
			NodeNames:   args.NodeNames,
			FailedNodes: nil,
			Error:       "",
		}, nil
	}
	nodeNames := []string{}

	if args.NodeNames != nil {
		nodeNames = *args.NodeNames
	} else if args.Nodes != nil {
		for _, n := range args.Nodes.Items {
			nodeNames = append(nodeNames, n.Name)
		}
	}
	log.Infof("predicating pod %s with nodes [%s]", pod.Name, nodeNames)
	canSchedule := make([]string, 0, len(*args.NodeNames))
	canNotSchedule := make(map[string]string)

	errStr := ""

	for _, nodeName := range nodeNames {
		log.Infof("predicating pod %s/%s with node %s", pod.Namespace, pod.Name, nodeName)

		node, err := p.Ctx.CoreV1Informers.Nodes().Lister().Get(nodeName)
		if err != nil {
			log.Errorf("unable to fetch node cache %s from informer: %s", nodeName, err.Error())
			continue
		}
		fits, failReasons, err := Predicates(p.Ctx, p.PredicateFuncs, pod, node)
		log.Infof("pod=%s/%s, node=%s,fits: %t,failReasons: %s, err: %v",
			pod.Namespace, pod.Name, node.Name, fits, failReasons, err)

		if err != nil {
			log.Errorf("node %s is not suitable for pod %s/%s, err: %s ", node.Name, pod.Namespace, pod.Name, err.Error())
			canNotSchedule[nodeName] = err.Error()
			errStr = err.Error()
			break
		} else {
			if fits {
				canSchedule = append(canSchedule, nodeName)
			} else {
				log.Infof("node %s is not suitable for pod %s/%s, reason: %s ", node.Name, pod.Namespace, pod.Name, failReasons)
				canNotSchedule[nodeName] = strings.Join(failReasons, ",")
			}
		}
	}

	if errStr != "" {
		// we need to set the failedNodes to all nodes, so that scheduler will really
		// terminate the scheduling
		for _, n := range nodeNames { // make a copy of error for all nodes
			canNotSchedule[n] = errStr
		}
		canSchedule = make([]string, 0)
	}
	result := schedulerapi.ExtenderFilterResult{
		//Nodes: &corev1.NodeList{
		//	Items: canSchedule,
		//},
		NodeNames:   &canSchedule,
		FailedNodes: canNotSchedule,
		Error:       "", // This is to signal scheduler not to treat it as a failure
	}

	return &result, nil
}

func Predicates(Ctx *algorithm.SchedulingContext, PredicateFuncs []PredicateFunc, pod *corev1.Pod, node *corev1.Node) (fits bool, failedReasons []string, err error) {
	for _, pre := range PredicateFuncs {
		fits, err = pre(Ctx, pod, node)
		isError, failReasons := normalizeError(err)
		log.Infof("fits: %t,failReasons: %s, err: %+v", fits, failReasons, err)

		if isError && err != nil {
			log.Errorf("scheduling terminated for %s/%s: %s", pod.Namespace, pod.Name, err.Error())
			return fits, failedReasons, err
		}
		if fits {
			//return fits, []string{}, nil
			continue
		} else {
			//log.Infof("node %s is not suitable for pod %s/%s, reason: %s ", node.Name, pod.Namespace, pod.Name, failReasons)
			failedReasons = append(failedReasons, failReasons...)
			//return fits, failReasons, nil
		}
	}
	return len(failedReasons) == 0 && fits, failedReasons, nil
}

const (
	// LuckyPred rejects a node if you're not lucky ¯\_(ツ)_/¯
	LuckyPredFailMsg = "Well, you're not lucky"
)

func LuckyPredicate(pod *corev1.Pod, node *corev1.Node) (bool, []string, error) {
	lucky := rand.Intn(2) == 0
	if lucky {
		log.Infof("pod %v/%v is lucky to fit on node %v\n", pod.Name, pod.Namespace, node.Name)
		return true, nil, nil
	}
	log.Infof("pod %v/%v is unlucky to fit on node %v\n", pod.Name, pod.Namespace, node.Name)
	return false, []string{LuckyPredFailMsg}, nil
}

func NewPredicate(ctx *algorithm.SchedulingContext) *Predicate {
	if ctx == nil {
		panic("scheduling context must not be nil")
	}
	return &Predicate{"open-local-predicate", ctx, DefaultPredicateFuncs}
}

func normalizeError(err error) (isError bool, failedReason []string) {
	if err == nil {
		return false, []string{}
	}
	v, ok := err.(errors.PredicateError)
	if ok { // predicate failure is not error
		return false, []string{v.GetReason()}
	}
	return true, []string{}
}
