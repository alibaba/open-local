/*
Copyright 2021 OECP Authors.

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
	"os"

	"github.com/oecp/open-local/pkg/csi"
	"github.com/oecp/open-local/pkg/om"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
			log.Errorf("error :%s, quitting now\n", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	opt.addFlags(Cmd.Flags())
}

func (option *csiOption) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Endpoint, "endpoint", csi.DefaultEndpoint, "the endpointof CSI")
	fs.StringVar(&option.NodeID, "nodeID", "", "the id of node")
	fs.StringVar(&option.Driver, "driver", csi.DefaultDriverName, "the name of CSI driver")
}

// Start will start agent
func Start(opt *csiOption) error {
	log.Infof("CSI Driver Name: %s, nodeID: %s, endPoints: %s", *&opt.Driver, *&opt.NodeID, *&opt.Endpoint)

	// Storage devops
	go om.StorageOM()

	// go func(endPoint string) {
	driver := csi.NewDriver(opt.Driver, opt.NodeID, opt.Endpoint)
	driver.Run()
	// }(opt.Endpoint)

	return nil
}
