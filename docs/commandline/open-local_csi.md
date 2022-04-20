## open-local csi

command for running csi plugin

```
open-local csi [flags]
```

### Options

```
      --driver string                 the name of CSI driver (default "local.csi.aliyun.com")
      --endpoint string               the endpointof CSI (default "unix://tmp/csi.sock")
      --grpc-connection-timeout int   grpc connection timeout(second) (default 3)
  -h, --help                          help for csi
      --lvmdIP string                 IP of lvm daemon. Set to [::] when deploying in IPv6 mode. (default "0.0.0.0")
      --lvmdPort string               Port of lvm daemon (default "1736")
      --nodeID string                 the id of node
      --path.sysfs string             Path of sysfs mountpoint (default "/host_sys")
```

### SEE ALSO

* [open-local](open-local.md)	 - 

