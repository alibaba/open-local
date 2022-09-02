/*
Copyright 2020 The Kubernetes Authors.

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

package adapter

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/alibaba/open-local/pkg/csi/client"
	log "k8s.io/klog/v2"
)

// BindingInfo represents the pvc and disk/lvm mapping
type BindingInfo struct {
	// node is the name of selected node
	Node string `json:"node"`
	// path for mount point
	Disk string `json:"disk"`
	// VgName is the name of selected volume group
	VgName string `json:"vgName"`
	// Device is the name for raw block device: /dev/vdb
	Device string `json:"device"`
	// [lvm] or [disk] or [device] or [quota]
	VolumeType string `json:"volumeType"`

	// PersistentVolumeClaim is the metakey for pvc: {namespace}/{name}
	PersistentVolumeClaim string `json:"persistentVolumeClaim"`
}

const (
	EnvSchedulerExtenderServiceIP   = "EXTENDER_SVC_IP"
	EnvSchedulerExtenderServicePort = "EXTENDER_SVC_PORT"

	DefaultSchedulerExtenderServiceName = "open-local-scheduler-extender"
	DefaultSchedulerExtenderPort        = "23000"
)

// ScheduleVolume make request and get expect schedule topology
func ScheduleVolume(volumeType, pvcName, pvcNamespace, vgName, nodeID string) (*BindingInfo, error) {
	bindingInfo := &BindingInfo{}

	// make request url
	urlPath := fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?nodeName=%s&volumeType=%s&vgName=%s", pvcNamespace, pvcName, nodeID, volumeType, vgName)
	if nodeID == "" && vgName != "" {
		urlPath = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s&vgName=%s", pvcNamespace, pvcName, volumeType, vgName)
	} else if nodeID != "" && vgName == "" {
		urlPath = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s&nodeName=%s", pvcNamespace, pvcName, volumeType, nodeID)
	} else if nodeID == "" && vgName == "" {
		urlPath = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s", pvcNamespace, pvcName, volumeType)
	}
	url := getExtenderURLHost() + urlPath

	// Request restful api
	respBody, err := client.DoRequest(url)
	if err != nil {
		log.Errorf("Schedule Volume with Url(%s) get error: %s", url, err.Error())
		return nil, err
	}
	// unmarshal json result.
	err = json.Unmarshal(respBody, bindingInfo)
	if err != nil {
		log.Errorf("Schedule Volume with Url(%s) get Unmarshal error: %s, and response: %s", url, err.Error(), string(respBody))
		return nil, err
	}

	log.V(6).Infof("Schedule Volume with Url(%s) Finished, get result: %v, %v", url, bindingInfo, string(respBody))
	return bindingInfo, nil
}

func getExtenderURLHost() string {
	extenderServicePort := os.Getenv(EnvSchedulerExtenderServicePort)
	if extenderServicePort == "" {
		extenderServicePort = DefaultSchedulerExtenderPort
	}

	var URLHost string = fmt.Sprintf("http://%s:%s", DefaultSchedulerExtenderServiceName, extenderServicePort)
	extenderServiceIP := os.Getenv(EnvSchedulerExtenderServiceIP)
	if extenderServiceIP != "" {
		ip := net.ParseIP(extenderServiceIP)
		if ip == nil {
			log.Warningf("%s is not a valid IP, so we use default url %s", extenderServiceIP, URLHost)
		}
		if ip.To4() == nil {
			// ipv6
			URLHost = fmt.Sprintf("http://[%s]:%s", extenderServiceIP, extenderServicePort)
		} else {
			// ipv4
			URLHost = fmt.Sprintf("http://%s:%s", extenderServiceIP, extenderServicePort)
		}
	}

	return URLHost
}
