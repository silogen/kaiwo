// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package download

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/silogen/kaiwo/pkg/runtime/common"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/api"
	"github.com/silogen/kaiwo/pkg/observe"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configMapFilename           = "config.yaml"
	secretsMount                = "/app/secrets"
	KaiwoDownloadTypeLabelValue = "downloader"
)

type DownloadConfigMapReconciler struct {
	ObjectKey   client.ObjectKey
	StorageSpec v1alpha1.StorageSpec
}

func NewDownloadConfigMapReconciler(objectKey client.ObjectKey, storageSpec v1alpha1.StorageSpec) *DownloadConfigMapReconciler {
	return &DownloadConfigMapReconciler{
		ObjectKey:   objectKey,
		StorageSpec: storageSpec,
	}
}

func (r *DownloadConfigMapReconciler) GetInitializedObject() client.Object {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseutils.FormatNameWithPostfix(r.ObjectKey.Name, "download"),
			Namespace: r.ObjectKey.Namespace,
		},
	}
}

func (r *DownloadConfigMapReconciler) BuildDesired(ctx context.Context, clusterCtx api.ClusterContext) (client.Object, error) {
	if r.StorageSpec.HasObjectStorageDownloads() {
		// Update secret paths
		setSecretPath := func(ref *v1alpha1.ValueReference) {
			if ref.File == "" {
				// Path where the secret will be mounted on in the primary container
				ref.File = filepath.Join(secretsMount, ref.SecretName, ref.SecretKey)
			}
		}

		// Set secret paths
		s3Downloads := r.StorageSpec.Data.Download.S3
		for i := range r.StorageSpec.Data.Download.S3 {
			if s3Downloads[i].AccessKeyId.SecretName != "" || s3Downloads[i].SecretKey.SecretName != "" {
				setSecretPath(&s3Downloads[i].AccessKeyId)
				setSecretPath(&s3Downloads[i].SecretKey)
			}
		}
		gcsDownloads := r.StorageSpec.Data.Download.GCS
		for i := range gcsDownloads {
			setSecretPath(&gcsDownloads[i].ApplicationCredentials)
		}
		azureBlobDownloads := r.StorageSpec.Data.Download.AzureBlob
		for i := range azureBlobDownloads {
			setSecretPath(&azureBlobDownloads[i].ConnectionString)
		}
		gitDownloads := r.StorageSpec.Data.Download.Git
		for i := range gitDownloads {
			if gitDownloads[i].Username != nil {
				setSecretPath(gitDownloads[i].Username)
			}
			if gitDownloads[i].Token != nil {
				setSecretPath(gitDownloads[i].Token)
			}
		}
	}

	downloadConfig := r.StorageSpec.CreateConfig()
	yamlData, err := yaml.Marshal(downloadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal download job config to yaml: %w", err)
	}

	configMap := r.GetInitializedObject().(*corev1.ConfigMap)
	configMap.Data = map[string]string{
		configMapFilename: string(yamlData),
	}

	return configMap, nil
}

func (r *DownloadConfigMapReconciler) MutateActual(ctx context.Context, clusterCtx api.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (r *DownloadConfigMapReconciler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousWorkloadStatus v1alpha1.WorkloadStatus) (*v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	return nil, nil, nil
}

type DownloadJobSucceededReason string

const (
	DownloadJobSucceededConditionType = "DownloadJobSucceeded"

	DownloadJobFailed     DownloadJobSucceededReason = "JobFailed"
	DownloadJobInProgress DownloadJobSucceededReason = "JobInProgress"
	DownloadJobCompleted  DownloadJobSucceededReason = "JobCompleted"
	DownloadJobPending    DownloadJobSucceededReason = "JobPending"
)

type DownloadJobReconciler struct {
	ObjectKey   client.ObjectKey
	StorageSpec v1alpha1.StorageSpec
	UserEnvVars []corev1.EnvVar
}

func NewDownloadJobReconciler(objectKey client.ObjectKey, storageSpec v1alpha1.StorageSpec, userEnvVars []corev1.EnvVar) *DownloadJobReconciler {
	return &DownloadJobReconciler{
		ObjectKey:   objectKey,
		StorageSpec: storageSpec,
		UserEnvVars: userEnvVars,
	}
}

func (r *DownloadJobReconciler) GetInitializedObject() client.Object {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseutils.FormatNameWithPostfix(r.ObjectKey.Name, "download"),
			Namespace: r.ObjectKey.Namespace,
			Labels: map[string]string{
				common.KaiwoTypeLabel: KaiwoDownloadTypeLabelValue,
			},
		},
	}
}

func (r *DownloadJobReconciler) BuildDesired(ctx context.Context, clusterCtx api.ClusterContext) (client.Object, error) {
	configMount := "/config"

	downloadConfig := r.StorageSpec.CreateConfig()

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: baseutils.FormatNameWithPostfix(r.ObjectKey.Name, "download"),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: configMount,
		},
	}

	var envs []corev1.EnvVar

	if r.StorageSpec.HasObjectStorageDownloads() {
		volume := corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(r.ObjectKey.Name, common.DataStoragePostfix),
				},
			},
		}

		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "data",
			MountPath: downloadConfig.DownloadRoot,
		})

		secretVolumes := map[string]*corev1.Volume{}

		// Closure to add a new secret volume if required, and to add the specific secret to the secret volume items list
		addSecret := func(ref *v1alpha1.ValueReference) {
			if _, ok := secretVolumes[ref.SecretName]; !ok {
				secretVolumes[ref.SecretName] = &corev1.Volume{
					Name: "secret-" + ref.SecretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: ref.SecretName,
						},
					},
				}
			}

			path := ref.SecretKey

			secretVolumes[ref.SecretName].Secret.Items = append(secretVolumes[ref.SecretName].Secret.Items, corev1.KeyToPath{
				Key:  ref.SecretKey,
				Path: path,
			})
		}

		// Set secret paths

		s3Downloads := r.StorageSpec.Data.Download.S3
		for i := range s3Downloads {
			if s3Downloads[i].AccessKeyId.SecretName != "" || s3Downloads[i].SecretKey.SecretName != "" {
				addSecret(&s3Downloads[i].AccessKeyId)
				addSecret(&s3Downloads[i].SecretKey)
			}
		}
		gcsDownloads := r.StorageSpec.Data.Download.GCS
		for i := range gcsDownloads {
			addSecret(&gcsDownloads[i].ApplicationCredentials)
		}
		azureBlobDownloads := r.StorageSpec.Data.Download.AzureBlob
		for i := range azureBlobDownloads {
			addSecret(&azureBlobDownloads[i].ConnectionString)
		}
		gitDownloads := r.StorageSpec.Data.Download.Git
		for i := range gitDownloads {
			if gitDownloads[i].Username != nil {
				addSecret(gitDownloads[i].Username)
			}
			if gitDownloads[i].Token != nil {
				addSecret(gitDownloads[i].Token)
			}
		}

		for secretName, secretVolume := range secretVolumes {
			volumeMount := corev1.VolumeMount{
				Name:      "secret-" + secretName,
				MountPath: secretsMount + "/" + secretName,
			}
			volumeMounts = append(volumeMounts, volumeMount)
			volumes = append(volumes, *secretVolume)
			// logger.Info("Added secret mount", "secret_name", secretName, "mount_path", volumeMount.MountPath)
		}
	}

	if r.StorageSpec.HasHfDownloads() {
		volume := corev1.Volume{
			Name: "hf",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(r.ObjectKey.Name, common.HfStoragePostfix),
				},
			},
		}
		volumes = append(volumes, volume)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "hf",
			MountPath: downloadConfig.HfHome,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "HF_HOME",
			Value: r.StorageSpec.HuggingFace.MountPath,
		})
	}

	downloadJob := r.GetInitializedObject().(*batchv1.Job)
	downloadJob.Spec = batchv1.JobSpec{
		BackoffLimit: baseutils.Pointer(int32(0)),
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsUser:  baseutils.Pointer(int64(1000)),
					RunAsGroup: baseutils.Pointer(int64(1000)),
					FSGroup:    baseutils.Pointer(int64(1000)),
				},
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Image:           "ghcr.io/silogen/kaiwo-python:0.6",
						ImagePullPolicy: corev1.PullAlways,
						Name:            "data-downloader",
						Command: []string{
							"python",
							"-m",
							"kaiwo.downloader",
							"--log-level=INFO",
							filepath.Join(configMount, configMapFilename),
						},
						Env:          envs,
						VolumeMounts: volumeMounts,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
				Volumes: volumes,
			},
		},
	}

	if len(r.UserEnvVars) > 0 && len(downloadJob.Spec.Template.Spec.Containers) > 0 {
		c := &downloadJob.Spec.Template.Spec.Containers[0]
		c.Env = append(c.Env, r.UserEnvVars...)
	}

	return downloadJob, nil
}

func (r *DownloadJobReconciler) MutateActual(ctx context.Context, clusterCtx api.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (r *DownloadJobReconciler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousWorkloadStatus v1alpha1.WorkloadStatus) (*v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	job := obj.(*batchv1.Job)

	status, err := observe.ObserveBatchJob(job, previousWorkloadStatus)
	if err != nil {
		return nil, nil, fmt.Errorf("error observing batch job: %w", err)
	}
	var condition metav1.Condition

	switch status {
	case v1alpha1.WorkloadStatusPending:
		status = v1alpha1.WorkloadStatusRunning
		condition = metav1.Condition{
			Type:    DownloadJobSucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  string(DownloadJobPending),
			Message: "Download job pending",
		}
	case v1alpha1.WorkloadStatusRunning:
		condition = metav1.Condition{
			Type:    DownloadJobSucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  string(DownloadJobInProgress),
			Message: "Download job in progress",
		}
	case v1alpha1.WorkloadStatusStarting:
		condition = metav1.Condition{
			Type:    DownloadJobSucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  string(DownloadJobInProgress),
			Message: "Download job starting",
		}
	case v1alpha1.WorkloadStatusFailed:
		condition = metav1.Condition{
			Type:    DownloadJobSucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  string(DownloadJobFailed),
			Message: "Download job failed",
		}
	case v1alpha1.WorkloadStatusComplete:
		condition = metav1.Condition{
			Type:    DownloadJobSucceededConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  string(DownloadJobCompleted),
			Message: "Download job succeeded",
		}
	default:
		return nil, nil, fmt.Errorf("unexpected job status: %s", status)
	}

	return baseutils.Pointer(status), []metav1.Condition{condition}, nil
}

// Observer

// DownloadJobObserver observes download Jobs
type DownloadJobObserver struct {
	observe.Identified
}

func NewDownloadJobObserver(nn types.NamespacedName, group observe.UnitGroup) *DownloadJobObserver {
	return &DownloadJobObserver{
		Identified: observe.Identified{
			NamespacedName: nn,
			Group:          group,
		},
	}
}

func (o *DownloadJobObserver) Kind() string {
	return "Job"
}

func (o *DownloadJobObserver) Observe(ctx context.Context, c client.Client) (observe.UnitStatus, error) {
	// Observe a regular Kubernetes Job used for downloading
	var j batchv1.Job
	if err := c.Get(ctx, o.NamespacedName, &j); apierrors.IsNotFound(err) {
		return observe.UnitStatus{
			Phase: observe.UnitPending,
		}, nil
	} else if err != nil {
		return observe.UnitStatus{
			Phase:   observe.UnitUnknown,
			Reason:  observe.ReasonGetError,
			Message: err.Error(),
		}, nil
	}

	// Check for terminal conditions
	for _, condition := range j.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				return observe.UnitStatus{
					Phase:   observe.UnitSucceeded,
					Ready:   true,
					Reason:  condition.Reason,
					Message: condition.Message,
				}, nil
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				return observe.UnitStatus{
					Phase:   observe.UnitFailed,
					Reason:  condition.Reason,
					Message: condition.Message,
				}, nil
			}
		}
	}

	// Check if job is actively running
	if j.Status.Active > 0 {
		return observe.UnitStatus{
			Phase: observe.UnitProgressing,
		}, nil
	}

	// Default to pending
	return observe.UnitStatus{
		Phase: observe.UnitPending,
	}, nil
}
