# Device类型PV使用手册

- [Device类型PV使用手册](#device类型pv使用手册)
  - [独占盘配置](#独占盘配置)
  - [PV动态供应](#pv动态供应)
    - [VolumeMode为FileSystem](#volumemode为filesystem)
    - [VolumeMode为Block](#volumemode为block)
  - [注意](#注意)

## 独占盘配置

按照[用户使用手册](./user-guide_zh_CN.md)部署 Open-Local 后，集群中默认并没有供分配的独占盘。需要用户手动配置对应节点的 [NodeLocalStorage](../api/nls_zh_CN.md) 资源。

```bash
# kubectl edit nls minikube
# kubectl get nls -oyaml minikube
```

编辑对应节点的 nls spec字段如下：

```yaml
# 已忽略不重要的字段
apiVersion: csi.aliyun.com/v1alpha1
kind: NodeLocalStorage
spec:
  listConfig:
    devices:
      include:
      - /dev/sdb                # 设置 /dev/sdb 为独占盘
  nodeName: minikube
status:
  filteredStorageInfo:          # Open-Local 相关组件会自动更新 filteredStorageInfo 字段。当在 .filteredStorageInfo.device 中看到 /dev/vdb 时，表示该设备已被纳管，可被动态分配。
    devices:
    - /dev/vdb
    updateStatusInfo:
      updateStatus: accepted
  nodeStorageInfo:
    deviceInfo:
    - condition: DiskReady
      mediaType: hdd            # 可知 /dev/vdb 磁盘为 hdd 设备
      name: /dev/vdb
      readOnly: false
      total: 107374182400
```

## PV动态供应

### VolumeMode为FileSystem

Open-Local 默认包含如下存储类模板:

```bash
NAME                    PROVISIONER                RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
open-local-device-hdd   local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-device-ssd   local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-lvm          local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
open-local-lvm-xfs      local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
```

目前环境中有一块盘（/dev/vdb）被 Open-Local 纳管。创建一个Pod，该 Pod 使用名为 open-local-device-hdd 存储类模板，由该模板生成的 PV 使用的磁盘介质为 hdd，其中 PVC 的 Spec 字段 VolumeMode 设置为 Filesystem。此时创建的存储卷的文件系统默认为 ext4。用户同样可以指定文件系统，见[存储类模板介绍](./../storageclass/param.md):

```bash
# kubectl apply -f ./example/device/sts-fs.yaml
```

检查 Pod/PVC/PV 状态:

```bash
# kubectl get pod
NAME             READY   STATUS    RESTARTS   AGE
nginx-device-0   1/1     Running   0          25s
# kubectl get pvc
NAME                  STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS            AGE
html-nginx-device-0   Bound    local-702adc19-b4b7-4e8a-bb02-fd0543ca4817   5Gi        RWO            open-local-device-hdd   38s
# kubectl get pv
Name:          html-nginx-device-0
Namespace:     default
StorageClass:  open-local-device-hdd
Status:        Bound
Volume:        local-702adc19-b4b7-4e8a-bb02-fd0543ca4817
Labels:        app=nginx-device
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: local.csi.aliyun.com
               volume.kubernetes.io/selected-node: izrj9aygu9f6mp6zz2m4ehz
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      5Gi
Access Modes:  RWO
VolumeMode:    Filesystem
Mounted By:    nginx-device-0
Events:
  Type    Reason                 Age                From                                                                               Message
  ----    ------                 ----               ----                                                                               -------
  Normal  WaitForFirstConsumer   77s                persistentvolume-controller                                                        waiting for first consumer to be created before binding
  Normal  Provisioning           77s                local.csi.aliyun.com_iZrj9aygu9f6mp6zz2m4ehZ_0969585b-bd43-45d2-b043-fb8c767073f1  External provisioner is provisioning volume for claim "default/html-nginx-device-0"
  Normal  ExternalProvisioning   77s (x2 over 77s)  persistentvolume-controller                                                        waiting for a volume to be created, either by external provisioner "local.csi.aliyun.com" or manually created by system administrator
  Normal  ProvisioningSucceeded  77s                local.csi.aliyun.com_iZrj9aygu9f6mp6zz2m4ehZ_0969585b-bd43-45d2-b043-fb8c767073f1  Successfully provisioned volume local-702adc19-b4b7-4e8a-bb02-fd0543ca4817
```

### VolumeMode为Block

目前环境中有一块盘（/dev/vdb）被 Open-Local 纳管。创建一个Pod，该 Pod 使用名为 open-local-device-hdd 存储类模板，由该模板生成的 PV 使用的磁盘介质为 hdd，其中 PVC 的 Spec 字段 VolumeMode 设置为 Block。此时创建的存储卷将以块设备形式出现（本例中块设备在容器 /dev/sdd 路径）:

```bash
# kubectl apply -f ./example/device/sts-block.yaml
```

检查 Pod/PVC/PV 状态:

```bash
# kubectl get pod
NAME             READY   STATUS    RESTARTS   AGE
nginx-device-0   1/1     Running   0          24m
# kubectl get pvc
NAME                  STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS            AGE
html-nginx-device-0   Bound    local-7a930cc5-73e8-4948-b46a-4b9aaed4ff4c   5Gi        RWO            open-local-device-hdd   24m
# kubectl get pv
NAME                                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                         STORAGECLASS            REASON   AGE
local-7a930cc5-73e8-4948-b46a-4b9aaed4ff4c   5Gi        RWO            Delete           Bound    default/html-nginx-device-0   open-local-device-hdd            24m
# kubectl describe pvc html-nginx-device-0
Name:          html-nginx-device-0
Namespace:     default
StorageClass:  open-local-device-hdd
Status:        Bound
Volume:        local-7a930cc5-73e8-4948-b46a-4b9aaed4ff4c
Labels:        app=nginx-device
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: local.csi.aliyun.com
               volume.kubernetes.io/selected-node: izrj9aygu9f6mp6zz2m4ehz
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      5Gi
Access Modes:  RWO
```

## 注意

独占盘类型的 PV 不支持 PV 扩容、PV 快照等操作。