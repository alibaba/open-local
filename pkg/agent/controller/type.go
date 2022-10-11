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

package controller

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/alibaba/open-local/pkg/agent/common"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
)

// Agent is the primary "node agent" for open-local that runs on each node
type Agent struct {
	*common.Configuration
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// localclientset is a clientset for our own API group
	localclientset clientset.Interface
	snapclientset  snapshot.Interface
	nlsSynced      cache.InformerSynced

	// eventRecorder is an event eventRecorder for recording Event resources to the
	// Kubernetes API.
	eventRecorder record.EventRecorder

	workqueue workqueue.RateLimitingInterface
}
