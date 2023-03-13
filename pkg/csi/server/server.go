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
	"net"
	"os"
	"time"

	"github.com/alibaba/open-local/pkg/csi/lib"
	"github.com/alibaba/open-local/pkg/csi/test"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/google/credstore/client"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	snapscheme "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/scheme"
	"github.com/opentracing/opentracing-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/test/bufconn"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
)

var (
	lvmdPort string
)

// Start start lvmd
func Start(port string) {
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
	if lvmCommads, ok := cmd.(*LvmCommads); ok {
		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("fail to build kubernetes clientset: %s", err.Error())
		}
		snapClient, err := snapshot.NewForConfig(config)
		if err != nil {
			log.Fatalf("fail to build snapClient: %s", err.Error())
		}

		utilruntime.Must(snapscheme.AddToScheme(snapscheme.Scheme))
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
		eventRecorder := eventBroadcaster.NewRecorder(snapscheme.Scheme, corev1.EventSource{Component: "open-local-csiplugin"})
		lvmCommads.kubeclient = kubeClient
		lvmCommads.snapclient = snapClient
		lvmCommads.recorder = eventRecorder
	}
	svr := NewServer(cmd)

	lvmdPort = port
	address := fmt.Sprintf(":%s", port)
	log.Infof("Lvmd Starting with socket: %s ...", address)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer, _, err := NewGRPCServer()
	if err != nil {
		log.Errorf("failed to init GRPC server: %v", err)
		return
	}

	lib.RegisterLVMServer(grpcServer, &svr)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	log.Infof("Lvmd End ...")
}

func MustRunThisWhenTest() {
	const bufSize = 1024 * 1024
	testfunc := func() {
		test.Lis = bufconn.Listen(bufSize)
	}
	test.Once.Do(testfunc)
}

func StartFake() {
	svr := NewServer(&FakeCommands{})

	grpcServer, _, err := NewGRPCServer()
	if err != nil {
		log.Errorf("failed to init GRPC server: %v", err)
		return
	}

	lib.RegisterLVMServer(grpcServer, &svr)

	go func() {
		log.Infof("start fake server")
		if err := grpcServer.Serve(test.Lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

// NewServer creates a new GRPC server stub with credstore auth (if requested).
func NewGRPCServer() (*grpc.Server, *client.CredstoreClient, error) {
	var grpcServer *grpc.Server
	var cc *client.CredstoreClient

	grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(
			otgrpc.OpenTracingServerInterceptor(opentracing.GlobalTracer())))

	reflection.Register(grpcServer)
	grpc_prometheus.Register(grpcServer)

	return grpcServer, cc, nil
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
