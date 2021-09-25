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
	"github.com/alibaba/open-local/pkg/om"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
	Cmd.DisableAutoGenTag = true
}

// Start will start agent
func Start(opt *csiOption) error {
	log.Infof("CSI Driver Name: %s, nodeID: %s, endPoints %s", opt.Driver, opt.NodeID, opt.Endpoint)

	// Storage devops
	go om.StorageOM()

	// go func(endPoint string) {
	driver := csi.NewDriver(opt.Driver, opt.NodeID, opt.Endpoint)
	driver.Run()
	// }(opt.Endpoint)

	return nil
}
