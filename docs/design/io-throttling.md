# IO限流方案

> 本方案仅限于 [Direct-IO](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/global_file_system/s1-manage-direct-io)
>
> 本方案通过调整 cgroupv1 的 [I/O Throttling Tunable Parameters](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/resource_management_guide/ch-subsystems_and_tunable_parameters#blkio-throttling) 来对 LogicalVolume 的 IO 进行设置

创建一个 StorageClass，对 Parameter 进行配置:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: open-local-lvm-io-throttling
provisioner: local.csi.aliyun.com
parameters:
  volumeType: "LVM"
  bps: "1048576" # same as 1Mi
  iops: "1024"
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
```

用户创建 PVC 时指定 StorageClass 为 open-local-lvm-io-throttling，Open-Local 将会创建一个 PV，其中 iops 和 bps 信息会在 PV 的 .spec.csi.volumeAttributes 字段中。

当CSI插件执行存储卷挂载操作时（ NodePublishVolume 阶段），Open-Local会获取如下信息：

- 根据 target_path 参数获得 Pod UUID
- Pod 信息（可根据 target_path 来获取），根据 Pod 信息可知
  - QoSClass
  - ~~containerID~~
    - ~~经测试，在 NodePublishVolume 阶段 Pod 的 cgroup 路径创建好了，container 的 cgroup 路径没有创建~~
- 根据 Pod 和 QoSClass 信息得到 cgroup 路径
- 创建逻辑卷
- 获取逻辑卷的 maj:min 信息
- 从 parameter 中获取 PV 的 iops 和 bps 信息
- 设置 Pod 的 cgroup blkio
