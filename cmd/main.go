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

package main

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/oecp/open-local-storage-service/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	log "k8s.io/klog"

	"github.com/oecp/open-local-storage-service/cmd/agent"
	"github.com/oecp/open-local-storage-service/cmd/scheduler"
	"github.com/oecp/open-local-storage-service/cmd/version"
)

var (
	MainCmd = &cobra.Command{
		Use: "open-local-storage-service",
	}
	VERSION  string = ""
	COMMITID string = ""
)

func main() {
	addCommands()
	log.Infof("Version: %s, Commit: %s", VERSION, COMMITID)
	if err := MainCmd.Execute(); err != nil {
		fmt.Printf("open-local-storage-service start error: %+v\n", err)
		os.Exit(1)
	}
}

func addCommands() {
	MainCmd.AddCommand(
		version.VersionCmd,

		// backend commands
		agent.Cmd,
		scheduler.Cmd,
	)
}

func init() {
	pflag.CommandLine.SetNormalizeFunc(utils.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	// TODO(yuzhi.wx): don't know what it is
	// Workaround the annoying "ERROR: logging before flag.Parse:"
	// https://github.com/jetstack/navigator/pull/74/files
	_ = goflag.CommandLine.Parse([]string{})
	logs.InitLogs()
}
