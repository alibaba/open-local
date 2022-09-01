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

package csi

import (
	csilib "github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"golang.org/x/net/context"
	log "k8s.io/klog/v2"
)

type identityServer struct {
	*csicommon.DefaultIdentityServer
}

// newIdentityServer create identity server
func newIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// GetPluginCapabilities returns available capabilities of the plugin
func (is *identityServer) GetPluginCapabilities(ctx context.Context, req *csilib.GetPluginCapabilitiesRequest) (*csilib.GetPluginCapabilitiesResponse, error) {
	log.V(6).Infof("GetPluginCapabilities is called with req: %+v", req)
	resp := &csilib.GetPluginCapabilitiesResponse{
		Capabilities: []*csilib.PluginCapability{
			{
				Type: &csilib.PluginCapability_Service_{
					Service: &csilib.PluginCapability_Service{
						Type: csilib.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csilib.PluginCapability_Service_{
					Service: &csilib.PluginCapability_Service{
						Type: csilib.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
			{
				Type: &csilib.PluginCapability_VolumeExpansion_{
					VolumeExpansion: &csilib.PluginCapability_VolumeExpansion{
						Type: csilib.PluginCapability_VolumeExpansion_OFFLINE,
					},
				},
			},
		},
	}
	return resp, nil
}
