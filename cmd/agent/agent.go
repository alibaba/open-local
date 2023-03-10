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
	"time"

	"github.com/alibaba/open-local/pkg/agent/common"
	"github.com/alibaba/open-local/pkg/agent/controller"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	localscheme "github.com/alibaba/open-local/pkg/generated/clientset/versioned/scheme"
	localinformers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	"github.com/alibaba/open-local/pkg/restic"
	"github.com/alibaba/open-local/pkg/signals"
	"github.com/alibaba/open-local/pkg/utils"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
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
}

// Start will start agent
func Start(opt *agentOption) error {
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(opt.Master, opt.Kubeconfig)
	if err != nil {
		return fmt.Errorf("fail to build kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("fail to build kubernetes clientset: %s", err.Error())
	}
	// restic
	restic.ClusterID, err = utils.GetClusterID(kubeClient)
	if err != nil {
		return fmt.Errorf("fail to get cluster id: %s", err.Error())
	}
	log.Infof("cluster id is %s", restic.ClusterID)

	localClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("fail to build example clientset: %s", err.Error())
	}

	snapClient, err := snapshot.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("fail to build example snapClient: %s", err.Error())
	}

	config, err := getAgentConfig(opt)
	if err != nil {
		return fmt.Errorf("fail to get agent config: %s", err.Error())
	}

	utilruntime.Must(localscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "open-local-agent"})
	localInformerFactory := localinformers.NewSharedInformerFactory(localClient, time.Second*30)
	agent := controller.NewAgent(config, kubeClient, localClient, snapClient, localInformerFactory, eventRecorder)

	log.Info("starting open-local agent")
	localInformerFactory.Start(stopCh)
	if err = agent.Run(stopCh); err != nil {
		return fmt.Errorf("Error running agent: %s", err.Error())
	}
	log.Info("quitting now")
	return nil
}

// getAgentConfig returns Configuration that agent needs
func getAgentConfig(opt *agentOption) (*common.Configuration, error) {
	configuration := &common.Configuration{
		Nodename:                opt.NodeName,
		SysPath:                 opt.SysPath,
		MountPath:               opt.MountPath,
		DiscoverInterval:        opt.Interval,
		LogicalVolumeNamePrefix: opt.LVNamePrefix,
		RegExp:                  opt.RegExp,
	}
	return configuration, nil
}
