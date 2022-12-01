package plugin

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/replicatedhq/local-volume-provider/pkg/k8sutil"
	"github.com/replicatedhq/local-volume-provider/pkg/version"
	"github.com/sirupsen/logrus"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kuberneteserrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type localVolumeObjectStoreOpts struct {
	veleroDeploymentName      string
	resticDaemonsetName       string
	fileserverImage           string
	securityContextRunAsUser  string
	securityContextRunAsGroup string
	securityContextFSGroup    string
	preserveVolumes           map[string]bool
}

const (
	fileServerContainerName = "local-volume-provider"

	defaultVeleroDeploymentName = "velero"
	defaultResticDaemonsetName  = "restic"

	signingSecretName = "lvp-signingsecret"
)

var (
	defaultFileServerContainerImage = fmt.Sprintf("replicated/local-volume-provider:%s", version.Get())
)

type EnsureResourcesOpts struct {
	clientset  kubernetes.Interface
	namespace  string
	bucket     string
	prefix     string
	path       string
	config     map[string]string
	pluginOpts *localVolumeObjectStoreOpts
	volumeType VolumeType
	log        *logrus.Entry
}

// ensureResources ensures that the resources needed for the plugin are present
// and will update them if they are not.
func ensureResources(opts EnsureResourcesOpts) error {
	ds, err := getDaemonset(opts.clientset, opts.namespace, opts.pluginOpts)
	if err != nil {
		return errors.Wrap(err, "could not get restic daemonset")
	}

	deployment, err := getDeployment(opts.clientset, opts.namespace, opts.pluginOpts)
	if err != nil {
		return errors.Wrap(err, "could not get Velero deployment")
	}

	// if `preserveVolumes` is specified, clean up all other volumes and volume mounts
	if len(opts.pluginOpts.preserveVolumes) > 0 {
		if !opts.pluginOpts.preserveVolumes[opts.bucket] {
			// BackupStorageLocation exists, but the bucket is not in `preserveVolumes`, do not update the resources
			opts.log.Warnf("`preserveVolumes` was specified, but %s was not included. The volume will not be created/mounted.", opts.bucket)
			return nil
		}

		if ds != nil {
			ds.Spec.Template.Spec.Volumes = removeUnusedVolumes(ds.Spec.Template.Spec.Volumes, opts.pluginOpts.preserveVolumes)
			ds.Spec.Template.Spec.Containers[0].VolumeMounts = removeUnusedVolumeMounts(ds.Spec.Template.Spec.Containers[0].VolumeMounts, opts.pluginOpts.preserveVolumes)
		}

		deployment.Spec.Template.Spec.Volumes = removeUnusedVolumes(deployment.Spec.Template.Spec.Volumes, opts.pluginOpts.preserveVolumes)
		// remove unused mounts from all containers in the deployment
		for idx := range deployment.Spec.Template.Spec.Containers {
			container := &deployment.Spec.Template.Spec.Containers[idx]
			container.VolumeMounts = removeUnusedVolumeMounts(container.VolumeMounts, opts.pluginOpts.preserveVolumes)
		}
	}

	volumeMountSpec := buildVolumeMount(opts.bucket, opts.path)

	volumeSpec, err := buildVolume(opts.volumeType, opts.config, opts.log)
	if err != nil {
		return errors.Wrap(err, "failed to build volume")
	}

	if ds != nil {
		// If restic is present, it must also mount the volume
		err = ensureDaemonsetHasVolume(ds, volumeSpec, volumeMountSpec)
		if err != nil {
			return errors.Wrap(err, "failed to ensure restic daemonset has volume")
		}

		err = ensureDaemonsetHasConfig(ds, opts.pluginOpts)
		if err != nil {
			return errors.Wrap(err, "failed to ensure restic daemonset has plugin configuration")
		}

		// Update the restic daemonset
		_, err = opts.clientset.AppsV1().DaemonSets(opts.namespace).Update(context.TODO(), ds, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "unable to update restic daemonset")
		}
	}

	err = ensureDeploymentHasVolume(deployment, volumeSpec, volumeMountSpec)
	if err != nil {
		return errors.Wrap(err, "failed to ensure velero deployment has volume")
	}

	// Always update the deployment for new configmap setting and the fileserver,
	// even if the local volume is already mounted.
	err = ensureDeploymentHasConfigAndFileserver(deployment, volumeMountSpec, opts.pluginOpts)
	if err != nil {
		return errors.Wrap(err, "could not ensure plugin configuration")
	}

	// Update Velero deployment
	_, err = opts.clientset.AppsV1().Deployments(opts.namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "unable to update velero deployment")
	}

	return nil
}

// getDeployment returns the deployment for velero. It will return an error if it can not be found.
func getDeployment(clientset kubernetes.Interface, namespace string, opts *localVolumeObjectStoreOpts) (*appsv1.Deployment, error) {
	name := defaultVeleroDeploymentName
	if opts.veleroDeploymentName != "" {
		name = opts.veleroDeploymentName
	}

	existingDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if kuberneteserrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "velero deployment not found")
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to get velero deployment")
	}

	return existingDeployment, nil
}

// ensureDeploymentHasVolume check the velero deployment for a matching Volume name
// and if it does not exist, adds it to the podspec.
func ensureDeploymentHasVolume(deployment *appsv1.Deployment, volumeSpec *corev1.Volume, volumeMountSpec *corev1.VolumeMount) error {

	// If the volume name is the same, but the path is different, we should fix the path in place
	if exists, idx := podHasDuplicateVolumeName(&deployment.Spec.Template.Spec, volumeSpec); exists {
		deployment.Spec.Template.Spec.Volumes[idx] = *volumeSpec
	} else {
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, *volumeSpec)

		// TODO (dans): user configuration for velero container name
		veleroContainer := getContainerByName(deployment, "velero")
		if veleroContainer == nil {
			return errors.New("velero container not found")
		}
		veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts, *volumeMountSpec)

		// Add the POD_IP for servering the signed URLs
		if !containerHasEnvVar(veleroContainer, "POD_IP") {
			veleroContainer.Env = append(veleroContainer.Env, corev1.EnvVar{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			})
		}
	}

	return nil
}

// getDaemonset returns the daemonset for restic. It will return nil if it cannot be found,
// as restic is an optional component
func getDaemonset(clientset kubernetes.Interface, namespace string, opts *localVolumeObjectStoreOpts) (*appsv1.DaemonSet, error) {
	name := defaultResticDaemonsetName
	if opts.resticDaemonsetName != "" {
		name = opts.resticDaemonsetName
	}

	existingDaemonset, err := clientset.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if kuberneteserrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to check for restic daemonset")
	}

	return existingDaemonset, nil
}

// ensureDaemonsetHasVolume checks the restic daemonset for a matching Volume name. If it does not find it,
// it adds it to the podspec and updates the daemonset.
func ensureDaemonsetHasVolume(ds *appsv1.DaemonSet, volumeSpec *corev1.Volume, volumeMountSpec *corev1.VolumeMount) error {

	// If the volume name is the same, but the path is different, we should fix the path in place
	if exists, idx := podHasDuplicateVolumeName(&ds.Spec.Template.Spec, volumeSpec); exists {
		ds.Spec.Template.Spec.Volumes[idx] = *volumeSpec
	} else {
		ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, *volumeSpec)
		ds.Spec.Template.Spec.Containers[0].VolumeMounts = append(ds.Spec.Template.Spec.Containers[0].VolumeMounts, *volumeMountSpec)
	}

	return nil
}

// ensureDaemonsetHasConfig will update the restic daemonset as-needed based on config options.
func ensureDaemonsetHasConfig(ds *appsv1.DaemonSet, opts *localVolumeObjectStoreOpts) error {
	podSecurityCxt, err := getPodSecurityContext(opts)
	if err != nil {
		return errors.Wrap(err, "unable to get security context")
	}
	if podSecurityCxt != nil {
		ds.Spec.Template.Spec.SecurityContext = podSecurityCxt
	}
	return nil
}

// removeUnusedVolumes removes volumes that are not specified in preserveVolumes
func removeUnusedVolumes(volumes []corev1.Volume, preserveVolumes map[string]bool) []corev1.Volume {
	var newVolumes []corev1.Volume
	for _, volume := range volumes {
		// always preserve 'plugins', 'host-pods', 'scratch', and 'cloud-credentials' as these are used by velero and restic
		if volume.Name == "plugins" || volume.Name == "host-pods" || volume.Name == "scratch" || volume.Name == "cloud-credentials" {
			newVolumes = append(newVolumes, volume)
			continue
		}
		if preserveVolumes[volume.Name] {
			newVolumes = append(newVolumes, volume)
		}
	}
	return newVolumes
}

// removeUnusedVolumeMounts removes volume mounts that are not specified in preserveVolumes
func removeUnusedVolumeMounts(volumeMounts []corev1.VolumeMount, preserveVolumes map[string]bool) []corev1.VolumeMount {
	var newVolumeMounts []corev1.VolumeMount
	for _, volumeMount := range volumeMounts {
		// always preserve 'plugins', 'host-pods', 'scratch', and 'cloud-credentials' as these are used by velero and restic
		if volumeMount.Name == "plugins" || volumeMount.Name == "host-pods" || volumeMount.Name == "scratch" || volumeMount.Name == "cloud-credentials" {
			newVolumeMounts = append(newVolumeMounts, volumeMount)
			continue
		}
		if preserveVolumes[volumeMount.Name] {
			newVolumeMounts = append(newVolumeMounts, volumeMount)
		}
	}
	return newVolumeMounts
}

// getContainerByName returns a pointer to the container with the given name in a deployment,
// or nil if it cannot be found.
func getContainerByName(deployment *appsv1.Deployment, name string) *corev1.Container {
	for idx, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == name {
			// need to get ptr, not the copy from range
			return &deployment.Spec.Template.Spec.Containers[idx]
		}
	}
	return nil
}

// podHasDuplicateVolumeName returns true if the pod has a volume with the given name, and the index of the
// the container.
func podHasDuplicateVolumeName(ps *corev1.PodSpec, volume *corev1.Volume) (bool, int) {
	for i, v := range ps.Volumes {
		if v.Name == volume.Name {
			return true, i
		}
	}
	return false, -1
}

// containerHasEnvVar returns true if the container has an env var with the given name.
func containerHasEnvVar(container *corev1.Container, name string) bool {
	for _, env := range container.Env {
		if env.Name == name {
			return true
		}
	}
	return false
}

// getPluginConfigMap return the config map for the plugin volume time based on velero label conventions.
// It returns nil if it cannot be found.
func getPluginConfigMap(kind VolumeType) (*corev1.ConfigMap, error) {
	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("replicated.com/%s=%s", string(kind), veleroplugin.PluginKindObjectStore),
	}

	clientset, err := k8sutil.GetClientset()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get kubernetes clientset")
	}

	list, err := clientset.CoreV1().ConfigMaps(os.Getenv("VELERO_NAMESPACE")).List(context.TODO(), listOpts)
	if err != nil {
		return nil, errors.Wrap(err, "could not list config maps")
	}

	if len(list.Items) == 0 {
		return nil, nil
	}

	if len(list.Items) > 1 {
		var items []string
		for _, item := range list.Items {
			items = append(items, item.Name)
		}
		return nil, errors.New(fmt.Sprintf("found more than one ConfigMap matching label selector %q: %v", listOpts.LabelSelector, items))
	}

	return &list.Items[0], nil
}

// getSigningKey returns a byte slice of the a signing key located in a given namespace. If the key cannot be found,
// it will generate a secret in the provided namespace.
func getSigningKey(namespace string) ([]byte, error) {
	clientset, err := k8sutil.GetClientset()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubernetes clientset")
	}

	secrets := clientset.CoreV1().Secrets(namespace)

	signingSecret, err := secrets.Get(context.Background(), signingSecretName, metav1.GetOptions{})
	if err != nil {
		// generate new signing secret if one isn't found
		signingSecret, err = createSigningSecret(namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to collect or generate signing key: %v", err)
		}
	}

	return signingSecret.Data["SigningKey"], nil
}

// createSigningSecret creates a new signing key secret in the given namespace.
func createSigningSecret(namespace string) (*corev1.Secret, error) {
	if namespace == "" {
		namespace = os.Getenv("VELERO_NAMESPACE")
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      signingSecretName,
			Namespace: namespace,
		},
		Type: "Opaque",
		Data: make(map[string][]byte),
	}

	secretKey := make([]byte, 16)
	rand.Seed(time.Now().UnixNano())
	rand.Read(secretKey)

	secret.Data["SigningKey"] = secretKey

	clientset, err := k8sutil.GetClientset()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubernetes clientset")
	}

	secrets := clientset.CoreV1().Secrets(namespace)

	secret, err = secrets.Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create signing secret")
	}

	return secret, nil
}

// getPodSecurityContext returns a pod security context object based on the plugin configuration provided in the options.
func getPodSecurityContext(opts *localVolumeObjectStoreOpts) (*corev1.PodSecurityContext, error) {
	var securityCxt *corev1.PodSecurityContext
	// If pod security context was provided, ensure that it is added to the deployment
	if opts.securityContextRunAsUser != "" || opts.securityContextRunAsGroup != "" || opts.securityContextFSGroup != "" {
		securityCxt = &corev1.PodSecurityContext{}

		if opts.securityContextRunAsUser != "" {
			runAsUser, err := StringToIntPointer(opts.securityContextRunAsUser)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse security context 'runAsUser' into integer")
			}
			securityCxt.RunAsUser = runAsUser
		}

		if opts.securityContextRunAsGroup != "" {
			runAsGroup, err := StringToIntPointer(opts.securityContextRunAsGroup)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse security context 'runAsGroup' into integer")
			}
			securityCxt.RunAsGroup = runAsGroup
		}

		if opts.securityContextFSGroup != "" {
			fsGroup, err := StringToIntPointer(opts.securityContextFSGroup)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse security context 'fsGroup' into integer")
			}
			securityCxt.FSGroup = fsGroup
		}
	}
	return securityCxt, nil
}

func containerHasVolumeMount(container *corev1.Container, name string) bool {
	for _, volumeMount := range container.VolumeMounts {
		if volumeMount.Name == name {
			return true
		}
	}
	return false
}

// ensureDeploymentHasConfig will update the velero deployment security context as-needed based on config options.
func ensureDeploymentHasConfigAndFileserver(deployment *appsv1.Deployment, volumeMountSpec *corev1.VolumeMount, opts *localVolumeObjectStoreOpts) error {

	// Security Context
	podSecurityCxt, err := getPodSecurityContext(opts)
	if err != nil {
		return errors.Wrap(err, "unable to get security context")
	}
	if podSecurityCxt != nil {
		deployment.Spec.Template.Spec.SecurityContext = podSecurityCxt
	}

	// Fileserver
	// TODO (dans): make sure that the MOUNT_POINT env exists, even if the container is already there.
	fileServerContainer := getContainerByName(deployment, fileServerContainerName)

	fileServerImage := defaultFileServerContainerImage
	if opts.fileserverImage != "" {
		fileServerImage = opts.fileserverImage
	}

	// If the sidecar already exists and a volume mount with the same name, nothing to change
	if fileServerContainer != nil && containerHasVolumeMount(fileServerContainer, volumeMountSpec.Name) {
		return nil
	}

	if fileServerContainer == nil {
		fileServerContainer = &corev1.Container{
			Name:    fileServerContainerName,
			Image:   fileServerImage,
			Command: []string{"/local-volume-fileserver"},
			Env: []corev1.EnvVar{
				{
					Name:  "MOUNT_POINT",
					Value: getRoot(),
				},
				{
					Name: "VELERO_NAMESPACE",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.namespace",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{*volumeMountSpec},
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, *fileServerContainer)
	} else {
		fileServerContainer.VolumeMounts = append(fileServerContainer.VolumeMounts, *volumeMountSpec)
	}

	return nil
}
