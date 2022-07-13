# upgrade

In general, upgrade can be done by command `helm upgrade open-local [path of chart]`.

If you run `helm upgrade` and encounter errors such as "Forbidden" or "field is immutable", you can reinstall open-local:

- delete open-local
  - helm delete open-local
- install open-local
  - helm install open-local [path of chart]
- observe whether the components are ready
  - watch kubectl get po -nkube-system -l app=open-local