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
	"context"
	"fmt"
	"reflect"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	snapshotinformerfactory "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v4/listers/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformerfactory "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/generated/clientset/versioned/scheme"
	localscheme "github.com/alibaba/open-local/pkg/generated/clientset/versioned/scheme"
	localinformerfactory "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	locallisters "github.com/alibaba/open-local/pkg/generated/listers/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
)

const (
	SuccessSynced                 = "Synced"
	MessageResourceSynced         = "NLSC synced successfully"
	AnnVolumeSnapshotBeingDeleted = "snapshot.storage.kubernetes.io/volumesnapshot-being-deleted"
)

type Controller struct {
	kubeclientset     kubernetes.Interface
	localclientset    clientset.Interface
	snapshotclientset snapshot.Interface

	nodeLister            corelisters.NodeLister
	nodeSynced            cache.InformerSynced
	podLister             corelisters.PodLister
	podSynced             cache.InformerSynced
	snapshotLister        snapshotlisters.VolumeSnapshotLister
	snapshotSynced        cache.InformerSynced
	snapshotContentLister snapshotlisters.VolumeSnapshotContentLister
	snapshotContentSynced cache.InformerSynced
	snapshotClassLister   snapshotlisters.VolumeSnapshotClassLister
	snapshotClassSynced   cache.InformerSynced
	nlsLister             locallisters.NodeLocalStorageLister
	nlsSynced             cache.InformerSynced
	nlscLister            locallisters.NodeLocalStorageInitConfigLister
	nlscSynced            cache.InformerSynced
	pvcLister             corelisters.PersistentVolumeClaimLister
	pvcSynced             cache.InformerSynced
	pvLister              corelisters.PersistentVolumeLister
	pvSynced              cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder

	nlscName string
}

type SyncNLSItem struct {
	nlscName string
	nlsName  string
}

type SyncPVByPodItem struct {
	podNameSpace string
	podName      string
}

// NewController returns a new sample c
func NewController(
	kubeclientset kubernetes.Interface,
	localclientset clientset.Interface,
	snapclientset snapshot.Interface,
	kubeInformerFactory kubeinformerfactory.SharedInformerFactory,
	localInformerFactory localinformerfactory.SharedInformerFactory,
	snapshotInformerFactory snapshotinformerfactory.SharedInformerFactory,
	nlscName string) *Controller {

	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()

	nlsInformer := localInformerFactory.Csi().V1alpha1().NodeLocalStorages()
	nlscInformer := localInformerFactory.Csi().V1alpha1().NodeLocalStorageInitConfigs()
	snapshotInformer := snapshotInformerFactory.Snapshot().V1().VolumeSnapshots()
	snapshotContentInformer := snapshotInformerFactory.Snapshot().V1().VolumeSnapshotContents()
	snapshotClassInformer := snapshotInformerFactory.Snapshot().V1().VolumeSnapshotClasses()

	// Create event broadcaster
	utilruntime.Must(localscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "open-local-controller"})

	c := &Controller{
		kubeclientset:         kubeclientset,
		localclientset:        localclientset,
		snapshotclientset:     snapclientset,
		nodeLister:            nodeInformer.Lister(),
		nodeSynced:            nodeInformer.Informer().HasSynced,
		podLister:             podInformer.Lister(),
		podSynced:             podInformer.Informer().HasSynced,
		pvcLister:             pvcInformer.Lister(),
		pvcSynced:             pvcInformer.Informer().HasSynced,
		pvLister:              pvInformer.Lister(),
		pvSynced:              pvInformer.Informer().HasSynced,
		nlsLister:             nlsInformer.Lister(),
		nlsSynced:             nlsInformer.Informer().HasSynced,
		nlscLister:            nlscInformer.Lister(),
		nlscSynced:            nlscInformer.Informer().HasSynced,
		snapshotLister:        snapshotInformer.Lister(),
		snapshotSynced:        snapshotInformer.Informer().HasSynced,
		snapshotContentLister: snapshotContentInformer.Lister(),
		snapshotContentSynced: snapshotContentInformer.Informer().HasSynced,
		snapshotClassLister:   snapshotClassInformer.Lister(),
		snapshotClassSynced:   snapshotClassInformer.Informer().HasSynced,
		workqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "NodeLocalStorageInitConfig"),
		recorder:              eventRecorder,
		nlscName:              nlscName,
	}

	klog.Info("Setting up event handlers")
	nlscInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.handleNLSC,
		UpdateFunc: func(old, new interface{}) {
			c.handleNLSC(new)
		},
	})
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.createNLSByNode,
		DeleteFunc: c.deleteNLSByNode,
	})
	nlsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.handleNLS(nil, obj)
		},
		UpdateFunc: c.handleNLS,
	})

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.handlePod(nil, obj)
		},
		UpdateFunc: c.handlePod,
	})

	return c
}

func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.nlscSynced, c.nlsSynced, c.nodeSynced, c.snapshotSynced, c.snapshotContentSynced, c.snapshotClassSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting controller workers")

	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	if DefaultFeatureGate.Enabled(OrphanedSnapshotContent) {
		go wait.Until(c.cleanOrphanSnapshotContents, time.Minute, stopCh)
	}

	klog.Info("Started controller")
	<-stopCh
	klog.Info("Shutting down controller")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		switch item := obj.(type) {
		case SyncNLSItem:
			if err := c.syncHandler(item); err != nil {
				c.workqueue.AddRateLimited(item)
				return fmt.Errorf("error SyncNLSItem '%#v': %s, requeuing", item, err.Error())
			}
		case SyncPVByPodItem:
			if err := c.addVGInfoToPVsForPod(item.podNameSpace, item.podName); err != nil {
				c.workqueue.AddAfter(item, time.Millisecond*500)
				return fmt.Errorf("error SyncPVByPodItem '%#v': %s, requeuing", item, err.Error())
			}
		default:
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected SyncNLSItem in workqueue but got %#v", obj))
			return nil
		}

		klog.V(6).Infof("Successfully synced '%#v'", obj)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncHandler(item SyncNLSItem) error {
	// step 1: get nls name slice
	var nlsNames []string
	if item.nlsName != "" {
		nlsNames = append(nlsNames, item.nlsName)
	} else {
		nodelist, err := c.nodeLister.List(labels.Everything())
		if err != nil {
			return err
		}
		for _, node := range nodelist {
			nlsNames = append(nlsNames, node.Name)
		}
	}

	// step 2: handle
	for _, name := range nlsNames {
		nls, err := c.nlsLister.Get(name)
		// create nls if not found
		if errors.IsNotFound(err) {
			klog.Warningf("nls %s not found", name)
			nls := new(localv1alpha1.NodeLocalStorage)
			nls.SetName(name)
			nls.Spec.NodeName = name
			nls, err := c.updateNLSSpec(nls)
			if err != nil {
				return err
			}
			_, createErr := c.localclientset.CsiV1alpha1().NodeLocalStorages().Create(context.Background(), nls, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
			continue
		}
		if err != nil {
			return err
		}

		// update nls if needed
		if DefaultFeatureGate.Enabled(UpdateNLS) {
			if err := c.updateNLSIfNeeded(nls); err != nil {
				return err
			}
		}
	}

	// step 3: update nlsc event
	nlsc, err := c.nlscLister.Get(item.nlscName)
	if err != nil {
		return err
	}
	if item.nlsName == "" {
		c.recorder.Event(nlsc, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	}
	return nil
}

func (c *Controller) updateNLSSpec(nls *localv1alpha1.NodeLocalStorage) (*localv1alpha1.NodeLocalStorage, error) {
	if nls == nil {
		return nil, fmt.Errorf("nls is nil!")
	}
	nlsc, err := c.nlscLister.Get(c.nlscName)
	if err != nil {
		return nil, fmt.Errorf("get nlsc %s failed: %s", c.nlscName, err.Error())
	}
	nlsCopy := nls.DeepCopy()
	nlsCopy.Spec.ListConfig = nlsc.Spec.GlobalConfig.ListConfig
	nlsCopy.Spec.SpdkConfig = nlsc.Spec.GlobalConfig.SpdkConfig
	nlsCopy.Spec.ResourceToBeInited = nlsc.Spec.GlobalConfig.ResourceToBeInited
	node, err := c.nodeLister.Get(nlsCopy.Name)
	if err != nil {
		return nil, fmt.Errorf("get node %s failed", nlsCopy.Name)
	}
	nodeLabels := node.Labels
	for _, nodeconfig := range nlsc.Spec.NodesConfig {
		selector, err := metav1.LabelSelectorAsSelector(nodeconfig.Selector)
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(nodeLabels)) {
			continue
		}
		nlsCopy.Spec.ListConfig = nodeconfig.ListConfig
		nlsCopy.Spec.SpdkConfig = nodeconfig.SpdkConfig
		nlsCopy.Spec.ResourceToBeInited = nodeconfig.ResourceToBeInited
	}

	return nlsCopy, nil
}

func (c *Controller) updateNLSIfNeeded(nls *localv1alpha1.NodeLocalStorage) error {
	nlsUpdated, err := c.updateNLSSpec(nls)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(nls, nlsUpdated) {
		klog.Infof("nls %s need to be updated", nls.Name)
		if _, err := c.localclientset.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), nlsUpdated, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) handleNLSC(obj interface{}) {
	var nlscName string
	var err error
	if nlscName, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.enqueueNLSC(nlscName, "")
}

func (c *Controller) handleNLS(old, new interface{}) {
	var nlsName string
	var err error
	if nlsName, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.enqueueNLSC(c.nlscName, nlsName)
}

func (c *Controller) handlePod(old, new interface{}) {
	c.enqueueSyncPVItemByPod(old, new)
}

func (c *Controller) enqueueSyncPVItemByPod(old, new interface{}) (skip bool) {
	// check
	pod, ok := new.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodUpdate]cannot convert newObj to *Pod: %#v", new)
		return true
	}

	if old != nil {
		oldPod, ok := old.(*corev1.Pod)
		if !ok {
			klog.Errorf("[OnPodUpdate]cannot convert oldObj to *Pod: %v", old)
			return true
		}

		oldAllocatedJson := localtype.GetAllocateInfoJSONFromPod(oldPod)
		newAllocatedJson := localtype.GetAllocateInfoJSONFromPod(pod)

		if newAllocatedJson == "" || newAllocatedJson == oldAllocatedJson {
			return true
		}
	}

	c.workqueue.Add(SyncPVByPodItem{
		podNameSpace: pod.Namespace,
		podName:      pod.Name,
	})
	return false
}

func (c *Controller) createNLSByNode(obj interface{}) {
	var nodeName string
	var err error
	if nodeName, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.enqueueNLSC(c.nlscName, nodeName)
}

func (c *Controller) deleteNLSByNode(obj interface{}) {
	var nodeName string
	var err error
	if nodeName, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	if err = c.localclientset.CsiV1alpha1().NodeLocalStorages().Delete(context.Background(), nodeName, *metav1.NewDeleteOptions(1)); err != nil {
		klog.Errorf("Delete nls %s failed: %s", nodeName, err.Error())
	}
}

// if nlsName is "", then controller will iterate over all nls. It will be time consuming
func (c *Controller) enqueueNLSC(nlscName string, nlsName string) {
	if nlscName == c.nlscName {
		c.workqueue.Add(SyncNLSItem{
			nlscName: c.nlscName,
			nlsName:  nlsName,
		})
	}
}

func (c *Controller) cleanOrphanSnapshotContents() {
	allContents, err := c.snapshotContentLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("fail to list snapshot contents: %s", err.Error())
		return
	}
	for _, content := range allContents {
		needAnnotation, err := c.isOrphanSnapshotContent(content)
		if err != nil {
			klog.Errorf("fail to check snapshot content %s is orphan: %s", content.Name, err.Error())
			continue
		}
		if needAnnotation {
			if err := c.setAnnVolumeSnapshotBeingDeleted(content); err != nil {
				klog.Errorf("fail to set annotation on orphan snapshot content %s: %s", content.Name, err.Error())
				continue
			}
			klog.Infof("annotate orphan snapshot content %s successfully", content.Name)
		}
	}
}

func (c *Controller) isOrphanSnapshotContent(content *snapshotapi.VolumeSnapshotContent) (bool, error) {
	if content.DeletionTimestamp == nil {
		return false, nil
	}
	if content.Spec.VolumeSnapshotRef.UID == "" {
		return false, nil
	}

	class, err := c.snapshotClassLister.Get(*content.Spec.VolumeSnapshotClassName)
	if err != nil {
		return false, err
	}
	if !utils.ContainsProvisioner(class.Driver) {
		return false, nil
	}

	_, err = c.snapshotLister.VolumeSnapshots(content.Spec.VolumeSnapshotRef.Namespace).Get(content.Spec.VolumeSnapshotRef.Name)
	if err == nil {
		return false, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	return true, nil
}

func (c *Controller) setAnnVolumeSnapshotBeingDeleted(content *snapshotapi.VolumeSnapshotContent) error {
	if !metav1.HasAnnotation(content.ObjectMeta, AnnVolumeSnapshotBeingDeleted) {
		klog.Infof("setAnnVolumeSnapshotBeingDeleted: set annotation %s on content %s", AnnVolumeSnapshotBeingDeleted, content.Name)
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, AnnVolumeSnapshotBeingDeleted, "yes")
	}
	_, err := c.snapshotclientset.SnapshotV1().VolumeSnapshotContents().Update(context.TODO(), content, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}
