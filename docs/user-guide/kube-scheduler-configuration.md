# kube-scheduler configuration

If kube-scheduler in your cluster is [static pod](https://kubernetes.io/docs/tasks/configure-pod-container/static-pod/), Open-Local will run a Job in every master node to edit your /etc/kubernetes/manifests/kube-scheduler.yaml file by default, you can see it in [init-job](./../../helm/templates/init-job.yaml) file.

And you can also configure your kube-scheduler manually, especially when kube-scheduler in the cluster are not static pod.

Set .extender.init_job to false in [values.yaml](../../helm/values.yaml) before install Open-Local, this will not run init-job.

Create a file, name it kube-scheduler-configuration.yaml, and put it in /etc/kubernetes/ path of every master node.

```yaml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/scheduler.conf # your kubeconfig filepath
extenders:
- urlPrefix: http://open-local-scheduler-extender.kube-system:23000/scheduler
  filterVerb: predicates
  prioritizeVerb: priorities
  weight: 10
  ignorable: true
  nodeCacheCapable: true
```

If your kube-scheduler is static pod, configure your kube-scheduler file like this:

```yaml
apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: null
  labels:
    component: kube-scheduler
    tier: control-plane
  name: kube-scheduler
  namespace: kube-system
spec:
  containers:
  - command:
    - kube-scheduler
    - --address=0.0.0.0
    - --authentication-kubeconfig=/etc/kubernetes/scheduler.conf
    - --authorization-kubeconfig=/etc/kubernetes/scheduler.conf
    - --bind-address=0.0.0.0
    - --feature-gates=TTLAfterFinished=true,EphemeralContainers=true
    - --kubeconfig=/etc/kubernetes/scheduler.conf
    - --leader-elect=true
    - --port=10251
    - --profiling=false
    - --config=/etc/kubernetes/kube-scheduler-configuration.yaml # add --config option
    image: sea.hub:5000/oecp/kube-scheduler:v1.20.4-aliyun.1
    imagePullPolicy: IfNotPresent
    livenessProbe:
      failureThreshold: 8
      httpGet:
        path: /healthz
        port: 10259
        scheme: HTTPS
      initialDelaySeconds: 10
      periodSeconds: 10
      timeoutSeconds: 15
    name: kube-scheduler
    resources:
      requests:
        cpu: 100m
    startupProbe:
      failureThreshold: 24
      httpGet:
        path: /healthz
        port: 10259
        scheme: HTTPS
      initialDelaySeconds: 10
      periodSeconds: 10
      timeoutSeconds: 15
    volumeMounts:
    - mountPath: /etc/kubernetes/kube-scheduler-configuration.yaml # mount this file into the container
      name: scheduler-policy-config
      readOnly: true
    - mountPath: /etc/kubernetes/scheduler.conf
      name: kubeconfig
      readOnly: true
    - mountPath: /etc/localtime
      name: localtime
      readOnly: true
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
  priorityClassName: system-node-critical
  volumes:
  - hostPath:
      path: /etc/kubernetes/kube-scheduler-configuration.yaml
      type: File
    name: scheduler-policy-config # this is kube-scheduler-configuration.yaml that we created before
  - hostPath:
      path: /etc/kubernetes/scheduler.conf
      type: FileOrCreate
    name: kubeconfig
  - hostPath:
      path: /etc/localtime
      type: File
    name: localtime
```