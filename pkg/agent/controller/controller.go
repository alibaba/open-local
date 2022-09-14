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
	"k8s.io/apimachinery/pkg/util/clock"
	"os"
	"strconv"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/agent/common"
	"github.com/alibaba/open-local/pkg/agent/discovery"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
)

// NewAgent returns a new open-local agent
func NewAgent(
	config *common.Configuration,
	kubeclientset kubernetes.Interface,
	localclientset clientset.Interface,
	snapclientset snapshot.Interface,
	eventRecorder record.EventRecorder) *Agent {

	controller := &Agent{
		Configuration:  config,
		kubeclientset:  kubeclientset,
		localclientset: localclientset,
		snapclientset:  snapclientset,
		eventRecorder:  eventRecorder,
	}

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Agent) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	// Start the informer factories to begin populating the informer caches
	discoverer := discovery.NewDiscoverer(c.Configuration, c.kubeclientset, c.localclientset, c.snapclientset, c.eventRecorder)
	go wait.Until(discoverer.Discover, time.Duration(discoverer.DiscoverInterval)*time.Second, stopCh)
	go wait.BackoffUntil(discoverer.InitResource,
		wait.NewExponentialBackoffManager(time.Duration(discoverer.DiscoverInterval)*time.Second,
			30*time.Duration(discoverer.DiscoverInterval)*time.Second,
			90*time.Duration(discoverer.DiscoverInterval)*time.Second,
			2.0, 1.0, &clock.RealClock{}),
		true, stopCh)

	// get auto expand snapshot interval
	var err error
	expandSnapInterval := discoverer.DiscoverInterval
	tmp := os.Getenv(localtype.EnvExpandSnapInterval)
	if tmp != "" {
		expandSnapInterval, err = strconv.Atoi(tmp)
		if err != nil {
			return err
		}
	}
	go wait.Until(discoverer.ExpandSnapshotLVIfNeeded, time.Duration(expandSnapInterval)*time.Second, stopCh)

	log.Info("Started open-local agent")
	<-stopCh
	log.Info("Shutting down agent")

	return nil
}
