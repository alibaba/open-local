## open-local csi

command for running csi plugin

```
open-local csi [flags]
```

### Options

```
      --cgroupDriver string                 the name of cgroup driver (default "systemd")
      --driver string                       the name of CSI driver (default "local.csi.aliyun.com")
      --driver-mode string                  driver mode (default "all")
      --endpoint string                     the endpointof CSI (default "unix://tmp/csi.sock")
      --extender-scheduler-names strings    extender scheduler names (default [default-scheduler])
      --framework-scheduler-names strings   framework scheduler names
      --grpc-connection-timeout int         grpc connection timeout(second) (default 3)
  -h, --help                                help for csi
      --kubeconfig string                   Path to the kubeconfig file to use.
      --lvmdPort string                     Port of lvm daemon (default "1736")
      --master string                       URL/IP for master.
      --nodeID string                       the id of node
      --path.sysfs string                   Path of sysfs mountpoint (default "/host_sys")
      --use-node-hostname                   use node hostname dns for grpc connection
      --konnectivity-uds                    apiserver-network-proxy unix socket path
      --konnectivity-proxy-host             apiserver-network-proxy server host
      --konnectivity-proxy-port             apiserver-network-proxy server port
      --konnectivity-proxy-mode             apiserver-network-proxy proxy mode, can be either 'grpc' or 'http-connect'
      --konnectivity-client-cert            apiserver-network-proxy client cert
      --konnectivity-client-key             apiserver-network-proxy client key
      --konnectivity-ca-cert                apiserver-network-proxy CA cert

```

### Options inherited from parent commands

```
      --add-dir-header                   If true, adds the file directory to the header of the log messages
      --alsologtostderr                  log to standard error as well as files
      --log-backtrace-at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log-dir string                   If non-empty, write log files in this directory
      --log-file string                  If non-empty, use this log file
      --log-file-max-size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --log-flush-frequency duration     Maximum number of seconds between log flushes (default 5s)
      --logtostderr                      log to standard error instead of files (default true)
      --one-output                       If true, only write logs to their native severity level (vs also writing to each lower severity level)
      --skip-headers                     If true, avoid header prefixes in the log messages
      --skip-log-headers                 If true, avoid headers when opening log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [open-local](open-local.md)	 - 

