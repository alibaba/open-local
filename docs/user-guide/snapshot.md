# open-local 快照介绍

自 v0.7.0 版本起, open-local 同时支持**读写快照**和**只读快照**。

## 特性

### 创建快照

|类型|创建速度|资源消耗|快照位置|快照个数|对原始存储卷影响|
|----|----|----|----|----|----|
|只读快照|秒级|小，默认消耗4Gi存储空间|与原始存储卷同一存储池|建议3个|原始存储卷IO随快照的增多而下降|
|读写快照|时间不定，第一次创建快照全量备份，往后增量备份|大，存储容量与原始存储卷文件总量一致|集群外S3存储|不限|无影响|

### 使用快照

|类型|新存储卷特性|节点亲和性要求|集群要求|
|----|----|----|----|
|只读快照|仅能以只读模式挂载|仅能与原始存储卷同一节点|仅能与原始存储卷同一集群|
|读写快照|初始数据为快照时刻数据，存储卷可被正常读写|无要求，可调度在任何节点|无要求，可在任何集群还原数据|

## 读写快照

### 介绍

读写快照为文件级别快照，open-local 借助 restic 工具，将存储卷数据备份到外部 S3。restic 工具备份数据特点为首次全量备份，往后增量备份，故首次创建快照资源，备份时间会随存储卷内部文件大小而定。open-local 为保证备份数据的数据一致性，备份数据前会先创建临时 LVM snapshot 逻辑卷，这样可保证在原始卷无需进行 fsfreeze 的情况下完成数据备份。

### 使用

首先需保证环境中有 s3 存储，若没有可部署一个 minio。

```bash
kubectl apply -f example/lvm/minio.yaml
```

部署完毕后需在 kube-system 命名空间下创建包含 S3 信息的 secret 资源。

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-config
  namespace: kube-system
stringData:
  s3URL: [e.g. http://172.25.174.228:30659]
  s3AK: [e.g. s3 ak]
  s3SK: [e.g. s3 sk]
  s3ForcePathStyle: "true"
  s3DisableSSL: "true"
```

创建快照类，在 parameter 中指定 secret 名称及命名空间。

```yaml
apiVersion: snapshot.storage.k8s.io/v1
deletionPolicy: Retain
driver: local.csi.aliyun.com
kind: VolumeSnapshotClass
metadata:
  name: open-local-rw
parameters:
  csi.storage.k8s.io/snapshotter-secret-name: s3-config
  csi.storage.k8s.io/snapshotter-secret-namespace: kube-system
```

此时部署一个应用

```bash
kubectl apply -f example/lvm/sts-nginx.yaml
kubectl get po -l app=nginx-lvm
```

等待 Pod 状态为 Running 后， `kubectl exec` 进入容器，在 /data/目录下创建新文件，编辑旧文件内容。

创建快照

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: volumesnapshot-rw
  namespace: default
spec:
  volumeSnapshotClassName: open-local-rw
  source:
    persistentVolumeClaimName: html-nginx-lvm-0
```

执行命令查看快照 READYTOUSE 字段是否为 true

```bash
kubectl get volumesnapshot volumesnapshot-rw
```

若为 true，则创建一个新的应用，其存储 source 指向刚创建的快照。

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx-lvm-snap-rw
  labels:
    app: nginx-lvm-snap-rw
spec:
  ports:
  - port: 80
    name: web
  clusterIP: None
  selector:
    app: nginx-lvm-snap-rw
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: nginx-lvm-snap-rw
spec:
  selector:
    matchLabels:
      app: nginx-lvm-snap-rw
  podManagementPolicy: Parallel
  serviceName: "nginx-lvm-snap-rw"
  replicas: 1
  volumeClaimTemplates:
  - metadata:
      name: html
    spec:
      storageClassName: open-local-lvm
      accessModes:
        - ReadWriteOnce
      dataSource:
        name: volumesnapshot-rw
        kind: VolumeSnapshot
        apiGroup: snapshot.storage.k8s.io
      resources:
        requests:
          storage: 5Gi
  template:
    metadata:
      labels:
        app: nginx-lvm-snap-rw
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values: ["nginx-lvm"]
              topologyKey: kubernetes.io/hostname
      tolerations:
        - key: node-role.kubernetes.io/master
          operator: Exists
          effect: NoSchedule
      containers:
      - name: nginx
        image: nginx
        imagePullPolicy: Always
        volumeMounts:
        - mountPath: "/data"
          name: html
        command:
        - sh
        - "-c"
        - |
            while true; do
              echo "restic testing";
              echo "hahaha new ">>/data/yes.txt;
              sleep 1s
            done;
```

待 Pod 的状态为 Running 后，通过 `kubectl get po -owide` 可看到新的 Pod 与原 Pod 不在同一个节点；`kubectl exec` 进入刚刚创建的 Pod，查看初始化数据是否是快照时刻的内容，且是否可正常读写数据。

### 注意事项

创建读写快照我们推荐 deletionPolicy 策略为 Retain，这意味着删除了快照资源，相关 volumesnapshotcontent 资源不会同时被删除。只有在删除了 volumesnapshotcontent 资源，才会从 S3 中删除对应的存储资源。设置为 Retain 的目的在于避免出现误删整个命名空间，导致快照资源也删除，这在 velero 备份还原场景下很有用。

## 只读快照

### 介绍

只读快照借助 LVM 快照的写时拷贝技术，支持秒级创建只读快照，底层对应 LVM Snapshot 类型逻辑卷，且必须与原始卷同节点同VG。当基于快照创建新的存储卷时，open-local 实际并没有创建新的LVM逻辑卷，而是将 LVM Snapshot 逻辑卷以只读模式直接挂载至容器。LVM 逻辑卷的 IO 性能随着快照数量增多而下降，故使用者必须定期清理快照资源。

### 使用

部署 open-local 后，集群中默认存在快照类 open-local-lvm。通过 `kubectl get volumesnapshotclass open-local-lvm -oyaml` 可查看其信息。

|字段|解释|
|----|----|
|csi.aliyun.com/readonly| 是否为只读快照，若不含该 key 则默认为读写快照|
|csi.aliyun.com/snapshot-expansion-size| LVM 类型快照扩容大小|
|csi.aliyun.com/snapshot-expansion-threshold|LVM 类型快照扩容阈值|
|csi.aliyun.com/snapshot-initial-size|LVM 类型快照初始大小|

创建 VolumeSnapshot 资源

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: volumesnapshot-ro
spec:
  volumeSnapshotClassName: open-local-lvm
  source:
    persistentVolumeClaimName: html-nginx-lvm-0
```

执行命令查看快照 READYTOUSE 字段是否为 true

```bash
kubectl get volumesnapshot volumesnapshot-ro
```

若为 true，则创建一个新的应用，其存储 source 指向刚创建的快照。

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx-lvm-snap-ro
  labels:
    app: nginx-lvm-snap-ro
spec:
  ports:
  - port: 80
    name: web
  clusterIP: None
  selector:
    app: nginx-lvm-snap-ro
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: nginx-lvm-snap-ro
spec:
  selector:
    matchLabels:
      app: nginx-lvm-snap-ro
  podManagementPolicy: Parallel
  serviceName: "nginx-lvm-snap-ro"
  replicas: 1
  volumeClaimTemplates:
  - metadata:
      name: html
    spec:
      storageClassName: open-local-lvm
      accessModes:
        - ReadWriteOnce
      dataSource:
        name: volumesnapshot-ro
        kind: VolumeSnapshot
        apiGroup: snapshot.storage.k8s.io
      resources:
        requests:
          storage: 5Gi
  template:
    metadata:
      labels:
        app: nginx-lvm-snap-ro
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values: ["nginx-lvm"]
              topologyKey: kubernetes.io/hostname
      tolerations:
        - key: node-role.kubernetes.io/master
          operator: Exists
          effect: NoSchedule
      containers:
      - name: nginx
        image: nginx
        imagePullPolicy: Always
        volumeMounts:
        - mountPath: "/data"
          name: html
        command:
        - sh
        - "-c"
        - |
            while true; do
              echo "ro testing";
              echo "ro new ">>/data/yes.txt;
              sleep 1s
            done;
```

待 Pod 的状态为 Running 后，通过 `kubectl get po -owide` 可看到新的 Pod 与原 Pod 在同一个节点，即使应用设置了节点反亲和（preferredDuringSchedulingIgnoredDuringExecution）；`kubectl exec` 进入刚刚创建的 Pod，查看初始化数据是否是快照时刻的内容。同时可看到数据是只读，应用不可写，报错"Read-only file system"。

### 注意事项

使用只读快照需注意：创建 K8s 快照时， open-local 会在原存储卷相同节点相同 VG 上创建 LVM snapshot 逻辑卷，而基于快照新创建的存储卷则直接使用该 LVM snapshot 逻辑卷，并没有创建新的 LVM 逻辑卷。若出现新 PV 没有被任何应用使用时，管理员可删除 K8s 快照，之后应用则无法使用该 PV，若挂载则从 Pod 侧可看到相关 event 报错。

```
  Warning  FailedMount          2s (x5 over 10s)  kubelet            MountVolume.SetUp failed for volume "local-f2d31a88-ba7d-4841-837f-6351ff79598c" : rpc error: code = Internal desc = NodePublishVolume(mountLvmFS): fail to mount lvm volume local-f2d31a88-ba7d-4841-837f-6351ff79598c with path /var/lib/kubelet/pods/d7966d19-4ea5-42e2-bb7e-eb84b01e579f/volumes/kubernetes.io~csi/local-f2d31a88-ba7d-4841-837f-6351ff79598c/mount: rpc error: code = Internal desc = persistentvolumes "snap-3aabb27e-5e89-4bf3-8981-678cd475723f" not found
```

故使用者需管理只读快照、基于只读快照创建的PV、使用PV的Pod之间的生命周期。