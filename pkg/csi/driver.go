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
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	log "k8s.io/klog/v2"
)

type driverOptions struct {
	driverName              string
	nodeID                  string
	endpoint                string
	sysPath                 string
	cgroupDriver            string
	grpcConnectionTimeout   int
	mode                    string
	extenderSchedulerNames  []string
	frameworkSchedulerNames []string

	kubeclient  kubernetes.Interface
	localclient clientset.Interface
	snapclient  snapshot.Interface
}

var defaultDriverOptions = driverOptions{
	sysPath:                 "/host_sys",
	cgroupDriver:            "systemd",
	grpcConnectionTimeout:   DefaultConnectTimeout,
	mode:                    "all",
	extenderSchedulerNames:  []string{"default-scheduler"},
	frameworkSchedulerNames: []string{},
}

// Option configures a Driver
type Option func(*driverOptions)

func NewDriver(driverName, nodeID, endpoint string, opts ...Option) *CSIPlugin {
	driverOptions := &defaultDriverOptions

	driverOptions.driverName = driverName
	driverOptions.nodeID = nodeID
	driverOptions.endpoint = endpoint
	for _, opt := range opts {
		opt(driverOptions)
	}
	plugin := &CSIPlugin{
		options: driverOptions,
	}

	switch driverOptions.mode {
	case "all":
		plugin.controllerServer = newControllerServer(driverOptions)
		plugin.nodeServer = newNodeServer(driverOptions)
	case "controller":
		plugin.controllerServer = newControllerServer(driverOptions)
	case "node":
		plugin.nodeServer = newNodeServer(driverOptions)
	default:
		log.Fatalf("unknown mode: %s", driverOptions.mode)
	}

	utils.SetupCgroupPathFormatter(utils.CgroupDriverType(driverOptions.cgroupDriver))

	return plugin
}

func (plugin *CSIPlugin) Run() error {
	scheme, addr, err := ParseEndpoint(plugin.options.endpoint)
	if err != nil {
		return err
	}

	listener, err := net.Listen(scheme, addr)
	if err != nil {
		return err
	}

	logErr := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			log.Errorf("GRPC error: %v", err)
		}
		return resp, err
	}
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logErr),
	}
	plugin.srv = grpc.NewServer(opts...)

	csi.RegisterIdentityServer(plugin.srv, plugin)

	switch plugin.options.mode {
	case "controller":
		csi.RegisterControllerServer(plugin.srv, plugin)
	case "node":
		csi.RegisterNodeServer(plugin.srv, plugin)
	case "all":
		csi.RegisterControllerServer(plugin.srv, plugin)
		csi.RegisterNodeServer(plugin.srv, plugin)
	default:
		return fmt.Errorf("unknown mode: %s", plugin.options.mode)
	}

	log.V(4).Infof("Listening for connections on address: %#v", listener.Addr())
	return plugin.srv.Serve(listener)
}

func (plugin *CSIPlugin) Stop() {
	plugin.srv.Stop()
}

func WithSysPath(sysPath string) Option {
	return func(o *driverOptions) {
		o.sysPath = sysPath
	}
}

func WithCgroupDriver(cgroupDriver string) Option {
	return func(o *driverOptions) {
		o.cgroupDriver = cgroupDriver
	}
}

func WithGrpcConnectionTimeout(grpcConnectionTimeout int) Option {
	return func(o *driverOptions) {
		o.grpcConnectionTimeout = grpcConnectionTimeout
	}
}

func WithDriverMode(mode string) Option {
	return func(o *driverOptions) {
		o.mode = mode
	}
}

func WithExtenderSchedulerNames(extenderSchedulerNames []string) Option {
	return func(o *driverOptions) {
		o.extenderSchedulerNames = extenderSchedulerNames
	}
}

func WithFrameworkSchedulerNames(frameworkSchedulerNames []string) Option {
	return func(o *driverOptions) {
		o.frameworkSchedulerNames = frameworkSchedulerNames
	}
}

func WithKubeClient(kubeclient kubernetes.Interface) Option {
	return func(o *driverOptions) {
		o.kubeclient = kubeclient
	}
}

func WithLocalClient(localclient clientset.Interface) Option {
	return func(o *driverOptions) {
		o.localclient = localclient
	}
}

func WithSnapshotClient(snapclient snapshot.Interface) Option {
	return func(o *driverOptions) {
		o.snapclient = snapclient
	}
}

func ParseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", fmt.Errorf("could not parse endpoint: %v", err)
	}

	addr := filepath.Join(u.Host, filepath.FromSlash(u.Path))

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "tcp":
	case "unix":
		addr = filepath.Join("/", addr)
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("could not remove unix domain socket %q: %v", addr, err)
		}
	default:
		return "", "", fmt.Errorf("unsupported protocol: %s", scheme)
	}

	return scheme, addr, nil
}
