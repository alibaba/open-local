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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Cluster,shortName=nlsc

// NodeLocalStorageInitConfig is configuration for agent to create NodeLocalStorage
type NodeLocalStorageInitConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NodeLocalStorageInitConfigSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// NodeLocalStorageList contains a list of NodeLocalStorageInitConfig
type NodeLocalStorageInitConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeLocalStorageInitConfig `json:"items"`
}

// GlobalConfig is configuration for agent to create default NodeLocalStorage
type GlobalConfig struct {
	SpdkConfig         SpdkConfig         `json:"spdkConfig,omitempty"`
	ListConfig         ListConfig         `json:"listConfig,omitempty"`
	ResourceToBeInited ResourceToBeInited `json:"resourceToBeInited,omitempty"`
}

// NodeConfig is configuration for agent to create NodeLocalStorage of specific node
type NodeConfig struct {
	Selector           *metav1.LabelSelector `json:"selector,omitempty"`
	SpdkConfig         SpdkConfig            `json:"spdkConfig,omitempty"`
	ListConfig         ListConfig            `json:"listConfig,omitempty"`
	ResourceToBeInited ResourceToBeInited    `json:"resourceToBeInited,omitempty"`
}

// NodeLocalStorageInitConfigSpec is spec of NodeLocalStorageInitConfig
type NodeLocalStorageInitConfigSpec struct {
	GlobalConfig GlobalConfig `json:"globalConfig,omitempty"`
	NodesConfig  []NodeConfig `json:"nodesConfig,omitempty"`
}
