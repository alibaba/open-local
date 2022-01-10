# LVM类型PV使用手册

- [LVM类型PV使用手册](#lvm类型pv使用手册)
  - [共享池配置](#共享池配置)
  - [PV动态供应](#pv动态供应)
  - [存储卷扩容](#存储卷扩容)
  - [存储卷快照](#存储卷快照)
  - [原生块设备](#原生块设备)
  - [IO 限流](#io-限流)
  - [临时卷](#临时卷)

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
NAME                           PROVISIONER                RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
open-local-device-hdd          local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-device-ssd          local.csi.aliyun.com        Delete          WaitForFirstConsumer   false                  6h56m
open-local-lvm                 local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
open-local-lvm-xfs             local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
open-local-lvm-io-throttling   local.csi.aliyun.com        Delete          WaitForFirstConsumer   true                   6h56m
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

## IO 限流

Open-Local 支持为 PV 设置 IO 限流:

```bash
# kubectl apply -f ./example/lvm/sts-io-throttling.yaml
```

Pod 处于 Running 状态后，进入 Pod 容器中:

```bash
# kubectl exec -it test-io-throttling-0 sh
```

此时存储卷是以原生块设备挂载在 /dev/sdd 上，执行 fio 命令:

```bash
# fio -name=test -filename=/dev/sdd -ioengine=psync -direct=1 -iodepth=1 -thread -bs=16k -rw=readwrite -numjobs=32 -size=1G -runtime=60 -time_based -group_reporting
```

结果如下所示，可见读写吞吐量限制在 1024KiB/s 上下:

```bash
Jobs: 32 (f=32): [M(32)][100.0%][r=1024KiB/s,w=800KiB/s][r=64,w=50 IOPS][eta 00m:00s]
test: (groupid=0, jobs=32): err= 0: pid=139: Fri Dec 24 11:20:53 2021
   read: IOPS=64, BW=1024KiB/s (1049kB/s)(60.4MiB/60406msec)
    clat (usec): min=327, max=507949, avg=352089.87, stdev=129528.82
     lat (usec): min=328, max=507950, avg=352090.72, stdev=129528.81
    clat percentiles (msec):
     |  1.00th=[   65],  5.00th=[   95], 10.00th=[  186], 20.00th=[  203],
     | 30.00th=[  296], 40.00th=[  376], 50.00th=[  393], 60.00th=[  401],
     | 70.00th=[  405], 80.00th=[  485], 90.00th=[  502], 95.00th=[  502],
     | 99.00th=[  502], 99.50th=[  502], 99.90th=[  506], 99.95th=[  506],
     | 99.99th=[  510]
   bw (  KiB/s): min=   31, max=  192, per=3.84%, avg=39.30, stdev=16.26, samples=3118
   iops        : min=    1, max=   12, avg= 2.40, stdev= 1.04, samples=3118
  write: IOPS=62, BW=993KiB/s (1017kB/s)(58.6MiB/60406msec)
    clat (usec): min=426, max=511623, avg=150543.02, stdev=128591.65
     lat (usec): min=428, max=511625, avg=150544.79, stdev=128591.67
    clat percentiles (usec):
     |  1.00th=[   469],  5.00th=[   586], 10.00th=[  1450], 20.00th=[ 14877],
     | 30.00th=[ 93848], 40.00th=[104334], 50.00th=[109577], 60.00th=[123208],
     | 70.00th=[204473], 80.00th=[295699], 90.00th=[320865], 95.00th=[404751],
     | 99.00th=[417334], 99.50th=[501220], 99.90th=[509608], 99.95th=[509608],
     | 99.99th=[509608]
   bw (  KiB/s): min=   31, max=  224, per=4.83%, avg=47.97, stdev=27.13, samples=2496
   iops        : min=    1, max=   14, avg= 2.95, stdev= 1.70, samples=2496
  lat (usec)   : 500=1.21%, 750=2.35%, 1000=0.47%
  lat (msec)   : 2=3.45%, 4=0.70%, 10=0.29%, 20=5.24%, 50=0.14%
  lat (msec)   : 100=7.97%, 250=27.60%, 500=47.56%, 750=3.01%
  cpu          : usr=0.00%, sys=0.01%, ctx=7629, majf=0, minf=14
  IO depths    : 1=100.0%, 2=0.0%, 4=0.0%, 8=0.0%, 16=0.0%, 32=0.0%, >=64=0.0%
     submit    : 0=0.0%, 4=100.0%, 8=0.0%, 16=0.0%, 32=0.0%, 64=0.0%, >=64=0.0%
     complete  : 0=0.0%, 4=100.0%, 8=0.0%, 16=0.0%, 32=0.0%, 64=0.0%, >=64=0.0%
     issued rwts: total=3866,3749,0,0 short=0,0,0,0 dropped=0,0,0,0
     latency   : target=0, window=0, percentile=100.00%, depth=1

Run status group 0 (all jobs):
   READ: bw=1024KiB/s (1049kB/s), 1024KiB/s-1024KiB/s (1049kB/s-1049kB/s), io=60.4MiB (63.3MB), run=60406-60406msec
  WRITE: bw=993KiB/s (1017kB/s), 993KiB/s-993KiB/s (1017kB/s-1017kB/s), io=58.6MiB (61.4MB), run=60406-60406msec

Disk stats (read/write):
    dm-1: ios=3869/3749, merge=0/0, ticks=4848/17833, in_queue=22681, util=6.68%, aggrios=3112/3221, aggrmerge=774/631, aggrticks=3921/13598, aggrin_queue=17396, aggrutil=6.75%
  vdb: ios=3112/3221, merge=774/631, ticks=3921/13598, in_queue=17396, util=6.75%
```

若想要不同的 iops 和 bps 设置，可参考[存储类模板介绍](./../storageclass/param.md)。

## 临时卷

Open-Local 支持为 Pod 创建临时卷，其中临时卷的生命周期与 Pod 一致，即 Pod 删除后，临时卷也随之删除。可理解为 Open-Local 版本的 emptydir。

```bash
# kubectl apply -f ./example/lvm/ephemeral.yaml
```

结果如下:

```bash
# kubectl describe po file-server
Name:         file-server
Namespace:    default
Priority:     0
Node:         izrj94p95e34mrm3aow48oz/172.23.28.137
Start Time:   Tue, 11 Jan 2022 17:43:44 +0800
Labels:       <none>
Annotations:  cni.projectcalico.org/podIP: 100.84.211.72/32
              cni.projectcalico.org/podIPs: 100.84.211.72/32
Status:       Running
IP:           100.84.211.72
IPs:
  IP:  100.84.211.72
Containers:
  file-server:
    Container ID:   docker://2c2468024e89b03748070b6e96f508c6c31c7ea16aa3bf7e71a6dd534fae94b0
    Image:          filebrowser/filebrowser:latest
    Image ID:       docker-pullable://filebrowser/filebrowser@sha256:f3849e911fecead5b7da2d1c4b4ed9e5990afe04895bfb904c6b4270405277dc
    Port:           <none>
    Host Port:      <none>
    State:          Running
      Started:      Tue, 11 Jan 2022 17:43:46 +0800
    Ready:          True
    Restart Count:  0
    Environment:    <none>
    Mounts:
      /srv from webroot (rw)
      /var/run/secrets/kubernetes.io/serviceaccount from default-token-dns4c (ro)
Conditions:
  Type              Status
  Initialized       True
  Ready             True
  ContainersReady   True
  PodScheduled      True
Volumes:
  webroot:
    Type:              CSI (a Container Storage Interface (CSI) volume source)
    Driver:            local.csi.aliyun.com
    FSType:
    ReadOnly:          false
    VolumeAttributes:      size=2Gi
                           vgName=open-local-pool-0
  default-token-dns4c:
    Type:        Secret (a volume populated by a Secret)
    SecretName:  default-token-dns4c
    Optional:    false
QoS Class:       BestEffort
Node-Selectors:  <none>
Tolerations:     node.kubernetes.io/not-ready:NoExecute op=Exists for 300s
                 node.kubernetes.io/unreachable:NoExecute op=Exists for 300s
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  10m   default-scheduler  Successfully assigned default/file-server to izrj94p95e34mrm3aow48oz
  Normal  Pulling    10m   kubelet            Pulling image "filebrowser/filebrowser:latest"
  Normal  Pulled     10m   kubelet            Successfully pulled image "filebrowser/filebrowser:latest" in 1.175191049s
  Normal  Created    10m   kubelet            Created container file-server
  Normal  Started    10m   kubelet            Started container file-server
```