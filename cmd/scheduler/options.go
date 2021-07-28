package scheduler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oecp/open-local-storage-service/pkg"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm/priorities"
	"github.com/spf13/pflag"
	log "k8s.io/klog"
)

type ExtenderOptions struct {
	Master                  string
	Kubeconfig              string
	DataDir                 string
	Port                    int32
	EnabledNodeAntiAffinity string
	Strategy                string
}

const (
	DefaultNodeAffinityWeight int = 5
)

func (option *ExtenderOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Kubeconfig, "kubeconfig", option.Kubeconfig, "Path to the kubeconfig file to use.")
	fs.StringVar(&option.Master, "master", option.Master, "URL/IP for master.")
	fs.Int32Var(&option.Port, "port", option.Port, "Port for receiving scheduler callback, set to '0' to disable http server")
	fs.StringVar(&option.EnabledNodeAntiAffinity, "enabled-node-anti-affinity", option.EnabledNodeAntiAffinity, "whether enable node anti-affinity for open-local-storage-service storage backend, example format: 'MountPoint=5,LVM=3'")
	fs.StringVar(&option.Strategy, "scheduler-strategy", "binpack", "Scheduler Strategy: binpack or spread")
}

func (option *ExtenderOptions) ParseWeight() (weights *pkg.NodeAntiAffinityWeight, err error) {
	weights = pkg.NewNodeAntiAffinityWeight()
	if len(option.EnabledNodeAntiAffinity) > 0 {
		// example format
		raw := strings.TrimSpace(option.EnabledNodeAntiAffinity) // trim left and right
		raw = strings.ReplaceAll(raw, " ", "")                   // trim inner space
		sec := strings.Split(option.EnabledNodeAntiAffinity, ",")
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
			if weight < priorities.MinScore || weight > priorities.MaxScore {
				err = fmt.Errorf("weight is out-of-range [%d, %d], current value is %d", priorities.MinScore, priorities.MaxScore, weight)
				log.Errorf(err.Error())
				return nil, err
			}
			log.V(3).Infof("node anti-affinity for %v added, weight is %d", vt, weight)
			weights.Put(vt, weight)
		}
	}
	return
}

func (option *ExtenderOptions) ParseStrategy() error {
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
