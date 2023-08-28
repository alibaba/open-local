# apiserver-network-proxy (ANP)

The [apiserver-network-proxy](https://github.com/kubernetes-sigs/apiserver-network-proxy)
service, also named [Konnectivity setup](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-konnectivity/),
provides a TCP level proxy for the control plane _to_ cluster communication.

Open-Local's CSI plugin runs an LVM daemon, by default on port `1736`, allowing
the controller and node plugins to communicate with worker nodes. However, in
some cases workers might be running at the edge, behind a NAT or other network
constraints. There are platforms like OpenYurt and SuperEdge that offer proxy
tunnels and various other edge solutions. With these, you might be interested in
the [`--use-node-hostname`](/docs/commandline/open-local_csi.md) argument, which
will use the node host-name DNS, instead of its IP, for the gRPC connection.

Konnectivity relies on an [`EgressSelectorConfiguration`](https://kubernetes.io/docs/reference/config-api/apiserver-config.v1alpha1/#apiserver-k8s-io-v1alpha1-EgressSelectorConfiguration)
to proxy traffic from the kube-apiserver (KAS) into the worker nodes. KAS can be
configured to send traffic (or not) to one or more of the proxies.

Open-Local supports the Konnectivity proxy using Unix socket or http-connect.
With this, Open-Local will communicate with the nodes through the Konnectivity
proxy and reach edge worker nodes.

Following are usage examples, with relevant changes to csi-plugin args:

## Using http-connect

```yaml
spec:
  containers:
  - name: csi-plugin
    args:
    - csi
    - --konnectivity-proxy-host=rafi-konnectivity-server.rafi
    - --konnectivity-proxy-port=8090
    - --konnectivity-proxy-mode=http-connect
    - --konnectivity-client-cert=/pki/konnectivity/tls.crt
    - --konnectivity-client-key=/pki/konnectivity/tls.key
    - --konnectivity-ca-cert=/pki/konnectivity/ca.crt
    volumeMounts:
    - mountPath: /pki/konnectivity/
      name: konnectivity-client
      readOnly: true
  volumes:
  - name: konnectivity-client
    secret:
      defaultMode: 420
      secretName: rafi-pki-konnectivity-client
```

## GRPC socket

```yaml
spec:
  containers:
  - name: csi-plugin
    args:
    - csi
    - --konnectivity-uds=/etc/kubernetes/konnectivity-server/konnectivity-server.socket
    - --konnectivity-proxy-mode=grpc
    volumeMounts:
    - name: konnectivity-uds
      mountPath: /etc/kubernetes/konnectivity-server
  volumes:
  - name: konnectivity-uds
    hostPath:
      path: /etc/kubernetes/konnectivity-server
      type: DirectoryOrCreate
```
