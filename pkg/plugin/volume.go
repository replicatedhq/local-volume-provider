package plugin

import (
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
)

type VolumeType string

const (
	Hostpath VolumeType = "hostpath"
	NFS      VolumeType = "nfs"
)

// buildVoume creates a new k8s volume object based on the Velero BSL Config
func buildVolume(vt VolumeType, config map[string]string) (*corev1.Volume, error) {
	var volumeSource *corev1.VolumeSource

	var err error
	switch vt {
	case Hostpath:
		volumeSource, err = getHostPathVolumeSource(config)
	case NFS:
		volumeSource, err = getNFSVolumeSource(config)
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

// buildVolumeMount creates a new k8s volume mount object
func buildVolumeMount(bucket string, mountPath string) *corev1.VolumeMount {
	return &corev1.VolumeMount{Name: bucket, MountPath: mountPath, ReadOnly: false}
}

// hostPathTypePtr returns a pointer to a HostPathType constant
func hostPathTypePtr(v corev1.HostPathType) *corev1.HostPathType {
	return &v
}
