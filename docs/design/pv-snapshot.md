# 存储卷快照

## Motivation

Kubernetes 从 1.17版本 开始，Volume Snapshot 功能进入 Beta 阶段。为支持CSI的快照能力，K8s 社区为三方存储厂商提供 [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter) 组件，并提供相应 API，即 [VolumeSnapshot](https://kubernetes.io/zh/docs/concepts/storage/volume-snapshots/#volume-snapshots)、[VolumeSnapshotContent](https://kubernetes.io/zh/docs/concepts/storage/volume-snapshots/#volume-snapshots)、[VolumeSnapshotClass](https://kubernetes.io/zh/docs/concepts/storage/volume-snapshot-classes/#the-volumesnapshortclass-resource)。

用户基于 K8s 的 VolumeSnapshot 资源创建 PV 存储卷，新创建 PV 中的数据即为原 PV 快照点时刻的数据。

## Use Case

### 数据库增量备份

应用备份数据时，需要 **停服**（停止写入新数据）才能进行备份过程。通过 Open-Local 的快照能力，可使应用在**不停服**的情况也能够正常备份数据。

### 还原历史数据

## Proposal

### 初始配置

Open-Local 支持 VG 保留池能力，保留池**仅供**用户创建非 Open-Local 的逻辑卷以及供 Open-Local 创建 Snapshot 逻辑卷。保留池存储大小可由用户配置。**保留池不计入Open-Local Scheduler的Cache。**

### 创建快照类

集群管理员创建 VolumeSnapshotClass。

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: open-local-lvm
  annotations:
    # 设置为默认快照类
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: local.csi.aliyun.com
deletionPolicy: Delete
parameters:
  # 意为只读快照，即通过mount -o ro [device] [directory]
  csi.aliyun.com/readonly: "true"
  # 快照初始容量（未指定的话默认4Gi）
  csi.aliyun.com/snapshot-initial-size: 4Gi
  # 快照扩容阈值：当快照使用率达到阈值时触发 Agent 执行快照扩容操作（未指定的话默认50%）
  csi.aliyun.com/snapshot-expansion-threshold: 50%
  # 快照增量值：快照扩容时的增长量（未指定的话默认1Gi）
  csi.aliyun.com/snapshot-expansion-size: 1Gi
```

### 创建快照

用户创建 VolumeSnapshot 资源。

```yaml
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshot
metadata:
  name: new-snapshot-test
spec:
  source:
    # 需执行快照的PVC名称
    persistentVolumeClaimName: pvc-test
```

CSI Snapshotter 监听到 VolumeSnapshot 创建事件，于是调用 Open-Local CSI 插件的 CreateSnapshot 接口在对应节点创建实际的 Snapshot LV，快照容量为对应 VolumeSnapshotClass Parameter 中的初始容量。Snapshot LV 创建完毕后 Snapshotter 在集群中创建 VolumeSnapshotContent 资源。

```yaml
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshotContent
metadata:
  # 命名规则为 snapcontent-[VolumeSnapshot uuid]
  name: snapcontent-72d9a349-aacd-42d2-a240-d775650d2455
spec:
  deletionPolicy: Delete
  driver: local.csi.aliyun.com
  source:
    # pv的uuid
    volumeHandle: ee0cfb94-f8d4-11e9-b2d8-0242ac110002
  volumeSnapshotClassName: open-local-lvm
  volumeSnapshotRef:
    name: new-snapshot-test
    namespace: default
    uid: 72d9a349-aacd-42d2-a240-d775650d2455
```

### 基于快照创建只读PV

用户创建PVC资源

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: restore-pvc
spec:
  storageClassName: open-local-lvm
  dataSource:
    name: new-snapshot-test
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

PVC 对应 StorageClass 的 VolumeBindingMode 为 WaitForFirstConsumer，PVC 处于 Pending 状态。

用户创建 Pod 使用该 PVC，此处 Open-Local Scheduler Extender 会干预 Pod 的调度，使 Pod 调度到原 PV 所在节点上。K8s 原生调度器为 PVC 打标 volume.kubernetes.io/selected-node，此时 Open-Local 的 Provisioner 会调用CSI插件的 CreateVolume 接口创建 PV 资源。注意此处不会创建 LV 物理资源，而是直接使用先前 CreateSnapshot 接口已创建的 Snapshot LV。

### 删除快照

用户删除 Pod 资源，Kubelet 会调用 CSI 插件的 NodeUnstageVolume 和 NodeUnpublishVolume 接口，将 Snapshot LV 从 TargetPath 上卸载（umount）。

用户删除 PVC 及 PV，注意 Provisioner 不会删除 Snapshot LV。

用户删除 VolumeSnapshot。注意删除 VolumeSnapshot 前，集群中相关 PV、PVC 需已删除，否则删除 VolumeSnapshot 会报错（通过 kubectl describe 可看到）。CSI Snapshotter 监听到 VolumeSnapshot 删除事件，于是调用 Open-Local CSI 插件的 DeleteSnapshot 接口在对应节点删除 Snapshot LV 资源。

### 快照扩容

> LVM快照原理：[https://www.clevernetsystems.com/lvm-snapshots-explained/](https://www.clevernetsystems.com/lvm-snapshots-explained/)

用户创建快照时指定了初始容量，Snapshot LV 存储使用率在使用过程中会增大。

Open-Local Agent 会周期性监控每个节点上 Snapshot LV 使用量，当使用率超出设定阈值时（VolumeSnapshot Parameter 中设置的阈值），Open-Local Agent 会检查 VG 保留池是否可扩容，若可以的话则扩容 Snapshot LV。

## Implementation

### Agent部分

#### Reserved存储保留池

NodeLocalStorage 的 .Status.VolumeGroups 字段中，VG容量方面有三个值：

- Total：VG总量
- Allocatable：可供 Open-Local 使用的 VG 存储量，这里会扣除非 Open-Local 使用的 LV 容量（e.g.用户自己创建的逻辑卷）。Open-Local Scheduler 会将 Allocatable 值作为可分配的存储池总量
- Available：VG 剩余量

Agent 监控 NodeLocalStorage 资源的 Update 事件，当其资源的 .metadata.annotations 中包含 "csi.aliyun.com/storage-reserved={"yoda-pool":"100Gi", "share":"10%"}，则更新 NodeLocalStorage 的 .Status.VolumeGroups 字段的 Allocatable 值，扣除保留量。

#### 快照扩容

Agent 周期性检查节点上 Snapshot LV 的存储使用率，并根据集群中 VolumeSnapshot 对应 VolumeSnapshotClass Parameter 中声明的扩容参数进行扩容，扩容前 Open-Local Agent 会检查 VG 保留池容量是否充沛。

Agent 可考虑配置宿主机 /etc/lvm/lvm.conf 作为最后防线，由 LVM 自己执行快照扩容，以防止 Open-Local Agent Pod 因某种原因没有执行快照扩容操作。/etc/lvm/lvm.conf 中的扩容阈值默认为 50%。

Open-Local Agent扩容 Snapshot 失败需暴露 Event事件到NLS上。

### CSI部分

lvm daemon 需支持快照接口。

```go
type LVMServer interface {
	CreateSnapshot(context.Context, *CreateSnapshotRequest) (*CreateSnapshotReply, error)
	RemoveSnapshot(context.Context, *RemoveSnapshotRequest) (*RemoveSnapshotReply, error)
}

type LVMClient interface {
	CreateSnapshot(ctx context.Context, in *CreateSnapshotRequest, opts ...grpc.CallOption) (*CreateSnapshotGReply, error)
	RemoveSnapshot(ctx context.Context, in *RemoveSnapshotRequest, opts ...grpc.CallOption) (*RemoveSnapshotGReply, error)
}
```

### Scheduler Extender 部分

> Open-Local Scheduler Extender 无需 watch 集群中的快照事件，且其内部 Cache 与环境中的快照资源无关。

Open-Local Scheduler Extender 需判断 Pod 所使用 PVC 的 .Spec.DataSource.Kind 是否是 VolumeSnapshot 类型，若是的话获取该 VolumeSnapshot 对应 PV 所在的 Node 节点，并在预选阶段**仅**选取该节点。注意 Scheduler Extender 不会扣除 Cache。

Open-Local Scheduler Extender 的 PV 监听事件，在遇到 Snapshot 类型的 PV 不会从 Cache 中扣除。

