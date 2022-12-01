package plugin

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
)

// test ensureResources function
func Test_ensureResources(t *testing.T) {
	tests := []struct {
		name           string
		opts           EnsureResourcesOpts
		wantDeployment *appsv1.Deployment
		wantDaemonSet  *appsv1.DaemonSet
	}{
		{
			name: "new configuration -- local-volume-provider container is added to the deployment along with the volume",
			opts: EnsureResourcesOpts{
				clientset: fake.NewSimpleClientset(&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "velero",
						Namespace: "velero",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "velero",
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "plugins",
												MountPath: "/plugins",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "plugins",
										VolumeSource: corev1.VolumeSource{
											EmptyDir: &corev1.EmptyDirVolumeSource{},
										},
									},
								},
							},
						},
					},
				}),
				namespace: "velero",
				bucket:    "my-bucket",
				prefix:    "",
				path:      "/var/velero-local-volume-provider/my-bucket",
				config: map[string]string{
					"bucket": "my-bucket",
					"prefix": "",
					"path":   "/backups",
				},
				pluginOpts: &localVolumeObjectStoreOpts{},
				volumeType: Hostpath,
				log:        logrus.NewEntry(logrus.New()),
			},
			wantDeployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: "velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "velero",
									Env:  getVeleroContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
										},
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
								{
									Name:    "local-volume-provider",
									Image:   "replicated/local-volume-provider:main",
									Command: []string{"/local-volume-fileserver"},
									Env:     getLVPContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "my-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "updated configuration -- volume is added to the deployment, but old volume is not removed because `preserveVolumes` is not set",
			opts: EnsureResourcesOpts{
				clientset: fake.NewSimpleClientset(&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "velero",
						Namespace: "velero",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "velero",
										Env:  getVeleroContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "plugins",
												MountPath: "/plugins",
											},
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
									{
										Name:    "local-volume-provider",
										Image:   "replicated/local-volume-provider:main",
										Command: []string{"/local-volume-fileserver"},
										Env:     getLVPContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "plugins",
										VolumeSource: corev1.VolumeSource{
											EmptyDir: &corev1.EmptyDirVolumeSource{},
										},
									},
									{
										Name: "my-bucket",
										VolumeSource: corev1.VolumeSource{
											HostPath: &corev1.HostPathVolumeSource{
												Path: "/backups",
												Type: hostPathTypePtr(corev1.HostPathDirectory),
											},
										},
									},
								},
							},
						},
					},
				}),
				namespace: "velero",
				bucket:    "my-new-bucket",
				prefix:    "",
				path:      "/var/velero-local-volume-provider/my-new-bucket",
				config: map[string]string{
					"bucket": "my-new-bucket",
					"prefix": "",
					"path":   "/new-backups",
				},
				pluginOpts: &localVolumeObjectStoreOpts{},
				volumeType: Hostpath,
				log:        logrus.NewEntry(logrus.New()),
			},
			wantDeployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: "velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "velero",
									Env:  getVeleroContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
										},
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
										{
											Name:      "my-new-bucket",
											MountPath: "/var/velero-local-volume-provider/my-new-bucket",
										},
									},
								},
								{
									Name:    "local-volume-provider",
									Image:   "replicated/local-volume-provider:main",
									Command: []string{"/local-volume-fileserver"},
									Env:     getLVPContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
										{
											Name:      "my-new-bucket",
											MountPath: "/var/velero-local-volume-provider/my-new-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "my-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
								{
									Name: "my-new-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/new-backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "updated configuration -- volume is added to the deployment, and the old volume is removed because `preserveVolumes` is set",
			opts: EnsureResourcesOpts{
				clientset: fake.NewSimpleClientset(&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "velero",
						Namespace: "velero",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "velero",
										Env:  getVeleroContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "plugins",
												MountPath: "/plugins",
											},
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
									{
										Name:    "local-volume-provider",
										Image:   "replicated/local-volume-provider:main",
										Command: []string{"/local-volume-fileserver"},
										Env:     getLVPContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "plugins",
										VolumeSource: corev1.VolumeSource{
											EmptyDir: &corev1.EmptyDirVolumeSource{},
										},
									},
									{
										Name: "my-bucket",
										VolumeSource: corev1.VolumeSource{
											HostPath: &corev1.HostPathVolumeSource{
												Path: "/backups",
												Type: hostPathTypePtr(corev1.HostPathDirectory),
											},
										},
									},
								},
							},
						},
					},
				}),
				namespace: "velero",
				bucket:    "my-new-bucket",
				prefix:    "",
				path:      "/var/velero-local-volume-provider/my-new-bucket",
				config: map[string]string{
					"bucket": "my-new-bucket",
					"prefix": "",
					"path":   "/new-backups",
				},
				pluginOpts: &localVolumeObjectStoreOpts{
					preserveVolumes: map[string]bool{
						"my-new-bucket": true,
					},
				},
				volumeType: Hostpath,
				log:        logrus.NewEntry(logrus.New()),
			},
			wantDeployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: "velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "velero",
									Env:  getVeleroContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
										},
										{
											Name:      "my-new-bucket",
											MountPath: "/var/velero-local-volume-provider/my-new-bucket",
										},
									},
								},
								{
									Name:    "local-volume-provider",
									Image:   "replicated/local-volume-provider:main",
									Command: []string{"/local-volume-fileserver"},
									Env:     getLVPContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-new-bucket",
											MountPath: "/var/velero-local-volume-provider/my-new-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "my-new-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/new-backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "updated configuration -- new volume is not added because `preserveVolumes` is set, but the new is not in the list",
			opts: EnsureResourcesOpts{
				clientset: fake.NewSimpleClientset(&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "velero",
						Namespace: "velero",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "velero",
										Env:  getVeleroContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "plugins",
												MountPath: "/plugins",
											},
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
									{
										Name:    "local-volume-provider",
										Image:   "replicated/local-volume-provider:main",
										Command: []string{"/local-volume-fileserver"},
										Env:     getLVPContainerEnv(),
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "my-bucket",
												MountPath: "/var/velero-local-volume-provider/my-bucket",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "plugins",
										VolumeSource: corev1.VolumeSource{
											EmptyDir: &corev1.EmptyDirVolumeSource{},
										},
									},
									{
										Name: "my-bucket",
										VolumeSource: corev1.VolumeSource{
											HostPath: &corev1.HostPathVolumeSource{
												Path: "/backups",
												Type: hostPathTypePtr(corev1.HostPathDirectory),
											},
										},
									},
								},
							},
						},
					},
				}),
				namespace: "velero",
				bucket:    "my-new-bucket",
				prefix:    "",
				path:      "/var/velero-local-volume-provider/my-new-bucket",
				config: map[string]string{
					"bucket": "my-new-bucket",
					"prefix": "",
					"path":   "/new-backups",
				},
				pluginOpts: &localVolumeObjectStoreOpts{
					preserveVolumes: map[string]bool{
						"my-bucket": true,
					},
				},
				volumeType: Hostpath,
				log:        logrus.NewEntry(logrus.New()),
			},
			wantDeployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: "velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "velero",
									Env:  getVeleroContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
										},
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
								{
									Name:    "local-volume-provider",
									Image:   "replicated/local-volume-provider:main",
									Command: []string{"/local-volume-fileserver"},
									Env:     getLVPContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "my-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "updated configuration -- pod security context is set on the deployment and daemonset",
			opts: EnsureResourcesOpts{
				clientset: fake.NewSimpleClientset(
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Deployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "velero",
							Namespace: "velero",
						},
						Spec: appsv1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: "velero",
											Env:  getVeleroContainerEnv(),
											VolumeMounts: []corev1.VolumeMount{
												{
													Name:      "plugins",
													MountPath: "/plugins",
												},
												{
													Name:      "my-bucket",
													MountPath: "/var/velero-local-volume-provider/my-bucket",
												},
											},
										},
										{
											Name:    "local-volume-provider",
											Image:   "replicated/local-volume-provider:main",
											Command: []string{"/local-volume-fileserver"},
											Env:     getLVPContainerEnv(),
											VolumeMounts: []corev1.VolumeMount{
												{
													Name:      "my-bucket",
													MountPath: "/var/velero-local-volume-provider/my-bucket",
												},
											},
										},
									},
									Volumes: []corev1.Volume{
										{
											Name: "plugins",
											VolumeSource: corev1.VolumeSource{
												EmptyDir: &corev1.EmptyDirVolumeSource{},
											},
										},
										{
											Name: "my-bucket",
											VolumeSource: corev1.VolumeSource{
												HostPath: &corev1.HostPathVolumeSource{
													Path: "/backups",
													Type: hostPathTypePtr(corev1.HostPathDirectory),
												},
											},
										},
									},
								},
							},
						},
					},
					&appsv1.DaemonSet{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "DaemonSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "restic",
							Namespace: "velero",
						},
						Spec: appsv1.DaemonSetSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: "restic",
											VolumeMounts: []corev1.VolumeMount{
												{
													Name:      "my-bucket",
													MountPath: "/var/velero-local-volume-provider/my-bucket",
												},
											},
										},
									},
									Volumes: []corev1.Volume{
										{
											Name: "my-bucket",
											VolumeSource: corev1.VolumeSource{
												HostPath: &corev1.HostPathVolumeSource{
													Path: "/backups",
													Type: hostPathTypePtr(corev1.HostPathDirectory),
												},
											},
										},
									},
								},
							},
						},
					},
				),
				namespace: "velero",
				bucket:    "my-bucket",
				prefix:    "",
				path:      "/var/velero-local-volume-provider/my-bucket",
				config: map[string]string{
					"bucket": "my-bucket",
					"prefix": "",
					"path":   "/backups",
				},
				pluginOpts: &localVolumeObjectStoreOpts{
					securityContextRunAsUser:  "1001",
					securityContextRunAsGroup: "1001",
					securityContextFSGroup:    "2001",
				},
				volumeType: Hostpath,
				log:        logrus.NewEntry(logrus.New()),
			},
			wantDeployment: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: "velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "velero",
									Env:  getVeleroContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "plugins",
											MountPath: "/plugins",
										},
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
								{
									Name:    "local-volume-provider",
									Image:   "replicated/local-volume-provider:main",
									Command: []string{"/local-volume-fileserver"},
									Env:     getLVPContainerEnv(),
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "plugins",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "my-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  pointer.Int64Ptr(1001),
								RunAsGroup: pointer.Int64Ptr(1001),
								FSGroup:    pointer.Int64Ptr(2001),
							},
						},
					},
				},
			},
			wantDaemonSet: &appsv1.DaemonSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "DaemonSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restic",
					Namespace: "velero",
				},
				Spec: appsv1.DaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "restic",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "my-bucket",
											MountPath: "/var/velero-local-volume-provider/my-bucket",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "my-bucket",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/backups",
											Type: hostPathTypePtr(corev1.HostPathDirectory),
										},
									},
								},
							},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  pointer.Int64Ptr(1001),
								RunAsGroup: pointer.Int64Ptr(1001),
								FSGroup:    pointer.Int64Ptr(2001),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureResources(tt.opts)
			require.NoError(t, err)

			if tt.wantDeployment != nil {
				got, err := tt.opts.clientset.AppsV1().Deployments("velero").Get(context.TODO(), "velero", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, tt.wantDeployment.Spec.Template.Spec, got.Spec.Template.Spec)
			}

			if tt.wantDaemonSet != nil {
				got, err := tt.opts.clientset.AppsV1().DaemonSets("velero").Get(context.TODO(), "restic", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, tt.wantDaemonSet.Spec.Template.Spec, got.Spec.Template.Spec)
			}
		})
	}
}

func getVeleroContainerEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}
}

func getLVPContainerEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
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
	}
}
