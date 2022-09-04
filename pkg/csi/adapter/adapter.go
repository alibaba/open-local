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

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/client"
	log "k8s.io/klog/v2"
)

const (
	EnvSchedulerExtenderServiceIP   = "EXTENDER_SVC_IP"
	EnvSchedulerExtenderServicePort = "EXTENDER_SVC_PORT"

	DefaultSchedulerExtenderServiceName = "open-local-scheduler-extender"
	DefaultSchedulerExtenderPort        = "23000"
)

type Adapter interface {
	ScheduleVolume(volumeType, pvcName, pvcNamespace, vgName, nodeID string) (*pkg.BindingInfo, error)
}

type ExtenderAdapter struct {
	hostURL string
}

func NewExtenderAdapter() Adapter {
	return &ExtenderAdapter{
		hostURL: getExtenderURLHost(),
	}
}

// ScheduleVolume make request and get expect schedule topology
func (adapter *ExtenderAdapter) ScheduleVolume(volumeType, pvcName, pvcNamespace, vgName, nodeID string) (*pkg.BindingInfo, error) {
	bindingInfo := &pkg.BindingInfo{}

	// make request url
	path := fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?nodeName=%s&volumeType=%s&vgName=%s", pvcNamespace, pvcName, nodeID, volumeType, vgName)
	if nodeID == "" && vgName != "" {
		path = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s&vgName=%s", pvcNamespace, pvcName, volumeType, vgName)
	} else if nodeID != "" && vgName == "" {
		path = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s&nodeName=%s", pvcNamespace, pvcName, volumeType, nodeID)
	} else if nodeID == "" && vgName == "" {
		path = fmt.Sprintf("/apis/scheduling/%s/persistentvolumeclaims/%s?volumeType=%s", pvcNamespace, pvcName, volumeType)
	}
	url := fmt.Sprintf("%s%s", adapter.hostURL, path)

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
