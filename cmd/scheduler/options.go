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
	"fmt"
	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/spf13/pflag"
)

type extenderOption struct {
	Master                  string
	Kubeconfig              string
	DataDir                 string
	Port                    int32
	EnabledNodeAntiAffinity string
	Strategy                string
}

func (option *extenderOption) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Kubeconfig, "kubeconfig", option.Kubeconfig, "Path to the kubeconfig file to use.")
	fs.StringVar(&option.Master, "master", option.Master, "URL/IP for master.")
	fs.Int32Var(&option.Port, "port", option.Port, "Port for receiving scheduler callback, set to '0' to disable http server")
	fs.StringVar(&option.EnabledNodeAntiAffinity, "enabled-node-anti-affinity", option.EnabledNodeAntiAffinity, "whether enable node anti-affinity for open-local storage backend, example format: 'MountPoint=5,LVM=3'")
	fs.StringVar(&option.Strategy, "scheduler-strategy", "binpack", "Scheduler Strategy: binpack or spread")
}

func (option *extenderOption) ParseWeight() (weights *pkg.NodeAntiAffinityWeight, err error) {
	return utils.ParseWeight(option.EnabledNodeAntiAffinity)
}

func (option *extenderOption) ParseStrategy() error {
	switch option.Strategy {
	case "binpack":
		pkg.SchedulerStrategy = pkg.StrategyBinpack
	case "spread":
		pkg.SchedulerStrategy = pkg.StrategySpread
	default:
		return fmt.Errorf("Scheduler strategy parameter may be wrong. You can set one of those: binpack or spread")
	}

	return nil
}
