package cache

import "sync"

type StoragePool struct {
	Name        string
	Total       uint64
	Allocatable uint64
	Requested   uint64
	ExtraInfo   map[string]string
}

type StoragePools struct {
	// map key: name of storage pool
	pools map[string]*StoragePool
	mux   *sync.RWMutex
}

type LocalStorageCache struct {
	// map key: name of storage pool kind
	storagePools map[string]*StoragePools
	mux          *sync.RWMutex
}

type NodeLocalStorageCache struct {
	// map key: name of node
	nodeLocalStorageCaches map[string]*LocalStorageCache
	mux                    *sync.RWMutex
}

func NewLocalStorageCache() *LocalStorageCache {
	return &LocalStorageCache{
		storagePools: make(map[string]*StoragePools),
		mux:          &sync.RWMutex{},
	}
}

func NewStoragePools() *StoragePools {
	return &StoragePools{
		pools: make(map[string]*StoragePool),
		mux:   &sync.RWMutex{},
	}
}

func NewNodeLocalStorageCache() *NodeLocalStorageCache {
	return &NodeLocalStorageCache{
		nodeLocalStorageCaches: make(map[string]*LocalStorageCache),
		mux:                    &sync.RWMutex{},
	}
}

func (pool *StoragePool) DeepCopy() *StoragePool {
	if pool == nil {
		return nil
	}
	extraInfo := make(map[string]string)
	for infoK, infoV := range pool.ExtraInfo {
		extraInfo[infoK] = infoV
	}
	out := &StoragePool{
		Name:        pool.Name,
		Total:       pool.Total,
		Allocatable: pool.Allocatable,
		Requested:   pool.Requested,
		ExtraInfo:   extraInfo,
	}
	return out
}

func (pools *StoragePools) DeepCopy() *StoragePools {
	if pools == nil {
		return nil
	}
	out := NewStoragePools()
	for k, v := range pools.pools {
		out.pools[k] = v.DeepCopy()
	}
	return out
}

func (localStorageCache *LocalStorageCache) DeepCopy() *LocalStorageCache {
	if localStorageCache == nil {
		return nil
	}
	out := NewLocalStorageCache()
	for k, v := range localStorageCache.storagePools {
		out.storagePools[k] = v.DeepCopy()
	}
	return out
}

func (storagePools *StoragePools) GetStoragePool(poolName string) *StoragePool {
	storagePools.mux.RLock()
	defer storagePools.mux.RUnlock()
	return storagePools.pools[poolName].DeepCopy()
}

func (storagePools *StoragePools) SetStoragePool(poolName string, pool *StoragePool) {
	storagePools.mux.Lock()
	defer storagePools.mux.Unlock()
	storagePools.pools[poolName] = pool
}

func (storagePools *StoragePools) GetStoragePoolList() []*StoragePool {
	list := make([]*StoragePool, len(storagePools.pools))
	storagePools.mux.RLock()
	defer storagePools.mux.RUnlock()
	for _, pool := range storagePools.pools {
		list = append(list, pool.DeepCopy())
	}
	return list
}

func (storagePools *StoragePools) SetStoragePoolList(list []*StoragePool) {
	storagePools.mux.Lock()
	defer storagePools.mux.Unlock()
	for _, pool := range list {
		storagePools.pools[pool.Name] = pool.DeepCopy()
	}
}

func (localStorageCache *LocalStorageCache) GetStoragePools(kind string) *StoragePools {
	localStorageCache.mux.RLock()
	defer localStorageCache.mux.RUnlock()
	return localStorageCache.storagePools[kind].DeepCopy()
}

func (localStorageCache *LocalStorageCache) SetStoragePools(kind string, pools *StoragePools) {
	localStorageCache.mux.Lock()
	defer localStorageCache.mux.Unlock()
	localStorageCache.storagePools[kind] = pools
}

func (nodeLocalStorageCache *NodeLocalStorageCache) GetLocalStorageCache(nodeName string) *LocalStorageCache {
	nodeLocalStorageCache.mux.RLock()
	defer nodeLocalStorageCache.mux.RUnlock()
	return nodeLocalStorageCache.nodeLocalStorageCaches[nodeName].DeepCopy()
}

func (nodeLocalStorageCache *NodeLocalStorageCache) SetLocalStorageCache(nodeName string, storageCache *LocalStorageCache) {
	nodeLocalStorageCache.mux.Lock()
	defer nodeLocalStorageCache.mux.Unlock()
	nodeLocalStorageCache.nodeLocalStorageCaches[nodeName] = storageCache
}
