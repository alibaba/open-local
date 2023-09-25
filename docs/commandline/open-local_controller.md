## open-local controller

command for starting a controller

```
open-local controller [flags]
```

### Options

```
      --feature-gates mapStringBool   A set of key=value pairs that describe feature gates for alpha/experimental features. Options are:
                                      AllAlpha=true|false (ALPHA - default=false)
                                      AllBeta=true|false (BETA - default=false)
                                      OrphanedSnapshotContent=true|false (ALPHA - default=true)
                                      UpdateNLS=true|false (ALPHA - default=true)
  -h, --help                          help for controller
      --initconfig string             initconfig is NodeLocalStorageInitConfig(CRD) for controller to create NodeLocalStorage (default "open-local")
      --kubeconfig string             Path to the kubeconfig file to use.
      --master string                 URL/IP for master.
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

