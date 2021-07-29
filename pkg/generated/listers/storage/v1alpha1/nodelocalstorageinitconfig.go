/*
Copyright 2021 OECP Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/oecp/open-local/pkg/apis/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NodeLocalStorageInitConfigLister helps list NodeLocalStorageInitConfigs.
type NodeLocalStorageInitConfigLister interface {
	// List lists all NodeLocalStorageInitConfigs in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.NodeLocalStorageInitConfig, err error)
	// Get retrieves the NodeLocalStorageInitConfig from the index for a given name.
	Get(name string) (*v1alpha1.NodeLocalStorageInitConfig, error)
	NodeLocalStorageInitConfigListerExpansion
}

// nodeLocalStorageInitConfigLister implements the NodeLocalStorageInitConfigLister interface.
type nodeLocalStorageInitConfigLister struct {
	indexer cache.Indexer
}

// NewNodeLocalStorageInitConfigLister returns a new NodeLocalStorageInitConfigLister.
func NewNodeLocalStorageInitConfigLister(indexer cache.Indexer) NodeLocalStorageInitConfigLister {
	return &nodeLocalStorageInitConfigLister{indexer: indexer}
}

// List lists all NodeLocalStorageInitConfigs in the indexer.
func (s *nodeLocalStorageInitConfigLister) List(selector labels.Selector) (ret []*v1alpha1.NodeLocalStorageInitConfig, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.NodeLocalStorageInitConfig))
	})
	return ret, err
}

// Get retrieves the NodeLocalStorageInitConfig from the index for a given name.
func (s *nodeLocalStorageInitConfigLister) Get(name string) (*v1alpha1.NodeLocalStorageInitConfig, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("nodelocalstorageinitconfig"), name)
	}
	return obj.(*v1alpha1.NodeLocalStorageInitConfig), nil
}
