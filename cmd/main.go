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

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alibaba/open-local/cmd/agent"
	"github.com/alibaba/open-local/cmd/controller"
	"github.com/alibaba/open-local/cmd/csi"
	"github.com/alibaba/open-local/cmd/doc"
	"github.com/alibaba/open-local/cmd/scheduler"
	"github.com/alibaba/open-local/cmd/version"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	log "k8s.io/klog/v2"
)

var (
	MainCmd = &cobra.Command{
		Use: "open-local",
	}
	VERSION  string = ""
	COMMITID string = ""
)

func main() {
	log.Infof("Version: %s, Commit: %s", VERSION, COMMITID)
	if err := MainCmd.Execute(); err != nil {
		fmt.Printf("open-local start error: %+v\n", err)
		os.Exit(1)
	}
}

func addCommands() {
	MainCmd.AddCommand(
		agent.Cmd,
		scheduler.Cmd,
		csi.Cmd,
		controller.Cmd,
		version.Cmd,
		doc.Cmd.Cmd,
	)
}

func init() {
	addCommands()
	MainCmd.SetGlobalNormalizationFunc(utils.WordSepNormalizeFunc)
	MainCmd.Flags().AddGoFlagSet(flag.CommandLine)
	MainCmd.DisableAutoGenTag = true

	pflag.CommandLine.SetNormalizeFunc(utils.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	_ = flag.CommandLine.Parse([]string{})
	logs.InitLogs()
}
