# Open-Local Scheduling Framework

## Local Plugin

### 结构体定义

cache 维护一个表

- node
  - 存储池数组
    - 存储类型（目前仅支持 LVM 类型）
    - 总量/使用量
    - 持久卷信息
    - 临时卷信息

### Local Plugin 初始化

设置 [nls](../api/nls_zh_CN.md) event handler 事件

- add
  - 创建 cache 中的 node 和 存储池 allocatable
- update
  - 更新 cache 中的 node 和 存储池 allocatable

设置完毕后同步 nls，等待其 Factory.WaitForCacheSync 同步完毕后，设置 pv/pod handler。

设置 pv event handler 事件：处理持久卷

- add
  - cache同步：获取环境中 **Bound** 状态的 PV 信息，并更新 cache 中对应的存储池信息
  - 新PV：对于动态创建的 PV，此时 PV 状态为 **Pending**，则只需更新 cache 中的存储卷信息（调度器Reserve阶段已经添加除 PV 信息以外的 cache 信息，Reserve阶段 PV 还没有创建）
- update
  - PV扩容：更新 cache 中对应存储池信息
- delete
  - 扣除 cache 中存储池对应使用量
  - 删除 cache 中存储池 pv 信息

设置 pod event handler 事件：处理临时卷

- add
  - cache同步：获取环境中 assigned 的 Pod（nodeName不为空），并更新 cache 中对应的存储池信息
  - 新Pod：不需更新cache（reserve阶段已更新cache）
- delete
  - 删除 cache 中对应使用量
  - 删除 cache 中临时卷信息

### Filter

对于新 pod，根据其 pvc/临时卷信息 获取申请的存储量，然后判断节点存储池是否满足要求。

对于 Snapshot/Clone，判断存储池是否满足要求，同时过滤掉其他节点。

### Score

对于新 pod，根据其 pvc/临时卷信息 获取申请的存储量，并打分。

### Reserve

- 获取使用的该节点的存储池名称
- 更新 cache 中的 存储池 使用量
- 更新 cache 中存储池的持久卷/临时卷信息
- 更新 nls annotation： 持久卷 与 存储池 的映射关系

若为快照，则不需要如上操作，详见[快照设计](../design/pv-snapshot.md)。

### Unreserve

- 扣掉 cache 中的 存储池 使用量
- 删除 nls annotation 中 持久卷 与 存储池 映射关系

若为快照，则不需要如上操作。

## CSI

### CreateVolume

移除之前的 CSI -> scheduler extender 回调过程

- 根据 PV 对应的 PVC 信息获取调度节点名称（pvc annotation：volume.kubernetes.io/selected-node）
- 获取对应节点的 nls
- 从 nls 上获取存储池信息
- 删除 nls annotation 中 持久卷 与 存储池 映射关系