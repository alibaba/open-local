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
	"github.com/alibaba/open-local/pkg/version"
	csilib "github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

func NewDriver(driverName, nodeID, endpoint string) *CSIPlugin {
	plugin := &CSIPlugin{}
	plugin.endpoint = endpoint

	csiDriver := csicommon.NewCSIDriver(driverName, version.Version, nodeID)
	plugin.driver = csiDriver
	plugin.driver.AddControllerServiceCapabilities([]csilib.ControllerServiceCapability_RPC_Type{
		csilib.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csilib.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csilib.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csilib.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
	})
	plugin.driver.AddVolumeCapabilityAccessModes([]csilib.VolumeCapability_AccessMode_Mode{csilib.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})

	plugin.idServer = newIdentityServer(csiDriver)
	plugin.nodeServer = newNodeServer(csiDriver, driverName, nodeID)
	plugin.controllerServer = newControllerServer(csiDriver)

	return plugin
}

func (plugin *CSIPlugin) Run() {
	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(plugin.endpoint, plugin.idServer, plugin.controllerServer, plugin.nodeServer)
	server.Wait()
}
