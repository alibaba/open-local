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

package server

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgocache "k8s.io/client-go/tools/cache"
	log "k8s.io/klog/v2"
	utiltrace "k8s.io/utils/trace"
)

const (
	MaxConcurrentWorkingRoutines = 5
)

func (e *ExtenderServer) onNodeLocalStorageAdd(obj interface{}) {
	local, ok := obj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("cannot convert to *NodeLocalStorage: %v", obj)
		return
	}

	trace := utiltrace.New(fmt.Sprintf("[onNodeLocalStorageAdd]trace nls %s", local.Name))
	defer trace.LogIfLong(50 * time.Millisecond)

	trace.Step("Computing add node cache")
	nodeName := local.Name
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	if v := e.Ctx.ClusterNodeCache.GetNodeCache(nodeName); v == nil {
		// Create a new node cache
		log.V(6).Infof("[onNodeLocalStorageAdd]Created new node cache for %s", nodeName)
		e.Ctx.ClusterNodeCache.AddNodeCache(local)
		log.V(6).Infof("[onNodeLocalStorageAdd]Add new node cache %s completed.", nodeName)
	} else {
		e.Ctx.ClusterNodeCache.UpdateNodeCache(local)
	}
}

func (e *ExtenderServer) onNodeLocalStorageUpdate(oldObj, newObj interface{}) {
	local, ok := newObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("cannot convert newObj to *NodeLocalStorage: %v", newObj)
		return
	}
	old, ok := oldObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("cannot convert oldObj to *NodeLocalStorage: %v", oldObj)
		return
	}
	log.V(6).Infof("get update on node local cache %s", local.Name)

	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	nodeName := local.Name
	if v := e.Ctx.ClusterNodeCache.GetNodeCache(nodeName); v == nil {
		// Create a new node cache
		log.V(6).Infof("[onNodeLocalStorageUpdate]Created new node cache for %s", nodeName)
		e.Ctx.ClusterNodeCache.AddNodeCache(local)
	} else {
		// if status changed, we need to update the storage
		if utils.HashWithoutState(old) != utils.HashWithoutState(local) {
			log.V(6).Infof("spec of node local storage %s get changed, try to update", local.Name)
			e.Ctx.ClusterNodeCache.UpdateNodeCache(local)
		}
	}
}

func (e *ExtenderServer) onPVAdd(obj interface{}) {
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		log.Errorf("cannot convert to *v1.PersistentVolume: %v", obj)
		return
	}
	log.Infof("[onPVAdd]pv %s is handling", pv.Name)

	trace := utiltrace.New(fmt.Sprintf("[onPVAdd]trace pv %s", pv.Name))
	defer trace.LogIfLong(50 * time.Millisecond)

	// check if PV is a Local PV
	// if it is, check what type it is
	containReadonlySnapshot := false
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, containReadonlySnapshot)
	if !isOpenLocalPV {
		return
	}
	if pv.Status.Phase == corev1.VolumePending {
		log.Infof("pv %s is in %s status, skipped", pv.Name, pv.Status.Phase)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	// get node name
	node := utils.NodeNameFromPV(pv)
	if node == "" {
		log.Infof("pv %s is not a valid open-local local pv, skipped", pv.Name)
		return
	}
	// get node cache
	trace.Step("Computing get or set node cache")
	nc := e.Ctx.ClusterNodeCache.GetNodeCache(node)
	if nc == nil {
		log.V(6).Infof("no node cache %q found when adding pv %q", node, pv.Name)
		nc = cache.NewNodeCache(node)
		e.Ctx.ClusterNodeCache.SetNodeCache(nc)
		log.V(6).Infof("created new node cache %q when adding pv %q", node, pv.Name)
	}
	// handle according to types
	switch pkg.VolumeType(pvType) {
	case pkg.VolumeTypeLVM:
		trace.Step("Computing AddLVM")
		err := nc.AddLVM(pv)
		if err != nil {
			log.Errorf("failed to add local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeMountPoint:
		trace.Step("Computing AddLocalMountPoint")
		err := nc.AddLocalMountPoint(pv)
		if err != nil {
			log.Errorf("failed to add local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeDevice:
		trace.Step("Computing AddLocalDevice")
		err := nc.AddLocalDevice(pv)
		if err != nil {
			log.Errorf("failed to add local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	default:
		log.V(6).Infof("not a open-local pv %s, type %s, not add to cache", pv.Name, pvType)
		return
	}
	pvcKey, err := algorithm.ExtractPVCKey(pv)
	if err != nil {
		log.Errorf("failed to extract pvc name from pv %s: %s", pv.Name, err.Error())
		return
	}
	au, err := algorithm.ConvertAUFromPV(pv, e.Ctx.StorageV1Informers, e.Ctx.CoreV1Informers)
	if err == nil {
		e.Ctx.ClusterNodeCache.BindingInfo[pvcKey] = au
		log.V(6).Infof("%s was added into binding info", pvcKey)
	}
	log.V(6).Infof("pv %s (type: %s) is added to node cache %s", pv.Name, pvType, nc.NodeName)
	trace.Step("Computing set node cache")
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}

func (e *ExtenderServer) onPVDelete(obj interface{}) {
	var pv *corev1.PersistentVolume
	switch t := obj.(type) {
	case *corev1.PersistentVolume:
		pv = t
	case clientgocache.DeletedFinalStateUnknown:
		var ok bool
		pv, ok = t.Obj.(*corev1.PersistentVolume)
		if !ok {
			log.Errorf("cannot convert to *v1.PersistentVolume: %v", t.Obj)
			return
		}
	default:
		log.Errorf("cannot convert to *v1.PersistentVolume: %v", t)
		return
	}
	log.Infof("[onPVDelete]pv %s is handling", pv.Name)

	containReadonlySnapshot := false
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, containReadonlySnapshot)
	if !isOpenLocalPV {
		return
	}
	node := utils.NodeNameFromPV(pv)
	if node == "" {
		log.Infof("pv %s is not a local pv, skipped", pv.Name)
		return
	}

	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	nc := e.Ctx.ClusterNodeCache.GetNodeCache(node)
	if nc == nil {
		log.Warningf("no node cache %s found", node)
		return
	}

	// handle according to different type
	switch pvType {
	case pkg.VolumeTypeLVM:
		err := nc.RemoveLVM(pv)
		if err != nil {
			log.Errorf("failed to remove local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeMountPoint:
		err := nc.RemoveLocalMountPoint(pv)
		if err != nil {
			log.Errorf("failed to remove local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeDevice:
		err := nc.RemoveLocalDevice(pv)
		if err != nil {
			log.Errorf("failed to remove local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	default:
		log.Infof("not a open-local pv %s, volumeType %s, skipped", pv.Name, pvType)
		return
	}
	log.V(6).Infof("pv %s (type: %s) is deleted from node cache %s", pv.Name, pvType, nc.NodeName)
	pvcName := utils.PVCName(pv)
	if len(pvcName) > 0 {
		delete(e.Ctx.ClusterNodeCache.BindingInfo, pvcName)
		log.V(6).Infof("%s was removed from binding info", pvcName)
	}
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}
func (e *ExtenderServer) onPVUpdate(oldObj, newObj interface{}) {
	var old *corev1.PersistentVolume
	var pv *corev1.PersistentVolume
	switch t := newObj.(type) {
	case *corev1.PersistentVolume:
		pv = t
		old = oldObj.(*corev1.PersistentVolume)
	default:
		log.Errorf("cannot convert to *v1.PersistentVolume: %v", t)
		return
	}
	log.Infof("[onPVUpdate]pv %s is handling", pv.Name)
	if pv.Status.Phase == corev1.VolumePending {
		log.Infof("pv %s is in %s status, skipped", pv.Status.Phase, pv.Name)
		return
	}
	node := utils.NodeNameFromPV(pv)
	if node == "" {
		log.Infof("pv %s is not a local pv, skipped", pv.Name)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	nc := e.Ctx.ClusterNodeCache.GetNodeCache(node)
	if nc == nil {
		log.Warningf("no node cache %s found", node)
		return
	}
	containReadonlySnapshot := false
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, containReadonlySnapshot)
	if !isOpenLocalPV {
		return
	}
	// handle according to different type
	switch pvType {
	case pkg.VolumeTypeLVM:
		// todo:
		// 此处仅处理了 extender 更新 cache 的情况
		// 当出现双模式共存（extender 和 framework）时，cache 会不准确
		// AllocatedNum 没有+1
		// request 没有新增
		// 此处引发的 bug 是: 当删除 PV 后会出现 request 值为负数。
		err := nc.UpdateLVM(old, pv)
		if err != nil {
			log.Errorf("failed to update local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeMountPoint:
		err := nc.AddLocalMountPoint(pv)
		if err != nil {
			log.Errorf("failed to update local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	case pkg.VolumeTypeDevice:
		err := nc.AddLocalDevice(pv)
		if err != nil {
			log.Errorf("failed to update local pv %s (type: %s) on node %s: %s", pv.Name, pvType, nc.NodeName, err.Error())
			return
		}
	default:
		log.Infof("not a open-local pv %s, volumeType %s, skipped", pv.Name, pvType)
		return
	}
	log.V(6).Infof("pv %s (type: %s) is updated in node cache %s", pv.Name, pvType, nc.NodeName)
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}

func (e *ExtenderServer) onPodAdd(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		log.Errorf("cannot convert to *v1.Pod: %v", obj)
		return
	}

	containReadonlySnapshot := true
	pvcs, err := algorithm.GetAllPodPvcs(pod, e.Ctx, containReadonlySnapshot)
	if err != nil {
		log.Errorf("failed to get pod pvcs: %s", err.Error())
		return
	}
	podName := utils.PodName(pod)

	containInlineVolumes, nodeName := utils.ContainInlineVolumes(pod)
	if len(pvcs) <= 0 && !containInlineVolumes {
		log.Infof("no open-local pvc found for %s", podName)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.PutPod(podName, pvcs)

	nc := e.Ctx.ClusterNodeCache.GetNodeCache(nodeName)
	if nc == nil {
		// Create a new node cache
		log.V(6).Infof("[onPodAdd]Created new node cache for %s", nodeName)
		nc = cache.NewNodeCache(nodeName)

	}
	if err := nc.AddPodInlineVolumeInfo(pod); err != nil {
		log.Infof("add pod %s inline volumeInfo failed: %s", podName, err.Error())
		return
	}
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}

func (e *ExtenderServer) onPodDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case clientgocache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*corev1.Pod)
		if !ok {
			log.Errorf("cannot convert to *v1.Pod: %v", t.Obj)
			return
		}
	default:
		log.Errorf("cannot convert to *v1.Pod: %v", t)
		return
	}
	podName := utils.PodName(pod)

	containReadonlySnapshot := true
	pvcs, err := algorithm.GetAllPodPvcs(pod, e.Ctx, containReadonlySnapshot)
	if err != nil {
		log.Errorf("failed to get pod pvcs: %s", err.Error())
		return
	}
	containInlineVolumes, nodeName := utils.ContainInlineVolumes(pod)
	if len(pvcs) <= 0 && !containInlineVolumes {
		log.Infof("no open-local pvc found for %s", podName)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.DeletePod(podName, pvcs)

	nc := e.Ctx.ClusterNodeCache.GetNodeCache(nodeName)
	if nc == nil {
		// Create a new node cache
		log.V(6).Infof("[onPodDelete]Created new node cache for %s", nodeName)
		nc = cache.NewNodeCache(nodeName)
	}
	if err := nc.DeletePodInlineVolumeInfo(pod); err != nil {
		log.Infof("delete pod %s inline volumeInfo failed: %s", podName, err.Error())
		return
	}
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}

func (e *ExtenderServer) onPodUpdate(_, newObj interface{}) {
	//var old *corev1.Pod
	var pod *corev1.Pod
	switch t := newObj.(type) {
	case *corev1.Pod:
		pod = t
		//old = oldObj.(*corev1.Pod)
	default:
		log.Errorf("cannot convert to *v1.Pod: %v", t)
		return
	}
	// if len(pod.Spec.NodeName) > 0 { // do not handle any scheduled pod
	// 	return
	// }

	containReadonlySnapshot := true
	pvcs, err := algorithm.GetAllPodPvcs(pod, e.Ctx, containReadonlySnapshot)
	if err != nil {
		log.Errorf("failed to get pod pvcs: %s", err.Error())
		return
	}
	podName := utils.PodName(pod)
	containInlineVolumes, nodeName := utils.ContainInlineVolumes(pod)
	if len(pvcs) <= 0 && !containInlineVolumes {
		log.Infof("no open-local pvc found for %s", podName)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.PutPod(podName, pvcs)
	// if a pvcs is pending, remove the selected node in a goroutine
	// so that to avoid ErrVolumeBindConflict(means the selected-node(on pvc)
	// does not match the newly selected node by scheduler
	for _, p := range pvcs {
		if p.Status.Phase != corev1.ClaimPending {
			return
		}
	}
	log.Infof("handing pod %s whose pvcs are all pending", utils.PodName(pod))
	if utils.PodPvcAllowReschedule(pvcs, nil) && pod.Spec.NodeName == "" {
		for _, pvc := range pvcs {
			contains := utils.PvcContainsSelectedNode(pvc)
			if contains {
				if atomic.CompareAndSwapInt32(&e.currentWorkingRoutines, MaxConcurrentWorkingRoutines, e.currentWorkingRoutines) {
					log.Warningf("max concurrent go routine reached: %d, wait for a later update for pod %s", MaxConcurrentWorkingRoutines, utils.PodName(pod))
					return
				}
				v := atomic.AddInt32(&e.currentWorkingRoutines, 1)
				log.Infof("[begin]current working go routine %d", v)
				go func(pvcCloned *corev1.PersistentVolumeClaim) {
					defer func() {
						v := atomic.AddInt32(&e.currentWorkingRoutines, -1)
						log.Infof("[end]current working go routine %d", v)
					}()
					oldNode := pvcCloned.Annotations[pkg.AnnoSelectedNode]
					delete(pvcCloned.Annotations, pkg.AnnoSelectedNode)
					_, err := e.kubeClient.CoreV1().PersistentVolumeClaims(pvcCloned.Namespace).Update(context.Background(), pvcCloned, v1.UpdateOptions{})
					if err != nil {
						log.Warningf("failed to remove %s from pvc %s: ", pkg.AnnoSelectedNode, utils.PVCName(pvc))
						return
					}
					log.Infof("successfully removed selected-node %q from pvc %s", oldNode, utils.PVCName(pvc))
				}(pvc.DeepCopy()) // make a copy to avoid modifying cache incidentally
			}
		}
	}
	nc := e.Ctx.ClusterNodeCache.GetNodeCache(nodeName)
	if nc == nil {
		// Create a new node cache
		log.V(6).Infof("[onPodUpdate]Created new node cache for %s", nodeName)
		nc = cache.NewNodeCache(nodeName)
	}
	if err := nc.UpdatePodInlineVolumeInfo(pod); err != nil {
		log.Infof("update pod %s inline volumeInfo failed: %s", podName, err.Error())
		return
	}
	e.Ctx.ClusterNodeCache.SetNodeCache(nc)
}

func (e *ExtenderServer) onPvcAdd(obj interface{}) {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("cannot convert to *v1.PersistentVolumeClaim: %v", obj)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.PutPvc(pvc)
}

func (e *ExtenderServer) onPvcDelete(obj interface{}) {
	var pvc *corev1.PersistentVolumeClaim
	switch t := obj.(type) {
	case *corev1.PersistentVolumeClaim:
		pvc = t
	case clientgocache.DeletedFinalStateUnknown:
		var ok bool
		pvc, ok = t.Obj.(*corev1.PersistentVolumeClaim)
		if !ok {
			log.Errorf("cannot convert to *v1.PersistentVolumeClaim: %v", t.Obj)
			return
		}
	default:
		log.Errorf("cannot convert to *v1.PersistentVolumeClaim: %v", t)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.DeletePvc(pvc)

}

func (e *ExtenderServer) onPvcUpdate(_, newObj interface{}) {
	//var old *corev1.PersistentVolumeClaim
	var pvc *corev1.PersistentVolumeClaim
	switch t := newObj.(type) {
	case *corev1.PersistentVolumeClaim:
		pvc = t
		//old = oldObj.(*corev1.PersistentVolumeClaim)
	default:
		log.Errorf("cannot convert to *v1.PersistentVolumeClaim: %v", t)
		return
	}
	e.Ctx.CtxLock.Lock()
	defer e.Ctx.CtxLock.Unlock()
	e.Ctx.ClusterNodeCache.PvcMapping.PutPvc(pvc)
}
