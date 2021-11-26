# LVM类型PV使用手册

- [LVM类型PV使用手册](#lvm类型pv使用手册)
  - [共享池配置](#共享池配置)
  - [PV动态供应](#pv动态供应)
  - [存储卷扩容](#存储卷扩容)
  - [存储卷快照](#存储卷快照)
  - [原生块设备](#原生块设备)

## 共享池配置

按照[用户使用手册](./user-guide_zh_CN.md)部署 Open-Local 后，集群中将存在名为 open-local-pool-0 的 VG。

用户也可以自定义配置共享池，详见[nls配置文档](../api/nls_zh_CN.md)和[nlsc配置文档](../api/nlsc_zh_CN.md)

以 minikube 集群为例。

```bash
# kubectl get nodelocalstorage
NAME       STATE       PHASE     AGENTUPDATEAT   SCHEDULERUPDATEAT   SCHEDULERUPDATESTATUS
minikube   DiskReady   Running   30s             0s
```

使用如下命令检查名为 open-local-pool-0 的 VG 是否被 Open-Local 成功纳管。

```bash
# kubectl get nodelocalstorage -ojson minikube|jq .status.filteredStorageInfo
{
  "updateStatusInfo": {
    "lastUpdateTime": "2021-09-23T15:37:21Z",
    "updateStatus": "accepted"
  },
  "volumeGroups": [
    "open-local-pool-0"
  ]
}
```

## PV动态供应

Open-Local 默认包含如下存储类模板:

```bash
NAME                    PROVISIONER                RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
open-local-device-hdd   local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-device-ssd   local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-lvm          local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
open-local-lvm-xfs      local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
```

创建一个Pod，该 Pod 使用名为 open-local-lvm 存储类模板。此时创建的存储卷的文件系统为 ext4，若用户指定 open-local-lvm-xfs 存储模板，则存储卷文件系统为 xfs。用户同样可以指定文件系统，见[存储类模板介绍](./../storageclass/param.md):

```bash
# kubectl apply -f ./example/lvm/sts-nginx.yaml
```

检查 Pod/PVC/PV 状态:

```bash
# kubectl get pod
NAME          READY   STATUS    RESTARTS   AGE
nginx-lvm-0   1/1     Running   0          3m5s
# kubectl get pvc
NAME               STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS     AGE
html-nginx-lvm-0   Bound    local-52f1bab4-d39b-4cde-abad-6c5963b47761   5Gi        RWO            open-local-lvm   104s
# kubectl get pv
NAME                                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                      STORAGECLASS    AGE
local-52f1bab4-d39b-4cde-abad-6c5963b47761   5Gi        RWO            Delete           Bound    default/html-nginx-lvm-0   open-local-lvm  2m4s
kubectl describe pvc html-nginx-lvm-0
Name:          html-nginx-lvm-0
Namespace:     default
StorageClass:  open-local-lvm
Status:        Bound
Volume:        local-52f1bab4-d39b-4cde-abad-6c5963b47761
Labels:        app=nginx-lvm
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: local.csi.aliyun.com
               volume.kubernetes.io/selected-node: minikube
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      5Gi
Access Modes:  RWO
VolumeMode:    Filesystem
Mounted By:    nginx-lvm-0
Events:
  Type    Reason                 Age                From                                                               Message
  ----    ------                 ----               ----                                                               -------
  Normal  WaitForFirstConsumer   11m                persistentvolume-controller                                        waiting for first consumer to be created before binding
  Normal  ExternalProvisioning   11m (x2 over 11m)  persistentvolume-controller                                        waiting for a volume to be created, either by external provisioner "local.csi.aliyun.com" or manually created by system administrator
  Normal  Provisioning           11m (x2 over 11m)  local.csi.aliyun.com_minikube_c4e4e0b8-4bac-41f7-88e4-149dba5bc058  External provisioner is provisioning volume for claim "default/html-nginx-lvm-0"
  Normal  ProvisioningSucceeded  11m (x2 over 11m)  local.csi.aliyun.com_minikube_c4e4e0b8-4bac-41f7-88e4-149dba5bc058  Successfully provisioned volume local-52f1bab4-d39b-4cde-abad-6c5963b47761
```

## 存储卷扩容

编辑对应 PVC 的 spec.resources.requests.storage 字段，将 PVC 声明的存储大小从 5Gi 扩容到 20 Gi

```bash
# kubectl patch pvc html-nginx-lvm-0 -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
```

检查 PVC/PV 状态

```bash
# kubectl get pvc
NAME                    STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS     AGE
html-nginx-lvm-0        Bound    local-52f1bab4-d39b-4cde-abad-6c5963b47761   20Gi       RWO            open-local-lvm   7h4m
# kubectl get pv
NAME                                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                           STORAGECLASS     REASON   AGE
local-52f1bab4-d39b-4cde-abad-6c5963b47761   20Gi       RWO            Delete           Bound    default/html-nginx-lvm-0        open-local-lvm            7h4m
```

## 存储卷快照

Open-Local有如下快照类:

```bash
NAME             DRIVER                DELETIONPOLICY   AGE
open-local-lvm   local.csi.aliyun.com   Delete           20m
```

创建 VolumeSnapshot 资源

```bash
# kubectl apply -f example/lvm/snapshot.yaml
volumesnapshot.snapshot.storage.k8s.io/new-snapshot-test created
# kubectl get volumesnapshot
NAME                READYTOUSE   SOURCEPVC          SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS    SNAPSHOTCONTENT                                    CREATIONTIME   AGE
new-snapshot-test   true         html-nginx-lvm-0                           1863          open-local-lvm   snapcontent-815def28-8979-408e-86de-1e408033de65   19s            19s
# kubectl get volumesnapshotcontent
NAME                                               READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER                VOLUMESNAPSHOTCLASS   VOLUMESNAPSHOT      AGE
snapcontent-815def28-8979-408e-86de-1e408033de65   true         1863          Delete           local.csi.aliyun.com   open-local-lvm        new-snapshot-test   48s
```

创建 Pod，该 Pod 对应的 PV 数据将来源于上面创建的 VolumeSnapshot 资源:

```bash
# kubectl apply -f example/lvm/sts-nginx-snap.yaml
service/nginx-lvm-snap created
statefulset.apps/nginx-lvm-snap created
# kubectl get po -l app=nginx-lvm-snap
NAME               READY   STATUS    RESTARTS   AGE
nginx-lvm-snap-0   1/1     Running   0          46s
# kubectl get pvc -l app=nginx-lvm-snap
NAME                    STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS     AGE
html-nginx-lvm-snap-0   Bound    local-1c69455d-c50b-422d-a5c0-2eb5c7d0d21b   4Gi        RWO            open-local-lvm   2m11s
# kubectl describe pvc html-nginx-lvm-snap-0
Name:          html-nginx-lvm-snap-0
Namespace:     default
StorageClass:  open-local-lvm
Status:        Bound
Volume:        local-1c69455d-c50b-422d-a5c0-2eb5c7d0d21b
Labels:        app=nginx-lvm-snap
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: local.csi.aliyun.com
               volume.kubernetes.io/selected-node: minikube
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      4Gi
Access Modes:  RWO
VolumeMode:    Filesystem
DataSource:
  APIGroup:  snapshot.storage.k8s.io
  Kind:      VolumeSnapshot
  Name:      new-snapshot-test
Mounted By:  nginx-lvm-snap-0
Events:
  Type    Reason                 Age                    From                                                               Message
  ----    ------                 ----                   ----                                                               -------
  Normal  WaitForFirstConsumer   2m37s                  persistentvolume-controller                                        waiting for first consumer to be created before binding
  Normal  ExternalProvisioning   2m37s                  persistentvolume-controller                                        waiting for a volume to be created, either by external provisioner "local.csi.aliyun.com" or manually created by system administrator
  Normal  Provisioning           2m37s (x2 over 2m37s)  local.csi.aliyun.com_minikube_c4e4e0b8-4bac-41f7-88e4-149dba5bc058  External provisioner is provisioning volume for claim "default/html-nginx-lvm-snap-0"
  Normal  ProvisioningSucceeded  2m37s (x2 over 2m37s)  local.csi.aliyun.com_minikube_c4e4e0b8-4bac-41f7-88e4-149dba5bc058  Successfully provisioned volume local-1c69455d-c50b-422d-a5c0-2eb5c7d0d21b
```

注意：若存储卷已经创建了快照资源，则该存储卷无法进行扩容操作！

## 原生块设备

Open-Local 同样支持创建的存储卷将以块设备形式出现在容器中（本例中块设备在容器 /dev/sdd 路径）:

```bash
# kubectl apply -f ./example/lvm/sts-block.yaml
```

检查 Pod/PVC/PV 状态:

```bash
# kubectl get pod
NAME                READY   STATUS    RESTARTS   AGE
nginx-lvm-block-0   1/1     Running   0          25s
# kubectl get pvc
NAME                     STATUS   VOLUME                                       CAPACITY   ACCESS MODES   STORAGECLASS     AGE
html-nginx-lvm-block-0   Bound    local-b048c19a-fe0b-455d-9f25-b23fdef03d8c   5Gi        RWO            open-local-lvm   36s
# kubectl get pv
NAME                                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                            STORAGECLASS     REASON   AGE
local-b048c19a-fe0b-455d-9f25-b23fdef03d8c   5Gi        RWO            Delete           Bound    default/html-nginx-lvm-block-0   open-local-lvm            53s
# kubectl describe pvc html-nginx-lvm-block-0
Name:          html-nginx-lvm-block-0
Namespace:     default
StorageClass:  open-local-lvm
Status:        Bound
Volume:        local-b048c19a-fe0b-455d-9f25-b23fdef03d8c
Labels:        app=nginx-lvm-block
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: local.csi.aliyun.com
               volume.kubernetes.io/selected-node: izrj96fgmgzcvhtz2vkrgez
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      5Gi
Access Modes:  RWO
VolumeMode:    Block
Mounted By:    nginx-lvm-block-0
Events:
  Type    Reason                 Age                From                                                                               Message
  ----    ------                 ----               ----                                                                               -------
  Normal  WaitForFirstConsumer   72s                persistentvolume-controller                                                        waiting for first consumer to be created before binding
  Normal  Provisioning           72s                local.csi.aliyun.com_iZrj96fgmgzcvhtz2vkrgeZ_f2b69212-7103-4f9a-a6c4-179f37036ef0  External provisioner is provisioning volume for claim "default/html-nginx-lvm-block-0"
  Normal  ExternalProvisioning   72s (x2 over 72s)  persistentvolume-controller                                                        waiting for a volume to be created, either by external provisioner "local.csi.aliyun.com" or manually created by system administrator
  Normal  ProvisioningSucceeded  72s                local.csi.aliyun.com_iZrj96fgmgzcvhtz2vkrgeZ_f2b69212-7103-4f9a-a6c4-179f37036ef0  Successfully provisioned volume local-b048c19a-fe0b-455d-9f25-b23fdef03d8c
```