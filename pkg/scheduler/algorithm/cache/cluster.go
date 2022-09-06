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

package cache

import (
	"fmt"
	"sync"

	"github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	log "k8s.io/klog/v2"
)

type ClusterInfo struct {
	Nodes map[string]*NodeCache `json:"Nodes,omitempty"`
	// Only records the requested open-local unit to avoid duplicate scheduling request
	BindingInfo BindingMap `json:"bindingInfo,omitempty"`
	// PvcMapping records requested pod and pvc mapping
	PvcMapping *PodPvcMapping `json:"pvcMapping"`
}

// ClusterNodeCache maintains mapping of allocated local PVs and Nodes
type ClusterNodeCache struct {
	mu sync.RWMutex
	ClusterInfo
}

func NewClusterNodeCache() *ClusterNodeCache {
	nodes := make(map[string]*NodeCache)
	info := make(BindingMap)
	pvcInfo := NewPodPvcMapping()
	return &ClusterNodeCache{
		ClusterInfo: ClusterInfo{
			Nodes:       nodes,
			BindingInfo: info,
			PvcMapping:  pvcInfo,
		}}
}

func (c *ClusterNodeCache) AddNodeCache(nodeLocal *nodelocalstorage.NodeLocalStorage) *NodeCache {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.Nodes[nodeLocal.Name]; ok {
		return v
	}
	nc := NewNodeCacheFromStorage(nodeLocal)
	c.Nodes[nodeLocal.Name] = nc
	return nc
}

func (c *ClusterNodeCache) UpdateNodeCache(nodeLocal *nodelocalstorage.NodeLocalStorage) *NodeCache {
	var cachedNode *NodeCache
	var ok bool
	if cachedNode, ok = c.Nodes[nodeLocal.Name]; !ok {
		log.Warningf("node local storage %s was not added to cache yet, skip updating", nodeLocal.Name)
		return nil
	}
	updatedCache := cachedNode.UpdateNodeInfo(nodeLocal)
	c.Nodes[nodeLocal.Name] = updatedCache
	return updatedCache
}

func (c *ClusterNodeCache) GetNodeCache(nodeName string) *NodeCache {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.Nodes[nodeName]; ok {
		return v
	}
	return nil
}

func (c *ClusterNodeCache) SetNodeCache(nodeCache *NodeCache) *NodeCache {
	if nodeCache == nil || nodeCache.NodeName == "" {
		log.V(6).Infof("not set node cache, it's nil or nodeName is nil")
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.Nodes[nodeCache.NodeName]; ok {
		c.Nodes[nodeCache.NodeName] = nodeCache
		log.V(6).Infof("node cache update")
		return nodeCache
	}
	c.Nodes[nodeCache.NodeName] = nodeCache
	return nodeCache
}

// Assume updates the allocated units into cache immediately
// to avoid any potential resource over allocated
func (c *ClusterNodeCache) Assume(units []AllocatedUnit) (err error) {
	// all pass, write cache now
	//TODO(yuzhi.wx) we need to move it out, after all check pass
	for _, u := range units {
		nodeCache := c.GetNodeCache(u.NodeName)
		if nodeCache == nil {
			return fmt.Errorf("node %s not found from cache when assume", u.NodeName)
		}
		volumeType := u.VolumeType
		switch volumeType {
		case pkg.VolumeTypeLVM:
			_, err = c.assumeLVMAllocatedUnit(u, nodeCache)
		case pkg.VolumeTypeDevice:
			_, err = c.assumeDeviceAllocatedUnit(u, nodeCache)
		case pkg.VolumeTypeMountPoint:
			_, err = c.assumeMountPointAllocatedUnit(u, nodeCache)
		default:
			err = fmt.Errorf("invalid volumeType %s", volumeType)
		}
	}
	return err
}

func (c *ClusterNodeCache) assumeMountPointAllocatedUnit(unit AllocatedUnit, nodeCache *NodeCache) (*NodeCache, error) {
	nodeCache.AllocatedNum += 1

	if v, ok := nodeCache.MountPoints[ResourceName(unit.MountPoint)]; ok {
		if v.IsAllocated {
			return nil, fmt.Errorf("disk resource %s was already allocated", v.Name)
		}
	}
	// TODO(huizhi.szh): this is very dangerous, cause type of nodeCache.MountPoints is Map, which is a reference type
	// it will affect nodeCache of extender when it is modified
	nodeCache.MountPoints[ResourceName(unit.MountPoint)] = ExclusiveResource{
		Name:        unit.MountPoint,
		Device:      nodeCache.MountPoints[ResourceName(unit.MountPoint)].Device,
		MediaType:   nodeCache.MountPoints[ResourceName(unit.MountPoint)].MediaType,
		Capacity:    unit.Allocated,
		IsAllocated: true,
	}
	nodeCache.PVCRecordsByExtend[unit.PVCName] = unit
	c.SetNodeCache(nodeCache)
	return nodeCache, nil
}

func (c *ClusterNodeCache) assumeLVMAllocatedUnit(unit AllocatedUnit, nodeCache *NodeCache) (*NodeCache, error) {
	vg, ok := nodeCache.VGs[ResourceName(unit.VgName)]
	if ok {
		if vg.Requested+unit.Requested > vg.Capacity {
			return nil, fmt.Errorf("VG %s resource is not enough, requested = %d, actual left = %d", vg.Name, unit.Requested, vg.Capacity-vg.Requested)
		}
	} else {
		// vg is not found
		return nil, fmt.Errorf("vg %s/%s is not found in cache, please retry later", nodeCache.NodeName, unit.VgName)
	}
	nodeCache.AllocatedNum += 1
	nodeCache.PVCRecordsByExtend[unit.PVCName] = unit
	nodeCache.VGs[ResourceName(vg.Name)] = SharedResource{
		Name:      vg.Name,
		Capacity:  vg.Capacity,
		Requested: vg.Requested + unit.Requested,
	}
	log.V(6).Infof("assume node cache successfully: node = %s, vg = %s", nodeCache.NodeName, vg.Name)
	c.SetNodeCache(nodeCache)
	return nodeCache, nil
}

func (c *ClusterNodeCache) assumeDeviceAllocatedUnit(unit AllocatedUnit, nodeCache *NodeCache) (*NodeCache, error) {
	nodeCache.AllocatedNum += 1

	if v, ok := nodeCache.Devices[ResourceName(unit.MountPoint)]; ok {
		if v.IsAllocated {
			return nil, fmt.Errorf("disk resource %s was already allocated", v.Name)
		}
	}
	nodeCache.Devices[ResourceName(unit.Device)] = ExclusiveResource{
		Name:        unit.Device,
		Capacity:    unit.Allocated,
		Device:      unit.Device,
		MediaType:   nodeCache.Devices[ResourceName(unit.Device)].MediaType,
		IsAllocated: true,
	}
	nodeCache.PVCRecordsByExtend[unit.PVCName] = unit
	log.V(6).Infof("assume node cache successfully: node = %s, device = %s", nodeCache.NodeName, unit.Device)
	c.SetNodeCache(nodeCache)
	return nodeCache, nil
}
