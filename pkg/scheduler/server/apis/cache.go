package apis

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm/cache"
	"github.com/oecp/open-local-storage-service/pkg/utils"
	log "k8s.io/klog"
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
				log.V(8).Infof("n: %#v", c)
				n := c.Nodes[nodeName]
				utils.HttpJSON(w, http.StatusOK, &n)
			} else {
				utils.HttpResponse(w, http.StatusBadRequest, []byte(fmt.Sprintf("%s is not found", nodeName)))
			}
		} else {
			log.V(8).Infof("c: %#v", ctx.ClusterNodeCache)

			utils.HttpJSON(w, http.StatusOK, ctx.ClusterNodeCache)
		}
		return
	}
}
