## open-local agent

command for collecting local storage information

```
open-local agent [flags]
```

### Options

```
  -h, --help                help for agent
      --interval int        The interval that the agent checks the local storage at one time (default 60)
      --kubeconfig string   Path to the kubeconfig file to use.
      --lvname string       The prefix of Logical Volume Name created by open-local (default "local")
      --master string       URL/IP for master.
      --nodename string     Kubernetes node name.
      --path.mount string   Path that specifies mount path of local volumes (default "/mnt/open-local")
      --path.sysfs string   Path of sysfs mountpoint (default "/sys")
      --regexp string       regexp is used to filter device names (default "^(s|v|xv)d[a-z]+$")
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

* [open-local](./open-local.md)	 - 

