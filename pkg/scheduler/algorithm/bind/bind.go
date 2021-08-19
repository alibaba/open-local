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
