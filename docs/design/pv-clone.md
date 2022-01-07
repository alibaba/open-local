# 存储卷克隆

> 参考资料 [Volume Cloning](https://kubernetes-csi.github.io/docs/volume-cloning.html)

用户创建 PVC，在 .spec.dataSource 中指定 PVC 名称。

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc-clone
spec:
  storageClassName: open-local-lvm
  dataSource:
    name: test-pvc
    kind: PersistentVolumeClaim
    apiGroup: ""
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

Open-Local scheduler extender 观察 Pod 创建事件，并判断 Pod 使用的 PVC DataSource 为 PersistentVolumeClaim，则在 Filter 阶段将其他节点排除（类似于 Open-Local 的快照方案），判断原 PV 所在节点上的存储池是否满足 PVC 申请的 Size（这里注意需与原卷 Size 一致），不满足则报错（其中需要包含失败原因）。

CSI 组件在 CreateVolume 阶段判断为克隆，则首先回调，调用 Open-Local Extender API 查询 VG 名称，并返回（注意 Response 的 Parameter 中需加上克隆信息）。

CSI 组件在 PublishVolume 阶段判断为克隆：

- 若 VolumeMode 为 FileSystem
  - 先创建 Snapshot
  - 将该 Snapshot 挂载到一个临时路径中（此处需要获取原 PV 的 FileSystem 类型）
  - 创建新 LV，格式化并挂载
  - 全量拷贝文件
- 若 VolumeMode 为 Block
  - 先创建 Snapshot
  - 创建新 LV
  - 执行 [dd 数据拷贝操作](https://serverfault.com/questions/4906/using-dd-for-disk-cloning)