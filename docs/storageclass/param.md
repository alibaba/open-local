# Storage Class

These parameters can be configured in StorageClass:

| Parameters                  | Values                                 | Default  | Description         |
|-----------------------------|----------------------------------------|----------|---------------------|
| "csi.storage.k8s.io/fstype" | xfs, ext2, ext3, ext4 | ext4 | File system type that will be formatted during volume creation. This parameter is case sensitive! |
| "volumeType" | LVM, MountPoint, Device                | | PV type that will be created by Open-Local. This parameter is case sensitive! |
| "mediaType" | hdd,ssd |      | Media type that will be used when allocate Device for PV. The param only works when volumeType is MountPoint or Device. |
| "iops" | | | I/O operations per second. |
| "bps" | | | Throughput in KiB/s. |