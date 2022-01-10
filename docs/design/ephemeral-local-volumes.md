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

NodePublishVolume 阶段

- 通过 volume_context["csi.storage.k8s.io/ephemeral"] 来判断是否是 ephemeral
  - 判断 req.GetVolumeContext() 中
    - 是否包含 vgName，不包含则报错
    - 是否包含 size，不包含则默认为 1Gi
  - 设置 volumeType 为 LvmVolumeType 类型
  - volume id 举例: csi-251336bf2bef6e9edd1502754b5511125d50259b81ee468b180cbe1114b5fd03，以 csi 开头，故不需要做字符串替换处理
- 修改 nodeServer 的 createVolume 函数，支持获取 pvSize 和 unit
  - 判断是否有 PV，没有的话默认为 ephemeral，从 VolumeContext 中获取 size 和 unit。
- 临时卷不支持 volumeDevices，报错内容：can only use volume source type of PersistentVolumeClaim for block mode

NodeUnpublishVolume 阶段

- 如何判断是 ephemeral
  - 判断是否有 PV。若没有 PV，则直接删除该 LV。故不需要特别判断是否是 ephemeral。根据 getPvInfo 函数来判断。
  - 怎么判断 LV 来自哪个 VG
    - umount 时获取 mountpoint 对应的块设备路径

