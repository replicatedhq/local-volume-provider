package plugin

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/replicatedhq/local-volume-provider/pkg/k8sutil"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	kuberneteserrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const VolumeProviderKey = "app"
const VolumeProviderLabel = "velero"

type VolumeType string

const (
	Hostpath VolumeType = "hostpath"
	NFS      VolumeType = "nfs"
	PVC      VolumeType = "pvc"
)

// buildVoume creates a new k8s volume object based on the Velero BSL Config
func buildVolume(vt VolumeType, config map[string]string, log *logrus.Entry) (*corev1.Volume, error) {
	var volumeSource *corev1.VolumeSource

	var err error
	switch vt {
	case Hostpath:
		volumeSource, err = getHostPathVolumeSource(config)
	case NFS:
		volumeSource, err = getNFSVolumeSource(config)
	case PVC:
		err = ensurePVC(config, log)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pvc for %s", config["bucket"])
		}
		volumeSource, err = getPVCVolumeSource(config)
	default:
		return nil, errors.New("unrecognized volume type")
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build volume for %s", vt)
	}

	volume := &corev1.Volume{
		Name:         config["bucket"],
		VolumeSource: *volumeSource,
	}

	return volume, nil
}

// getHostPathVolumeSource returns a hostpath volume source to be used in a k8s volume
func getHostPathVolumeSource(config map[string]string) (*corev1.VolumeSource, error) {
	if _, ok := config["path"]; !ok {
		return nil, errors.New("hostpath config missing path")
	}

	volumeSource := &corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{
			Path: config["path"],
			Type: hostPathTypePtr(corev1.HostPathDirectory),
		},
	}

	return volumeSource, nil
}

// getNFSVolumeSource returns an nfs volume source to be used in a k8s volume
func getNFSVolumeSource(config map[string]string) (*corev1.VolumeSource, error) {
	path, ok := config["path"]
	if !ok {
		return nil, errors.New("nfs config missing path")
	}

	server, ok := config["server"]
	if !ok {
		return nil, errors.New("nfs config missing server address")
	}

	volumeSource := &corev1.VolumeSource{
		NFS: &corev1.NFSVolumeSource{
			Path:   path,
			Server: server,
		},
	}

	return volumeSource, nil
}

// getPVCVolumeSource returns an nfs volume source to be used in a k8s volume
func getPVCVolumeSource(config map[string]string) (*corev1.VolumeSource, error) {
	pvcName, ok := config["bucket"]
	if !ok {
		return nil, errors.New("pvc config missing pvc name")
	}
	volumeSource := &corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}

	return volumeSource, nil
}

// buildVolumeMount creates a new k8s volume mount object
func buildVolumeMount(bucket string, mountPath string) *corev1.VolumeMount {
	return &corev1.VolumeMount{Name: bucket, MountPath: mountPath, ReadOnly: false}
}

// hostPathTypePtr returns a pointer to a HostPathType constant
func hostPathTypePtr(v corev1.HostPathType) *corev1.HostPathType {
	return &v
}

// ensurePVC creates a PVC based on the config present in the backupstoragelocation CRD
func ensurePVC(config map[string]string, log *logrus.Entry) error {
	namespace := os.Getenv("VELERO_NAMESPACE")

	clientset, err := k8sutil.GetClientset()
	if err != nil {
		return errors.Wrap(err, "failed to get clientset")
	}

	pvcObj, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), config["bucket"], metav1.GetOptions{})
	if err != nil && !kuberneteserrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get velero pvc")
	}
	if err == nil {
		log.Infof("pvc already exists: %s", pvcObj.Name)
		return nil
	}

	var storageClassNamePtr *string
	_, ok := config["storageClassName"]
	if ok {
		storageClassName := config["storageClassName"]
		storageClassNamePtr = &storageClassName
	}
	persistentVolumeClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: config["bucket"],
			Labels: map[string]string{
				VolumeProviderKey: VolumeProviderLabel,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(config["storageSize"]),
				},
			},
			StorageClassName: storageClassNamePtr,
		},
	}

	_, err = clientset.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), persistentVolumeClaim, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to create velero pvc")
	}

	return nil
}
