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
	"fmt"
	"net/http"

	"github.com/alibaba/open-local/pkg/scheduler/server/apis"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/julienschmidt/httprouter"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog/v2"
)

const schedulingPVCPrefix = "/apis/scheduling/:namespace/persistentvolumeclaims/:name"

func AddSchedulingApis(router *httprouter.Router, ctx *algorithm.SchedulingContext) {
	router.POST(schedulingPVCPrefix, DebugLogging(SchedulingPVCWrap(ctx), schedulingPVCPrefix))
}

// SchedulingPVCWrap handles the request during volume provisioning
func SchedulingPVCWrap(ctx *algorithm.SchedulingContext) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		var pvc *corev1.PersistentVolumeClaim
		var err error
		if pvc, err = validatePVCParams(ctx, w, r, ps); err != nil {
			err = fmt.Errorf("[SchedulingPVCWrap]failed to validate pvc params: %s", err.Error())
			log.Errorf(err.Error())
			return
		}
		var node *corev1.Node
		nodeName := r.Form.Get("nodeName")
		if len(nodeName) > 0 {
			node, err = ctx.CoreV1Informers.Nodes().Lister().Get(nodeName)
			if err != nil {
				err := fmt.Errorf("failed to fetch node %s: %s", nodeName, err.Error())
				utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
				return
			}
		}

		info, err := apis.SchedulingPVC(ctx, pvc, node)
		if err != nil {
			log.Errorf("failed to scheduling pvc %s/%s: %s", pvc.Namespace, pvc.Name, err.Error())
			utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
			return
		}
		utils.HttpJSON(w, http.StatusOK, info)
	}
}

func AddGetNodeCache(router *httprouter.Router, ctx *algorithm.SchedulingContext) {
	router.POST(cachePath, DebugLogging(apis.CacheRoute(ctx), cachePath))
}

func validatePVCParams(ctx *algorithm.SchedulingContext, w http.ResponseWriter, r *http.Request, ps httprouter.Params) (pvc *corev1.PersistentVolumeClaim, err error) {
	if err := r.ParseForm(); err != nil {
		utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		return nil, err
	}
	checkBody(w, r)
	namespace := ps.ByName("namespace")
	name := ps.ByName("name")
	if utils.IsEmpty(namespace) || utils.IsEmpty(name) {
		err := fmt.Errorf("neither namespace or name can be empty: namespace=%q,name=%q", namespace, name)
		utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		return nil, err
	}
	//var buf bytes.Buffer
	//body := io.TeeReader(r.Body, &buf)
	pvc, err = ctx.CoreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(namespace).Get(name)
	if err != nil {
		log.Errorf("failed to fetch requested pvc %s/%s: %s", namespace, name, err.Error())
		utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		return nil, err
	}

	if pvc.Status.Phase != corev1.ClaimPending {
		// support pvc expansion
		if pvc.Status.Phase == corev1.ClaimBound {
			var pvcSpecSize int64 = 0
			var pvcStatusSize int64 = 0
			if v, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				pvcSpecSize = v.Value()
			}
			if v, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				pvcStatusSize = v.Value()
			}
			if pvcSpecSize != pvcStatusSize {
				log.Infof("expand pvc size %s/%s from %d to %d", pvc.Namespace, pvc.Name, pvcStatusSize, pvcSpecSize)
				return pvc, nil
			}
		}

		err := fmt.Errorf("invalid status for scheduling pvc %s/%s: unexpected phase %s", pvc.Namespace, pvc.Name, pvc.Status.Phase)
		utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		return nil, err
	}
	return pvc, nil
}
