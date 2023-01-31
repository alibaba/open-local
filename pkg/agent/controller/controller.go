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
	"fmt"
	"time"

	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/alibaba/open-local/pkg/agent/common"
	"github.com/alibaba/open-local/pkg/agent/discovery"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	localinformerfactory "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
)

const initResourceKey = "initResource"

// NewAgent returns a new open-local agent
func NewAgent(
	config *common.Configuration,
	kubeclientset kubernetes.Interface,
	localclientset clientset.Interface,
	snapclientset snapshot.Interface,
	localInformerFactory localinformerfactory.SharedInformerFactory,
	eventRecorder record.EventRecorder) *Agent {

	nlsInformer := localInformerFactory.Csi().V1alpha1().NodeLocalStorages()
	controller := &Agent{
		Configuration:  config,
		kubeclientset:  kubeclientset,
		localclientset: localclientset,
		snapclientset:  snapclientset,
		nlsSynced:      nlsInformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "InitResource"),
		eventRecorder:  eventRecorder,
	}
	nlsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.handleNLS(nil, obj)
		},
		UpdateFunc: controller.handleNLS,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Agent) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if ok := cache.WaitForCacheSync(stopCh, c.nlsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	// Start the informer factories to begin populating the informer caches
	discoverer := discovery.NewDiscoverer(c.Configuration, c.kubeclientset, c.localclientset, c.snapclientset, c.eventRecorder)
	go wait.Until(discoverer.Discover, time.Duration(discoverer.DiscoverInterval)*time.Second, stopCh)
	go wait.BackoffUntil(func() {
		c.workqueue.Add(initResourceKey)
	},
		wait.NewExponentialBackoffManager(time.Duration(discoverer.DiscoverInterval)*time.Second,
			30*time.Duration(discoverer.DiscoverInterval)*time.Second,
			90*time.Duration(discoverer.DiscoverInterval)*time.Second,
			2.0, 1.0, &clock.RealClock{}),
		true, stopCh)

	go wait.Until(func() {
		c.runWorker(discoverer)
	}, time.Second, stopCh)

	log.Info("Started open-local agent")
	<-stopCh
	log.Info("Shutting down agent")

	return nil
}

func (c *Agent) handleNLS(old, new interface{}) {
	var nlsName string
	var err error
	if nlsName, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		utilruntime.HandleError(err)
		return
	}
	if nlsName == c.Nodename {
		if old == nil {
			c.workqueue.Add(initResourceKey)
			return
		}
		oldNLS, ok := old.(*localv1alpha1.NodeLocalStorage)
		if !ok {
			return
		}
		newNLS, ok := new.(*localv1alpha1.NodeLocalStorage)
		if !ok {
			return
		}
		if oldNLS.Generation != newNLS.Generation {
			c.workqueue.Add(initResourceKey)
			log.Info("nls spec changed, trigger init resource")
		}
	}
}

func (c *Agent) runWorker(discovery *discovery.Discoverer) {
	for c.processNextWorkItem(discovery) {
	}
}

func (c *Agent) processNextWorkItem(discovery *discovery.Discoverer) bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	func(obj interface{}) {
		defer c.workqueue.Done(obj)

		switch item := obj.(type) {
		case string:
			if item == initResourceKey {
				discovery.InitResource()
			}
		default:
			c.workqueue.Forget(obj)
			return
		}
	}(obj)

	return true
}
