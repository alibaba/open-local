# https://kubernetes-csi.github.io/docs/snapshot-controller.html
snap_ver=v6.3.0
docker pull registry.k8s.io/sig-storage/snapshot-controller:$snap_ver
docker pull registry.k8s.io/sig-storage/csi-snapshotter:$snap_ver
docker pull registry.k8s.io/sig-storage/snapshot-validation-webhook:$snap_ver

# https://kubernetes-csi.github.io/docs/external-attacher.html
attacher_ver=v4.4.0
docker pull --platform=linux/amd64 --platform=linux/arm64 registry.k8s.io/sig-storage/csi-attacher:$attacher_ver

# https://kubernetes-csi.github.io/docs/external-provisioner.html
provisioner_ver=v3.5.0
docker pull --platform=linux/amd64 --platform=linux/arm64 registry.k8s.io/sig-storage/csi-provisioner:$provisioner_ver

# https://kubernetes-csi.github.io/docs/external-resizer.html

resizer_ver=v1.9.0
docker pull --platform=linux/amd64 --platform=linux/arm64 registry.k8s.io/sig-storage/csi-resizer:$resizer_ver

# https://kubernetes-csi.github.io/docs/livenessprobe.html
live_ver=v2.12.0
docker pull --platform=linux/amd64 --platform=linux/arm64 registry.k8s.io/sig-storage/livenessprobe:$live_ver

# https://kubernetes-csi.github.io/docs/node-driver-registrar.html
registrar_ver=v2.9.0
docker pull --platform=linux/amd64 --platform=linux/arm64 registry.k8s.io/sig-storage/csi-node-driver-registrar:$registrar_ver

#https://kubernetes-csi.github.io/docs/cluster-driver-registrar.html
cluster_ver=v1.0.1
docker pull --platform=linux/amd64 --platform=linux/arm64 quay.io/k8scsi/csi-cluster-driver-registrar:$cluster_ver














