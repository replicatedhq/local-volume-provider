package plugin

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/replicatedhq/local-volume-provider/pkg/k8sutil"
	"github.com/sirupsen/logrus"
)

// Velero doesn't allow other non-velero directories in the root of the object store.
// These directories are filter out of the reported list.
var directoryDenyList = []string{"lost+found"}

type LocalVolumeObjectStore struct {
	log        logrus.FieldLogger
	volumeType VolumeType
	opts       *localVolumeObjectStoreOpts
}

// NewLocalVolumeObjectStore instantiates a LocalVolumeObjectStore with a particular target volume type.
func NewLocalVolumeObjectStore(log logrus.FieldLogger, v VolumeType) *LocalVolumeObjectStore {
	return &LocalVolumeObjectStore{
		log:        log,
		volumeType: v,
	}
}

// Init initializes the plugin. It can be called multiple times.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) Init(config map[string]string) error {
	bucket := config["bucket"]
	prefix := config["prefix"]
	path := filepath.Join(getRoot(), bucket)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"path":   path,
		"prefix": prefix,
	})
	log.Debug("LocalVolumeObjectStore.Init called")

	if err := o.getLocalVolumeStoreOpts(); err != nil {
		return errors.Wrap(err, "failed to get local volume configuration")
	}

	if err := ensureFilesystem(path, prefix, log); err != nil {
		return errors.Wrap(err, "failed to ensure filesystem")
	}

	clientset, err := k8sutil.GetClientset()
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes clientset")
	}

	ensureResourcesOpts := EnsureResourcesOpts{
		clientset:  clientset,
		namespace:  os.Getenv("VELERO_NAMESPACE"),
		bucket:     bucket,
		prefix:     prefix,
		path:       path,
		config:     config,
		pluginOpts: o.opts,
		volumeType: o.volumeType,
		log:        log,
	}

	if err := ensureResources(ensureResourcesOpts); err != nil {
		return errors.Wrap(err, "failed to ensure resources")
	}

	return nil
}

// PutObject puts an object into the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) PutObject(bucket string, key string, body io.Reader) error {
	path := filepath.Join(getRoot(), bucket, key)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
		"path":   path,
	})
	log.Debug("LocalVolumeObjectStore.PutObject called")

	dir := filepath.Dir(path)
	log.Debugf("Creating dir %s", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	log.Debug("Creating file")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	log.Debug("Writing to file")
	_, err = io.Copy(file, body)

	log.Debug("Done")
	return err
}

// ObjectExists returns truthy if an object is in the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) ObjectExists(bucket, key string) (bool, error) {
	path := filepath.Join(getRoot(), bucket, key)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
		"path":   path,
	})
	log.Debug("LocalVolumeObjectStore.ObjectExists called")

	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return true, err
}

// GetObject returns truthy if an object is in the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	path := filepath.Join(getRoot(), bucket, key)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
		"path":   path,
	})
	log.Debug("LocalVolumeObjectStore.GetObject called")

	return os.Open(path)
}

// ListCommonPrefixes returns a list of subdirectories in the root of the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) ListCommonPrefixes(bucket, prefix, delimiter string) ([]string, error) {
	path := filepath.Join(getRoot(), bucket, prefix, delimiter)

	log := o.log.WithFields(logrus.Fields{
		"bucket":    bucket,
		"delimiter": delimiter,
		"path":      path,
		"prefix":    prefix,
	})
	log.Debug("LocalVolumeObjectStore.ListCommonPrefixes called")

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() && !sliceContainsString(directoryDenyList, dirEntry.Name()) {
			dirs = append(dirs, dirEntry.Name())
		}
	}

	return dirs, nil
}

// ListObjects returns a list of files in the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	path := filepath.Join(getRoot(), bucket, prefix)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"prefix": prefix,
		"path":   path,
	})
	log.Debug("LocalVolumeObjectStore.ListObjects called")

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var objects []string
	for _, dirEntry := range dirEntries {
		objects = append(objects, filepath.Join(prefix, dirEntry.Name()))
	}

	return objects, nil
}

// DeleteObject removes a files from the LocalVolumeObjectStore.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) DeleteObject(bucket, key string) error {
	path := filepath.Join(getRoot(), bucket, key)

	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
		"path":   path,
	})
	log.Debug("LocalVolumeObjectStore.DeleteObject called")

	err := os.Remove(path)

	// This logic is specific to a file system; we need to clean up the backup directory
	// if there's nothing left. "Normal" object stores only mimic directory structures and don't need this.
	keyParts := strings.Split(key, "/")
	var backupPath string
	if len(keyParts) > 1 {
		backupPath = filepath.Join(getRoot(), bucket, keyParts[0], keyParts[1])
	}
	if backupPath != "" {
		infos, err := ioutil.ReadDir(backupPath)
		if err != nil {
			return err
		}
		if len(infos) == 0 {
			l := o.log.WithFields(logrus.Fields{
				"backupPath": backupPath,
			})
			l.Debug("Deleted backup directory")
			os.Remove(backupPath)
		}
	}

	return err
}

// CreateSignedURL creates a signed URL to the pod ID for anonymous external access to LocalVolumeObjectStore files.
// It is part of the Velero plugin interface.
func (o *LocalVolumeObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Debug("LocalVolumeObjectStore.CreateSignedURL called")

	namespace := os.Getenv("VELERO_NAMESPACE")

	signedUrl := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", os.Getenv("POD_IP"), 3000),
		Path:   fmt.Sprintf("/%s/%s", bucket, key),
	}

	err := SignURL(&signedUrl, namespace, ttl)
	if err != nil {
		return "", errors.Wrap(err, "failed to create signed url")
	}

	return signedUrl.String(), nil
}

// getLocalVolumeStoreOpts looks for the optional plugin config map and then uses it
// to populate options for the rest of the plugin calls.
func (o *LocalVolumeObjectStore) getLocalVolumeStoreOpts() error {
	pluginConfigMap, err := getPluginConfigMap(o.volumeType)
	if err != nil {
		return errors.Wrap(err, "failed to get plugin config map")
	}
	if pluginConfigMap == nil {
		o.log.Debug("Did not find a configmap fot this plugin")
		o.opts = &localVolumeObjectStoreOpts{}
	} else {
		o.log.Debug("Found a configmap for this plugin")

		preserveVolumes := make(map[string]bool)
		if pluginConfigMap.Data["preserveVolumes"] != "" {
			preserveVolumesList := strings.Split(pluginConfigMap.Data["preserveVolumes"], ",")
			for _, volume := range preserveVolumesList {
				preserveVolumes[volume] = true
			}
		}

		o.opts = &localVolumeObjectStoreOpts{
			veleroDeploymentName:      pluginConfigMap.Data["veleroDeploymentName"],
			resticDaemonsetName:       pluginConfigMap.Data["resticDaemonsetName"],
			fileserverImage:           pluginConfigMap.Data["fileserverImage"],
			securityContextRunAsUser:  pluginConfigMap.Data["securityContextRunAsUser"],
			securityContextRunAsGroup: pluginConfigMap.Data["securityContextRunAsGroup"],
			securityContextFSGroup:    pluginConfigMap.Data["securityContextFsGroup"],
			preserveVolumes:           preserveVolumes,
		}
	}
	return nil
}
