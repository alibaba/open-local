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
	"github.com/alibaba/open-local/pkg/agent/common"
	"github.com/spf13/pflag"
)

type agentOption struct {
	Master       string
	Kubeconfig   string
	NodeName     string
	SysPath      string
	MountPath    string
	Interval     int
	LVNamePrefix string
	RegExp       string
}

func (option *agentOption) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Kubeconfig, "kubeconfig", option.Kubeconfig, "Path to the kubeconfig file to use.")
	fs.StringVar(&option.Master, "master", option.Master, "URL/IP for master.")
	fs.StringVar(&option.NodeName, "nodename", option.NodeName, "Kubernetes node name.")
	fs.StringVar(&option.SysPath, "path.sysfs", "/sys", "Path of sysfs mountpoint")
	fs.StringVar(&option.MountPath, "path.mount", "/mnt/open-local", "Path that specifies mount path of local volumes")
	fs.IntVar(&option.Interval, "interval", common.DefaultInterval, "The interval that the agent checks the local storage at one time")
	fs.StringVar(&option.LVNamePrefix, "lvname", "local", "The prefix of Logical Volume Name created by open-local")
	fs.StringVar(&option.RegExp, "regexp", "^(s|v|xv)d[a-z]+$", "regexp is used to filter device names")
}
