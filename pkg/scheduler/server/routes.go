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
	"bytes"
	// "encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alibaba/open-local/pkg/metrics"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/bind"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/predicates"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/preemptions"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/priorities"
	"github.com/alibaba/open-local/pkg/version"
	jsoniter "github.com/json-iterator/go"
	"github.com/julienschmidt/httprouter"
	"github.com/peter-wangxu/simple-golang-tools/pkg/httputil"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "k8s.io/klog/v2"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

const (
	versionPath      = "/version"
	metricsPath      = "/metrics"
	cachePath        = "/cache"
	apiPrefix        = "/scheduler"
	bindPath         = apiPrefix + "/bind"
	preemptionPath   = apiPrefix + "/preemption"
	predicatesPrefix = apiPrefix + "/predicates"
	prioritiesPrefix = apiPrefix + "/priorities"
)

func checkBody(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
}

func PredicateRoute(predicate predicates.Predicate) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)
		log.V(6).Infof("predicate name: %s, ExtenderArgs = %s", predicate.Name, buf.String())

		var extenderArgs schedulerapi.ExtenderArgs
		var extenderFilterResult *schedulerapi.ExtenderFilterResult

		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {
			extenderFilterResult = &schedulerapi.ExtenderFilterResult{
				Nodes:       nil,
				FailedNodes: nil,
				Error:       err.Error(),
			}
		} else {
			if extenderFilterResult, err = predicate.Handler(extenderArgs); err != nil {
				panic(err)
			}
		}

		if resultBody, err := json.Marshal(extenderFilterResult); err != nil {
			panic(err)
		} else {
			log.V(6).Infof("predicate name: %s, extenderFilterResult = %s", predicate.Name, string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err = w.Write(resultBody); err != nil {
				panic(err)
			}
		}
	}
}

func PrioritizeRoute(prioritize priorities.Prioritize) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)
		log.V(6).Infof("prioritize name: %s, ExtenderArgs = %s", prioritize.Name, buf.String())

		var extenderArgs schedulerapi.ExtenderArgs
		var hostPriorityList *schedulerapi.HostPriorityList
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {
			panic(err)
		}

		if list, err := prioritize.Handler(extenderArgs); err != nil {
			panic(err)
		} else {
			hostPriorityList = list
		}

		if resultBody, err := json.Marshal(hostPriorityList); err != nil {
			panic(err)
		} else {
			log.V(6).Infof("prioritize name: %s, hostPriorityList = %s", prioritize.Name, string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err = w.Write(resultBody); err != nil {
				panic(err)
			}
		}
	}
}

func BindRoute(bind bind.Bind) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)
		log.V(6).Infof("extenderBindingArgs = %s", buf.String())

		var extenderBindingArgs schedulerapi.ExtenderBindingArgs
		var extenderBindingResult *schedulerapi.ExtenderBindingResult
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.NewDecoder(body).Decode(&extenderBindingArgs); err != nil {
			extenderBindingResult = &schedulerapi.ExtenderBindingResult{
				Error: err.Error(),
			}
		} else {
			extenderBindingResult = bind.Handler(extenderBindingArgs)
		}

		if resultBody, err := json.Marshal(extenderBindingResult); err != nil {
			panic(err)
		} else {
			log.V(6).Infof("extenderBindingResult = %s", string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err = w.Write(resultBody); err != nil {
				panic(err)
			}
		}
	}
}

func PreemptionRoute(preemption preemptions.Preemption) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)
		log.V(6).Infof("extenderPreemptionArgs = %s", buf.String())

		var extenderPreemptionArgs schedulerapi.ExtenderPreemptionArgs
		var extenderPreemptionResult *schedulerapi.ExtenderPreemptionResult
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.NewDecoder(body).Decode(&extenderPreemptionArgs); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
		} else {
			extenderPreemptionResult = preemption.Handler(extenderPreemptionArgs)
		}

		if resultBody, err := json.Marshal(extenderPreemptionResult); err != nil {
			panic(err)
		} else {
			log.V(6).Infof("extenderPreemptionResult = %s", string(resultBody))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err = w.Write(resultBody); err != nil {
				panic(err)
			}
		}
	}
}

func VersionRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprintf(w, "%s-%s", version.Version, version.GitCommit)
}

func AddVersion(router *httprouter.Router) {
	router.GET(versionPath, DebugLogging(VersionRoute, versionPath))
}

func AddMetrics(router *httprouter.Router, ctx *algorithm.SchedulingContext) {
	// cannot use promhttp.Handler() (type http.Handler) as type httprouter.Handle
	// router.Handler(http.MethodGet, metricsPath, promhttp.Handler())
	handler := func(writer http.ResponseWriter, request *http.Request) {
		metrics.UpdateMetrics(ctx.ClusterNodeCache)
		promhttp.Handler().ServeHTTP(writer, request)
	}
	router.HandlerFunc(http.MethodGet, metricsPath, handler)
}

func DebugLogging(h httprouter.Handle, path string) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		wr := httputil.NewWrappedRequest(r)
		ww := httputil.NewWrappedResponseWriter(w)
		log.V(6).Infof("path: %s, request body: %s", path, string(wr.GetRequestBytes()))
		h(ww, r, p)
		log.V(6).Infof("path: %s, code=%d, response body=%s", path, ww.Code(), string(ww.Get()))
	}
}

func AddPredicate(router *httprouter.Router, predicate predicates.Predicate) {
	path := predicatesPrefix
	router.POST(path, DebugLogging(PredicateRoute(predicate), path))
}

func AddPrioritize(router *httprouter.Router, prioritize priorities.Prioritize) {
	path := prioritiesPrefix
	router.POST(path, DebugLogging(PrioritizeRoute(prioritize), path))
}

func AddBind(router *httprouter.Router, bind bind.Bind) {
	if handle, _, _ := router.Lookup("POST", bindPath); handle != nil {
		log.Warning("warning: AddBind was called more then once!")
	} else {
		router.POST(bindPath, DebugLogging(BindRoute(bind), bindPath))
	}
}

func AddPreemption(router *httprouter.Router, preemption preemptions.Preemption) {
	if handle, _, _ := router.Lookup("POST", preemptionPath); handle != nil {
		log.Warning("warning: AddPreemption was called more then once!")
	} else {
		router.POST(preemptionPath, DebugLogging(PreemptionRoute(preemption), preemptionPath))
	}
}
