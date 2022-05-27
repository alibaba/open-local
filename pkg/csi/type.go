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
	csivendor "github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const (
	DefaultEndpoint                    string = "unix://tmp/csi.sock"
	DefaultDriverName                  string = "local.csi.aliyun.com"
	DefaultEphemeralVolumeDataFilePath string = "/var/lib/kubelet/open-local-volumes.json"
	// VolumeOperationAlreadyExists is message fmt returned to CO when there is another in-flight call on the given volumeID
	VolumeOperationAlreadyExists = "An operation with the given volume=%q is already in progress"
)

type CSIPlugin struct {
	driver           *csicommon.CSIDriver
	endpoint         string
	idServer         *identityServer
	nodeServer       csivendor.NodeServer
	controllerServer *controllerServer
}
