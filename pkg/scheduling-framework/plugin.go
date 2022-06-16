package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/alibaba/open-local/pkg"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	informers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type LocalPlugin struct {
	handle    framework.Handle
	pvcLister corelisters.PersistentVolumeClaimLister
	scLister  storagelisters.StorageClassLister
	// localStorageClient clientset.Interface
	// snapClient         volumesnapshotinformers.Interface
	cache *cache.NodeLocalStorageCache
}

const PluginName = "Open-Local"

var _ = framework.FilterPlugin(&LocalPlugin{})
var _ = framework.ScorePlugin(&LocalPlugin{})
var _ = framework.ReservePlugin(&LocalPlugin{})

// NewLocalPlugin
func NewLocalPlugin(configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
	// TODO: kubeConfigPath can be gotten from allocArgs
	cfg, err := clientcmd.BuildConfigFromFlags("", "/etc/kubernetes/scheduler.conf")
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %s", err.Error())
	}
	// client
	kubeClient := f.ClientSet()
	localClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("error building yoda clientset: %s", err.Error())
	}
	// snapClient, err := volumesnapshotinformers.NewForConfig(cfg)
	// if err != nil {
	// 	return nil, fmt.Errorf("error building snapshot clientset: %s", err.Error())
	// }

	// cache
	cxt := context.TODO()
	nodeCache := cache.NewNodeLocalStorageCache()
	localStorageInformerFactory := informers.NewSharedInformerFactory(localClient, 0)
	localStorageInformer := localStorageInformerFactory.Csi().V1alpha1().NodeLocalStorages().Informer()
	localStorageInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    nodeCache.OnNodeLocalStorageAdd,
		UpdateFunc: nodeCache.OnNodeLocalStorageUpdate,
	})
	localStorageInformerFactory.Start(cxt.Done())
	localStorageInformerFactory.WaitForCacheSync(cxt.Done())

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	pvInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    nodeCache.OnPVAdd,
		UpdateFunc: nodeCache.OnPVUpdate,
		DeleteFunc: nodeCache.OnPVDelete,
	})
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    nodeCache.OnPodAdd,
		DeleteFunc: nodeCache.OnPodDelete,
	})
	kubeInformerFactory.Start(cxt.Done())
	kubeInformerFactory.WaitForCacheSync(cxt.Done())

	return &LocalPlugin{
		handle:    f,
		cache:     nodeCache,
		pvcLister: kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		scLister:  kubeInformerFactory.Storage().V1().StorageClasses().Lister(),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (plugin *LocalPlugin) Name() string {
	return PluginName
}

type CacheUnit struct {
	name     string
	kind     string
	poolName string
	request  uint64
}

func (plugin *LocalPlugin) processLVM(pod *corev1.Pod, nodeName string, reserved bool) ([]CacheUnit, error) {
	// 定义一个结构体，记录一个 Pod 申请多少存储卷（持久和临时）
	// 用于返回
	// 将 pvcs 分为有 VG 和 无VG
	var cacheUnitWithPoolName []CacheUnit
	var cacheUnitWithoutPoolName []CacheUnit
	// 是否需要处理
	// 获取 pod pvc/pv 信息
	pvcs, err := plugin.getPodLocalPvcs(pod, true)
	if err != nil {
		return nil, fmt.Errorf("fail to get pod %s local pvcs: %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name), err.Error())
	}
	for _, pvc := range pvcs {
		vgName := utils.GetVGNameFromPVC(pvc, plugin.scLister)
		size := utils.GetPVCRequested(pvc)
		if vgName == "" {
			// 里面没有 vg 信息
			cacheUnitWithoutPoolName = append(cacheUnitWithoutPoolName, CacheUnit{
				name:    fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
				request: uint64(size),
				kind:    cache.PoolKindLVM,
			})
		} else {
			cacheUnitWithPoolName = append(cacheUnitWithPoolName, CacheUnit{
				name:     fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
				poolName: vgName,
				request:  uint64(size),
				kind:     cache.PoolKindLVM,
			})
		}
	}
	// 获取 pod 临时卷信息，必须只能有 VG
	if containInlineVolume, _ := utils.ContainInlineVolumes(pod); containInlineVolume {
		for _, volume := range pod.Spec.Volumes {
			if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
				vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
				if vgName == "" {
					return nil, fmt.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				}
				cacheUnitWithPoolName = append(cacheUnitWithPoolName, CacheUnit{
					name:     fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, volume.Name),
					poolName: vgName,
					request:  uint64(size),
					kind:     cache.PoolKindLVM,
				})
			}
		}
	}
	if len(cacheUnitWithoutPoolName)+len(cacheUnitWithPoolName) == 0 {
		return nil, nil
	}

	// 按照原 extender 的算法来处理
	localStorageCache := plugin.cache.GetLocalStorageCache(nodeName)
	// LVM
	storagePools := localStorageCache.GetStoragePools(cache.PoolKindLVM)
	// cacheUnitWithPoolName first
	for _, unit := range cacheUnitWithPoolName {
		// vg
		storagePool := storagePools.GetStoragePool(unit.poolName)
		if storagePool == nil {
			return nil, fmt.Errorf("no pool %s in node", unit.poolName)
		}
		// free size
		poolFreeSize := storagePool.Allocatable - storagePool.Requested
		if poolFreeSize < unit.request {
			return nil, errors.NewInsufficientLVMError(int64(unit.request), int64(storagePool.Requested), int64(storagePool.Allocatable), unit.poolName, nodeName)
		}
		// 更新临时 cache
		storagePool.Requested += unit.request
		storagePools.SetStoragePool(unit.poolName, storagePool)
	}

	// cacheUnitWithoutPoolName
	// 获取 storage pool 列表以用作排序用
	cacheVGList := storagePools.GetStoragePoolList()
	if len(cacheVGList) <= 0 {
		return nil, fmt.Errorf(errors.NewNoAvailableVGError(nodeName).GetReason())
	}
	// process pvcsWithoutVG
	for i, unit := range cacheUnitWithoutPoolName {
		// sort by free size
		sort.Slice(cacheVGList, func(i, j int) bool {
			return (cacheVGList[i].Allocatable - cacheVGList[i].Requested) < (cacheVGList[j].Allocatable - cacheVGList[j].Requested)
		})

		for j, vg := range cacheVGList {
			poolFreeSize := vg.Allocatable - vg.Requested
			log.Debugf("validating vg(name=%s,free=%d) for pvc(name=%s,requested=%d)", vg.Name, poolFreeSize, unit.name, unit.request)

			if poolFreeSize < unit.request {
				if j == len(cacheVGList)-1 {
					return nil, errors.NewInsufficientLVMError(int64(unit.request), int64(vg.Requested), int64(vg.Allocatable), unit.poolName, nodeName)
				}
				continue
			}
			cacheVGList[j].Requested += unit.request
			cacheUnitWithoutPoolName[i].poolName = vg.Name
			break
		}
	}

	if reserved {
		storagePools.SetStoragePoolList(cacheVGList)
		localStorageCache.SetStoragePools(cache.PoolKindLVM, storagePools)
		plugin.cache.SetLocalStorageCache(nodeName, localStorageCache)
	}

	return append(cacheUnitWithPoolName, cacheUnitWithoutPoolName...), nil
}

func (plugin *LocalPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	nodeName := nodeInfo.Node().Name

	_, err := plugin.processLVM(pod, nodeName, false)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	return framework.NewStatus(framework.Success)
}

const MinScore int64 = 0
const MaxScore int64 = 10

func (plugin *LocalPlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	units, err := plugin.processLVM(pod, nodeName, false)
	if err != nil {
		return 0, framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// todo: 按照类别分类

	if len(units) == 0 {
		return MinScore, framework.NewStatus(framework.Success)
	}

	// make a map store VG size pvcs used
	// key: VG name
	// value: used size
	scoreMap := make(map[string]uint64)
	for _, unit := range units {
		if size, ok := scoreMap[unit.poolName]; ok {
			size += unit.request
			scoreMap[unit.poolName] = size
		} else {
			scoreMap[unit.poolName] = unit.request
		}
	}

	var scoref float64 = 0
	count := 0
	// todo
	strategy := pkg.StrategyBinpack
	// 获取调度策略
	switch strategy {
	case pkg.StrategyBinpack:
		for vgName, used := range scoreMap {
			scoref += float64(used) / float64(plugin.cache.GetLocalStorageCache(nodeName).GetStoragePools("LVM").GetStoragePool(vgName).Allocatable)
			count++
		}
	case pkg.StrategySpread:
		for vgName, used := range scoreMap {
			scoref += (1.0 - float64(used)/float64(plugin.cache.GetLocalStorageCache(nodeName).GetStoragePools("LVM").GetStoragePool(vgName).Allocatable))
			count++
		}
	}
	score := int64(scoref / float64(count) * float64(MaxScore))
	return score, framework.NewStatus(framework.Success)
}

// saveVolumeData persists parameter data as json file at the provided location
func (plugin *LocalPlugin) saveCache(dataFilePath string) error {
	file, err := os.Create(dataFilePath)
	if err != nil {
		return fmt.Errorf("failed to save volume data file %s: %v", dataFilePath, err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(plugin.cache); err != nil {
		return fmt.Errorf("failed to save volume data file: %s", err.Error())
	}
	return nil
}

func (plugin *LocalPlugin) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	_, err := plugin.processLVM(pod, nodeName, true)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	if err := plugin.saveCache("/open-local-cache.txt"); err != nil {
		log.Errorf("fail to save cache: %s", err.Error())
	}

	return framework.NewStatus(framework.Success)
}

func (plugin *LocalPlugin) Unreserve(ctx context.Context, state *framework.CycleState, p *corev1.Pod, nodeName string) {
	log.Infof("here come pod %s and node %s, but function is not implemented", p.Name, nodeName)
	return
}

// ScoreExtensions of the Score plugin.
func (plugin *LocalPlugin) ScoreExtensions() framework.ScoreExtensions {
	return plugin
}

// NormalizeScore invoked after scoring all nodes.
func (plugin *LocalPlugin) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	// Find highest and lowest scores.
	var highest int64 = -math.MaxInt64
	var lowest int64 = math.MaxInt64
	for _, nodeScore := range scores {
		if nodeScore.Score > highest {
			highest = nodeScore.Score
		}
		if nodeScore.Score < lowest {
			lowest = nodeScore.Score
		}
	}

	// Transform the highest to lowest score range to fit the framework's min to max node score range.
	oldRange := highest - lowest
	newRange := framework.MaxNodeScore - framework.MinNodeScore
	for i, nodeScore := range scores {
		if oldRange == 0 {
			scores[i].Score = framework.MinNodeScore
		} else {
			scores[i].Score = ((nodeScore.Score - lowest) * newRange / oldRange) + framework.MinNodeScore
		}
	}

	return nil
}

//GetPodPvcs returns the pending pvcs which are needed for scheduling
func (plugin *LocalPlugin) getPodLocalPvcs(pod *corev1.Pod, skipBound bool) (pvcs []*corev1.PersistentVolumeClaim, err error) {

	ns := pod.Namespace
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			name := volume.PersistentVolumeClaim.ClaimName
			pvc, err := plugin.pvcLister.PersistentVolumeClaims(ns).Get(name)
			if err != nil {
				log.Errorf("failed to get pvc by name %s/%s: %s", ns, name, err.Error())
				return nil, err
			}
			if pvc.Status.Phase == corev1.ClaimBound && skipBound {
				log.Infof("skip scheduling bound pvc %s/%s", pvc.Namespace, pvc.Name)
				continue
			}
			// 处理 bound 和 pending 的 pvc
			scName := pvc.Spec.StorageClassName
			if scName == nil {
				continue
			}
			sc, err := plugin.scLister.Get(*scName)
			if err != nil {
				log.Errorf("failed to get storage class by name %s: %s", *scName, err.Error())
				return nil, err
			}
			if !utils.ContainsProvisioner(sc.Provisioner) {
				continue
			}
			pvType := utils.LocalPVType(sc)
			switch pvType {
			case pkg.VolumeTypeLVM:
				log.Infof("got pvc %s/%s as lvm pvc", pvc.Namespace, pvc.Name)
				pvcs = append(pvcs, pvc)
			case pkg.VolumeTypeMountPoint:
				log.Infof("got pvc %s/%s as mount point pvc, skip...", pvc.Namespace, pvc.Name)
			case pkg.VolumeTypeDevice:
				log.Infof("got pvc %s/%s as device pvc, skip...", pvc.Namespace, pvc.Name)
			default:
				log.Infof("not a open-local pvc %s/%s, should handled by other provisioner", pvc.Namespace, pvc.Name)
			}
		}
	}
	return
}
