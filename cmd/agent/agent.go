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

package agent

import (
	"fmt"
	"os"

	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	"github.com/oecp/open-local/pkg/agent/common"
	"github.com/oecp/open-local/pkg/agent/controller"
	lssv1alpha1 "github.com/oecp/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/oecp/open-local/pkg/generated/clientset/versioned"
	"github.com/oecp/open-local/pkg/signals"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	opt = agentOption{}
)

var Cmd = &cobra.Command{
	Use:   "agent",
	Short: "command for collecting local storage information",
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

func (option *agentOption) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Config, "config", common.DefaultConfigPath, "Path to the open-local config file to use.")
	fs.StringVar(&option.Kubeconfig, "kubeconfig", option.Kubeconfig, "Path to the kubeconfig file to use.")
	fs.StringVar(&option.Master, "master", option.Master, "URL/IP for master.")
	fs.StringVar(&option.NodeName, "nodename", option.NodeName, "Kubernetes node name.")
	fs.StringVar(&option.SysPath, "path.sysfs", "/sys", "Path of sysfs mountpoint")
	fs.StringVar(&option.MountPath, "path.mount", "/mnt/open-local", "Path that specifies mount path of local volumes")
	fs.IntVar(&option.Interval, "interval", common.DefaultInterval, "The interval that the agent checks the local storage at one time")
	fs.StringVar(&option.LVNamePrefix, "lvname", "local", "The prefix of Logical Volume Name created by open-local")
	fs.StringVar(&option.RegExp, "regexp", "^(s|v|xv)d[a-z]+$", "regexp is used to filter device names")
	fs.StringVar(&option.InitConfig, "initconfig", "open-local", "initconfig is NodeLocalStorageInitConfig(CRD) for agent to create NodeLocalStorage")
}

// Start will start agent
func Start(opt *agentOption) error {
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(opt.Master, opt.Kubeconfig)
	if err != nil {
		return fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	}

	lssClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("Error building example clientset: %s", err.Error())
	}

	snapClient, err := snapshot.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("Error building example snapClient: %s", err.Error())
	}

	config, err := getAgentConfig(opt)
	if err != nil {
		return fmt.Errorf("Error LoadAgentConfigs: %s", err.Error())
	}

	agent := controller.NewAgent(config, kubeClient, lssClient, snapClient)

	log.Info("starting open-local agent")
	if err = agent.Run(stopCh); err != nil {
		return fmt.Errorf("Error running agent: %s", err.Error())
	}
	log.Info("quitting now")
	return nil
}

// getAgentConfig returns Configuration that agent needs
func getAgentConfig(opt *agentOption) (*common.Configuration, error) {
	spec := &lssv1alpha1.NodeLocalStorageSpec{
		NodeName: opt.NodeName,
	}
	status := new(lssv1alpha1.NodeLocalStorageStatus)

	configuration := &common.Configuration{
		Nodename:                opt.NodeName,
		ConfigPath:              opt.Config,
		SysPath:                 opt.SysPath,
		MountPath:               opt.MountPath,
		DiscoverInterval:        opt.Interval,
		CRDSpec:                 spec,
		CRDStatus:               status,
		LogicalVolumeNamePrefix: opt.LVNamePrefix,
		RegExp:                  opt.RegExp,
		InitConfig:              opt.InitConfig,
	}
	return configuration, nil
}
