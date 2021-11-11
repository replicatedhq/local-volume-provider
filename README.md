# local-volume-provider

`local-volume-provider` is a Velero plugin to enable storage directly to native Kubernetes' volume types instead of using Object or Blob storage APIs. 
It also supports volume snapshots with Restic. It is designed to service small and air-gapped clusters that may not have access directly to Object Storage APIs like S3.

The plugin leverages the existing Velero service account credentials to mount volumes directly to the velero/restic pods. 
This plugin is also heavily based off of [Velero's example plugin](https://github.com/vmware-tanzu/velero-plugin-example).

⚠️ **Cautions**
1. Hostpath volumes are not designed to work on multi-node clusters unless the underlying host mounts point to shared storage. 
Volume snapshots performed in this configuration without shared storage can result in fragmented backups.
1. Customized deployments of Velero (RBAC, container names), may not be supported.
1. When BackupStorageLocations are removed, they are NOT cleaned up from the Velero and Restic pods.
1. This plugin relies on an additional sidecar container, `local-volume-fileserver`, to provide signed-url access to storage data.

## Deploying the plugin

To deploy the plugin image to a Velero server:

1. Make sure Velero is installed, optionally with Restic if Volume Snapshots are needed.
1. (For NFS or HostPath volumes) Prepare the volume target.
    1. The NFS share or host directory must already exist prior to creating the BackupStorageLocation
    1. The directory must have write permissions that are either writable by the Velero container by default, which runs as non-root, or to the same Uid/Gid as the plugin configuration. 
    See the Customization section below for how to configuration these settings.
1. Make sure the plugin images are pushed to a registry that is accessible to your cluster's nodes.
There are two images required for the plugin:
    1. replicated/local-volume-provider:v0.1.0
    1. replicated/local-volume-fileserver:v0.1.0
2. Run `velero plugin add replicated/local-volume-provider:v0.1.0`.
This will re-deploy Velero with the plugin installed.
3. Create a BackupStorageLocation according to the schemas below.
The plugin will attach the volume to Velero (and Restic if available)
It will also add a fileserver sidecar to the Velero pod if not already present. 
This is used to server assets like backup logs directly to consumers of the Velero api (e.g. the Velero CLI uses these logs to print backup status info)

### Customization

You can configure certain aspects of plugin behavior by customizing the following ConfigMap spec and adding to the Velero namespace. 
It is based on the [Velero Plugin Configuration scheme](https://velero.io/docs/v1.6/custom-plugins/).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-volume-provider-config
  namespace: velero
  labels:
    velero.io/plugin-config: ""
    replicated.com/nfs: ObjectStore
    replicated.com/hostpath: ObjectStore
data:
  # Customize these values
  veleroDeploymentName: velero
  resticDaemonsetName: restic
  # Useful for local development
  fileserverImage: ttl.sh/<your user>/local-volume-fileserver:2h
  # Helps to lock down file permissions to known users/groups on the target volume
  securityContextRunAsUser: "1001"
  securityContextRunAsGroup: "1001"
  securityContextFsGroup: "1001"
```

## Removing the plugin

The plugin can be removed with `velero plugin remove replicated/local-volume-provider:latest`.
This does not detach/delete any volumes that were used during operation.
These can be removed manually using `kubectl edit` or by re-deploying velero (`velero uninstall` and `velero install ...`)

## Usage Examples

### HostPath

```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: default
  namespace: velero
spec:
  backupSyncPeriod: 2m0s
  provider: replicated.com/hostpath
  objectStorage:
    # This corresponds to a unique volume name
    bucket: hostPath-snapshots
  config:
    # This path must exist on the host and be writable outside the group
    path: /tmp/snapshots
    # Must be provided if you're using Restic; [default mount] + [bucket] + [prefix] + "restic"
    resticRepoPrefix: /var/velero-local-volume-provider/hostpath-snapshots/restic
```

### NFS

```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: default
  namespace: velero
spec:
  backupSyncPeriod: 2m0s
  provider: replicated.com/nfs
  objectStorage:
    # This corresponds to a unique volume name
    bucket: nfs-snapshots
  config:
    # Path and server on share
    path: /tmp/nfs-snapshots
    server: 1.2.3.4
    # Must be provided if you're using Restic; [default mount] + [bucket] + [prefix] + "restic"
    resticRepoPrefix: /var/velero-local-volume-provider/nfs-snapshots/restic
```


## Building & Testing the Plugin

To build the plugin and fileserver, run

```bash
$ make plugin
$ make fileserver
```

To build the image, run

```bash
$ make container-plugin
$ make container-fileserver
```

This builds an image tagged as `replicated/local-volume-provider:latest`. If you want to specify a different name or version/tag, run:

```bash
$ IMAGE=your-repo/your-name VERSION=your-version-tag make container 
```

To build a temporary image for testing, run

```bash
$ make ttl.sh
```

This builds images tagged as `ttl.sh/<unix user>/local-volume-provider:2h` and `ttl.sh/<unix user>/local-volume-fileserver:2h`.

Make sure the plugin will be configured to use the correct security context and development images by applying the optional [ConfigMap](https://raw.githubusercontent.com/replicatedhq/local-volume-provider/main/examples/pluginConfigMap.yaml) (edit this configmap first with your username):

### Velero Install Option 1

1. To install Velero 1.6+ without the plugin installed (useful for testing the `velero` install/remove plugin commands). You need at least one plugin by default:
```bash
velero install --use-restic --use-volume-snapshots=false --namespace velero --plugins velero/velero-plugin-for-aws:v1.2.0 --no-default-backup-location --no-secret
```
1. Add the plugin
```bash
velero plugin add ttl.sh/<user>/local-volume-provider:2h
```
1. Create the default BackupStorageLocation (assuming Hostpath here)
```bash
velero backup-location create default --default --bucket my-hostpath-snaps --provider replicated.com/hostpath --config path=/tmp/my-host-path-to-snaps,resticRepoPrefix=/var/velero-local-volume-provider/my-hostpath-snaps/restic
```

### Install Option 2

To install Velero with the plugin configured to Hostpath by default:
```bash
velero install --use-restic --use-volume-snapshots=false --namespace velero --provider replicated.com/hostpath --plugins ttl.sh/<username>/local-volume-provider:2h --bucket my-hostpath-snaps --backup-location-config path=/tmp/my-host-path-to-snaps,resticRepoPrefix=/var/velero-local-volume-provider/my-hostpath-snaps/restic --no-secret 
```

### Install Option 3

To update an BackupStorageLocation (BSL) in an existing cluster with Velero, you must first delete the BSL and re-create as follows (assuming you are using the BSL created by default):
```bash
velero plugin add ttl.sh/<user>/local-volume-provider:2h
velero backup-location delete default  #Hit "Y" to confirm 
velero backup-location create default --default --bucket my-hostpath-snaps --provider replicated.com/hostpath --config path=/tmp/my-host-path-to-snaps,resticRepoPrefix=/var/velero-local-volume-provider/my-hostpath-snaps/restic
```

# Troubleshooting 

1. The Velero pod is stuck initializing: 
    1. Verify the volume exists on the host. Create if it doesn't and delete the Velero pod.
1. [HostPath Only] The Velero pod is running, but the backupstorage location is unavailable.
    1. Verify the path on the host is writable by the Velero pod. The Velero pod runs as user `nobody`.
1. Backups are partially failing and you're using Restic.
    1. Make sure you have defined `resticRepoPrefix` in you BackupStorageLocation Config. It should point to the `restic` directory mountpoint in the Velero container
    1. Delete your Restic Repo CR `k -n velero delete resticrepositories.velero.io default-default-<ID>` to have this regenerated.

# Future
- [ ] TESTING!
