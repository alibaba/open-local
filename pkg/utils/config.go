/*
Copyright 2022/8/26 Alibaba Cloud.

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
package utils

import (
	"fmt"
	"github.com/alibaba/open-local/pkg"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
)

const (
	DefaultNodeAffinityWeight int = 5
	MinScore                  int = 0
	MaxScore                  int = 10
)

func ParseWeight(nodeAntiAffinity string) (weights *pkg.NodeAntiAffinityWeight, err error) {
	weights = pkg.NewNodeAntiAffinityWeight()
	if len(nodeAntiAffinity) > 0 {
		// example format
		sec := strings.Split(nodeAntiAffinity, ",")
		for _, e := range sec {
			var vt pkg.VolumeType
			var weight int
			if strings.Contains(e, "=") { // e is "<volumeType>=<weight>"
				ex := strings.SplitN(e, "=", 2)
				t := ex[0]
				vt, err = pkg.VolumeTypeFromString(t)
				if err != nil {
					return nil, err
				}
				tmp, err := strconv.ParseInt(ex[1], 10, 32)
				weight = int(tmp)
				if err != nil {
					return nil, err
				}
			} else { // e is "volumeType"
				vt, err = pkg.VolumeTypeFromString(e)
				if err != nil {
					return
				}
				weight = DefaultNodeAffinityWeight
			}

			//
			if weight < MinScore || weight > MaxScore {
				err = fmt.Errorf("weight is out-of-range [%d, %d], current value is %d", MinScore, MaxScore, weight)
				klog.Errorf(err.Error())
				return nil, err
			}
			klog.Infof("node anti-affinity for %v added, weight is %d", vt, weight)
			weights.Put(vt, weight)
		}
	}
	return
}
