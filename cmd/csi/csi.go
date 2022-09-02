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
	"github.com/alibaba/open-local/pkg/om"
	"github.com/spf13/cobra"
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

	driver := csi.NewDriver(
		opt.Driver,
		opt.NodeID,
		opt.Endpoint,
		csi.WithSysPath(opt.SysPath),
		csi.WithCgroupDriver(opt.CgroupDriver),
		csi.WithGrpcConnectionTimeout(opt.GrpcConnectionTimeout),
		csi.WithExtenderSchedulerNames(opt.ExtenderSchedulerNames),
		csi.WithFrameworkSchedulerNames(opt.FrameworkSchedulerNames),
	)
	driver.Run()

	return nil
}
