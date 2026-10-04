# Export storage: which class to use

VM export writes the OVA to a ReadWriteMany volume (`mig-harvester-migration-exports`) that is created in
the **VM's namespace** (and, for the UI pod itself, in `harvester-system`). Several pods can mount the same
volume at once, possibly on different nodes:

- two exports of VMs in the same namespace (`export.maxConcurrent` defaults to 2);
- an export Job and the short-lived download pod of another export;
- a cleanup Job and a download pod;
- the UI pod during an upgrade (the chart now replaces that pod instead, see below).

## The problem with Harvester's default class
`harvester-longhorn` is a **migratable** Longhorn class: its ReadWriteMany volumes are block devices meant
for VM live migration and attach to **one node at a time**. A second node asking for the same volume makes
Longhorn start a migration (a second engine), and the mount fails:

```
MountVolume.MountDevice failed ... rpc error: code = InvalidArgument desc = volume pvc-... has invalid controller count 2
```

Seen on the lab on 2026-10-04: a rolling update of the UI pod hung with the new pod stuck in
`ContainerCreating` for 30+ minutes. A volume of this kind has **no** `share-manager-pvc-...` pod.

## What to use instead
A Longhorn class that serves ReadWriteMany through a share manager (NFS), i.e. `migratable: "false"`.
Example (create it once; **validate on your cluster** by provisioning a test claim and checking that a
`share-manager-pvc-...` pod appears in `longhorn-system`):

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: harvester-longhorn-rwx
provisioner: driver.longhorn.io
allowVolumeExpansion: true
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  numberOfReplicas: "2"
  staleReplicaTimeout: "30"
  migratable: "false"
  dataLocality: "disabled"
```

then set `export.storage.storageClass: harvester-longhorn-rwx` (an existing claim cannot change class: delete
the empty export claims first, they are recreated on the next export).

## What the chart does
- With `export.enabled=true` the UI Deployment uses the **Recreate** strategy, so an upgrade replaces the pod
  (about a minute of downtime) instead of running two pods against the volume.
- The install notes warn when export is on and the class is empty (the cluster default) or
  `harvester-longhorn`. `hack/check-chart-export.sh` tests both and runs in CI.

## Unsticking an upgrade that is already hung
```
kubectl -n harvester-system scale deploy mig-harvester-migration-ui --replicas=0
kubectl -n longhorn-system get volumes.longhorn.io <pvc-...> -o jsonpath='{.status.state}{"\n"}'   # until: detached
kubectl -n harvester-system scale deploy mig-harvester-migration-ui --replicas=1
```
