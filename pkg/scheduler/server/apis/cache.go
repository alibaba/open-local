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

package apis

import (
	"fmt"
	"net/http"

	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/julienschmidt/httprouter"
)

func CacheRoute(ctx *algorithm.SchedulingContext) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		if err := r.ParseForm(); err != nil {
			utils.HttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
			return
		}
		//checkBody(w, r)
		nodeName := r.Form.Get("nodeName")
		if len(nodeName) > 0 {
			nc := ctx.ClusterNodeCache.GetNodeCache(nodeName)
			if nc != nil {
				c := cache.NewClusterNodeCache()
				c.SetNodeCache(nc)
				n := c.Nodes[nodeName]
				utils.HttpJSON(w, http.StatusOK, &n)
			} else {
				utils.HttpResponse(w, http.StatusBadRequest, []byte(fmt.Sprintf("%s is not found", nodeName)))
			}
		} else {
			utils.HttpJSON(w, http.StatusOK, ctx.ClusterNodeCache)
		}
		return
	}
}
