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
	"github.com/alibaba/open-local/pkg/csi"
	lvmserver "github.com/alibaba/open-local/pkg/csi/server"
	local "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/om"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	log "k8s.io/klog/v2"
)

var (
	opt = csiOption{}
)

var Cmd = &cobra.Command{
	Use:   "csi",
	Short: "command for running csi plugin",
	Run: func(cmd *cobra.Command, args []string) {
		err := Start(&opt)
		if err != nil {
			log.Fatalf("error :%s, quitting now\n", err.Error())
		}
	},
}

func init() {
	// Cmd.Flags().AddGoFlagSet(flag.CommandLine)
	opt.addFlags(Cmd.Flags())
}

// Start will start agent
func Start(opt *csiOption) error {
	log.Infof("CSI Driver Name: %s, nodeID: %s, endPoints %s", opt.Driver, opt.NodeID, opt.Endpoint)

	// Storage devops
	go om.StorageOM()
	// local volume daemon
	// GRPC server to provide volume manage
	go lvmserver.Start(opt.LVMDPort)

	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}
	snapClient, err := snapshot.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building snapshot clientset: %s", err.Error())
	}
	localclient, err := local.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building local clientset: %s", err.Error())
	}

	driver := csi.NewDriver(
		opt.Driver,
		opt.NodeID,
		opt.Endpoint,
		csi.WithSysPath(opt.SysPath),
		csi.WithCgroupDriver(opt.CgroupDriver),
		csi.WithGrpcConnectionTimeout(opt.GrpcConnectionTimeout),
		csi.WithExtenderSchedulerNames(opt.ExtenderSchedulerNames),
		csi.WithFrameworkSchedulerNames(opt.FrameworkSchedulerNames),
		csi.WithKubeClient(kubeClient),
		csi.WithSnapshotClient(snapClient),
		csi.WithLocalClient(localclient),
	)
	if err := driver.Run(); err != nil {
		return err
	}

	return nil
}
