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

package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alibaba/open-local/pkg/csi/lib"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	serverhelpers "github.com/google/go-microservice-helpers/server"
	log "k8s.io/klog/v2"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"
)

var (
	lvmdPort string
)

// Start start lvmd
func Start(port string) {
	lvmdPort = port
	address := fmt.Sprintf(":%s", port)
	log.Infof("Lvmd Starting with socket: %s ...", address)

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Errorf("Failed to build config: %v", err)
	}

	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		log.Errorf("Failed creates clientset: %v", err)
	}

	var cmd LvmCmd
	cmd = nil
	if clientset != nil {
		retry := 0

		for {
			if cmd, _ = newCmd(clientset); cmd != nil {
				break
			}

			time.Sleep(time.Millisecond * 1000)
			retry++

			if retry >= 60 {
				log.Errorf("retrieve nls timeout")
				break
			}
		}
	}

	if cmd == nil {
		log.Errorf("retrieve nls failed, try to run as LVM")
		cmd = &LvmCommads{}
	}
	svr := NewServer(cmd)

	serverhelpers.ListenAddress = &address
	grpcServer, _, err := serverhelpers.NewServer()
	if err != nil {
		log.Errorf("failed to init GRPC server: %v", err)
		return
	}

	lib.RegisterLVMServer(grpcServer, &svr)

	err = serverhelpers.ListenAndServe(grpcServer, nil)
	if err != nil {
		log.Errorf("failed to serve: %v", err)
		return
	}
	log.Infof("Lvmd End ...")
}

// GetLvmdPort get lvmd port
func GetLvmdPort() string {
	return lvmdPort
}

func newCmd(clientset *clientset.Clientset) (LvmCmd, error) {
	nodeName := os.Getenv("KUBE_NODE_NAME")

	if nls, err := clientset.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), nodeName, metav1.GetOptions{}); err != nil {
		if k8serr.IsNotFound(err) {
			log.Infof("node local storage %s not found, waiting for the controller to create the resource", nodeName)
		} else {
			log.Errorf("get NodeLocalStorages failed: %s", err.Error())
		}

		return nil, err
	} else {
		if nls.Spec.SpdkConfig.DeviceType != "" {
			return NewSpdkCommands(nls.Spec.SpdkConfig.RpcSocket), nil
		} else {
			return &LvmCommads{}, nil
		}
	}
}
