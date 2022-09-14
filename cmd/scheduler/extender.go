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

package scheduler

import (
	"context"
	"fmt"

	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	informers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	"github.com/alibaba/open-local/pkg/scheduler/server"
	volumesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions"
	"github.com/spf13/cobra"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	log "k8s.io/klog/v2"
)

var (
	opt = extenderOption{}
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
			log.Fatalf("error :%s, quitting now\n", err.Error())
		}
	},
}

func Run(opt *extenderOption) error {
	cxt := context.TODO()

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
	localClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building open-local clientset: %s", err.Error())
	}
	snapClient, err := volumesnapshot.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building snapshot clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	localStorageInformerFactory := informers.NewSharedInformerFactory(localClient, 0)
	snapshotInformerFactory := volumesnapshotinformers.NewSharedInformerFactory(snapClient, 0)

	extenderServer := server.NewExtenderServer(kubeClient, localClient, snapClient, kubeInformerFactory, localStorageInformerFactory, snapshotInformerFactory, opt.Port, weights)

	log.Info("starting open-local scheduler extender")
	kubeInformerFactory.Start(cxt.Done())
	localStorageInformerFactory.Start(cxt.Done())
	snapshotInformerFactory.Start(cxt.Done())
	extenderServer.Start(cxt.Done())
	log.Info("quitting now")
	return nil
}
