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

package scheduler

import (
	"fmt"
	"os"

	volumesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v3/informers/externalversions"
	clientset "github.com/oecp/open-local/pkg/generated/clientset/versioned"
	informers "github.com/oecp/open-local/pkg/generated/informers/externalversions"
	"github.com/oecp/open-local/pkg/scheduler/server"
	"github.com/spf13/cobra"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	log "k8s.io/klog"
	"k8s.io/sample-controller/pkg/signals"
)

var (
	opt = ExtenderOptions{}
)

func init() {
	opt.AddFlags(Cmd.Flags())
}

var Cmd = &cobra.Command{
	Use:   "scheduler",
	Short: "scheduler is a scheduler extender implementation for local storage",
	Long:  `scheduler provides the capabilities for scheduling cluster local storage as a whole`,
	Run: func(cmd *cobra.Command, args []string) {
		err := Run(&opt)
		if err != nil {
			log.Errorf("error :%s, quitting now\n", err.Error())
			os.Exit(1)
		}
	},
}

func Run(opt *ExtenderOptions) error {
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	weights, err := opt.ParseWeight()
	if err != nil {
		return err
	}

	err = opt.ParseStrategy()
	if err != nil {
		return err
	}

	cfg, err := clientcmd.BuildConfigFromFlags(opt.Master, opt.Kubeconfig)
	if err != nil {
		return fmt.Errorf("error building kubeconfig: %s", err.Error())
	}
	// cfg.UserAgent = version.ExtenderNameWithVersion(false)
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientset: %s", err.Error())
	}
	lssClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building open-local clientset: %s", err.Error())
	}
	snapClient, err := volumesnapshot.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building snapshot clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	localStorageInformerFactory := informers.NewSharedInformerFactory(lssClient, 0)
	snapshotInformerFactory := volumesnapshotinformers.NewSharedInformerFactory(snapClient, 0)

	extenderServer := server.NewExtenderServer(kubeClient, lssClient, snapClient, kubeInformerFactory, localStorageInformerFactory, snapshotInformerFactory, opt.Port, weights)

	log.Info("starting open-local scheduler extender")
	kubeInformerFactory.Start(stopCh)
	localStorageInformerFactory.Start(stopCh)
	snapshotInformerFactory.Start(stopCh)
	extenderServer.Start(stopCh)
	log.Info("quitting now")
	return nil
}
