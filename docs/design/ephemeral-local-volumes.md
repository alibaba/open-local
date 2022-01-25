# ephemeral-local-volumes 实现方案

> CSI ephemeral local volumes 概念参考[链接](https://kubernetes-csi.github.io/docs/ephemeral-local-volumes.html)

首先需要设置 CSIDriver 的 [podInfoOnMount](https://kubernetes-csi.github.io/docs/pod-info.html) 字段为 true。

先支持 CSI ephemeral inline volume 特性（K8s >= 1.16 就为 Beta 了），Generic Ephemeral Inline Volumes 等到成为 Beta 再考虑支持。

用户申请 Pod 如下：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: file-server
spec:
  containers:
   - name: file-server
     image: filebrowser/filebrowser:latest
     volumeMounts:
       - mountPath: /srv
         name: webroot
  volumes:
    - name: webroot
      csi:
        driver: local.csi.aliyun.com
        volumeAttributes:
          vgName: open-local-pool-0 # 【必填】临时卷所用的 VG
          size: 1Gi                 # 【选填】临时卷大小，不填默认为 1 Gi
```

## Extender 部分

- 内部Cache
  - NodeInfo 中添加一个名为 PodInlineVolumeInfo 的结构体，即 map[string][]inlineVolumeInfo，记录 pod uuid 对应的临时卷大小。inlineVolumeInfo包含 VG名称 及 临时卷大小
  - 每次更新 PodInlineVolumeInfo 结构体，一并更新 NodeInfo 中的 VGs 信息
    - Add/Update：若 PodInlineVolumeInfo 中已存在 uuid，不对 VGs 做任何操作。不存在则 PodInlineVolumeInfo 中添加 uuid，并扣除 VGs 中的存储量
    - Delete：若 PodInlineVolumeInfo 中已存在 uuid，删除 uuid，并添加 VGs 中的存储量。不存在则不对 VGs 做操作
  - 🔐机制需要做好（操作 map 要特别注意多线程操作！）

- nls
  - onNodeLocalStorageAdd/onNodeLocalStorageUpdate
    - 若 nodecache 不存在，不做任何更改
    - 若 nodecache 已存在，进入 UpdateNodeInfo 函数。这里要实现处理 PodInlineVolumeInfo 结构体的操作（类似处理LocalPV）。

- pod
  - onPodAdd/onPodUpdate
    - 需要判断 pod 是否包含临时卷
    - 若 pod 的 nodeName 不为空，且包含临时卷，则更新 PodInlineVolumeInfo
      - 若 nodecache 不存在，参考 nls 的实现，新创建一个，并更新 PodInlineVolumeInfo
      - 若 nodecache 存在，直接更新内容（修改 SetNodeCache 函数）
  - onPodDelete
    - 若 pod 的 nodeName 不为空，且包含临时卷，则需要删除 PodInlineVolumeInfo 中相关内容
      - 若 nodecache 不存在，不做处理
      - 若 nodecache 存在，直接更新内容（修改 SetNodeCache 函数）

- 调度
  - routes.go 中
    - func NeedSkip(args schedulerapi.ExtenderArgs) bool 需判断是否包含临时卷
  - CapacityPredicate 函数
    - 获取 inline volumes 信息：若出现没有 vg 名称的则报错
    - 需要针对 inline volumes 写一个容量判断函数，跟其他 pvc 判断对齐
  - CapacityMatch 函数
    - 写inline volume的score

- Metrics
  - 需要暴露所有 inline volume 信息

## CSI 部分

CSI 部分有两种方案

- 回调方案
  - kubelet 没有 NodePublishVolume 重试机制：There is no guarantee that NodePublishVolume will be called again after a failure。若有容器网络问题，则gg（不像csi可以重试）。
  - 能够保证分配物理资源前把cache扣除。但出现多个pod同时调度，如果物理资源不够也gg，因为不能触发重新调度。
- 不回调方案（采用）
  - csi侧直接扣除
  - 依赖 onPodUpdate 事件，检查 pod 是否 Running（即判断 inline volume 是否完全创建成功）

### NodePublishVolume 阶段

- 通过 volume_context["csi.storage.k8s.io/ephemeral"] 来判断是否是 ephemeral
  - 判断 req.GetVolumeContext() 中
    - 是否包含 vgName，不包含则报错
    - 是否包含 size，不包含则默认为 1Gi
  - 设置 volumeType 为 LvmVolumeType 类型
  - volume id 举例: csi-251336bf2bef6e9edd1502754b5511125d50259b81ee468b180cbe1114b5fd03，以 csi 开头，故不需要做字符串替换处理
- 修改 nodeServer 的 createVolume 函数，支持获取 pvSize 和 unit
  - 判断是否有 PV，没有的话默认为 ephemeral，从 VolumeContext 中获取 size 和 unit。
- 临时卷不支持 volumeDevices，报错内容：can only use volume source type of PersistentVolumeClaim for block mode

### NodeUnpublishVolume 阶段

- 如何判断是 ephemeral
  - 判断是否有 PV。若没有 PV，则直接删除该 LV。故不需要特别判断是否是 ephemeral。根据 getPvInfo 函数来判断。
  - 怎么判断 LV 来自哪个 VG
    - umount 时获取 mountpoint 对应的块设备路径

### Agent 信息上报

- lvname 默认包含前缀为 csi 的逻辑卷