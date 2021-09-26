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

package agent

import (
	"fmt"

	"github.com/alibaba/open-local/pkg/agent/common"
	"github.com/alibaba/open-local/pkg/agent/controller"
	lssv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/signals"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
			log.Fatalf("error :%s, quitting now\n", err.Error())
		}
	},
}

func init() {
	opt.addFlags(Cmd.Flags())
	Cmd.DisableAutoGenTag = true
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
