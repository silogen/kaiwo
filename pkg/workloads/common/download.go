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

package workloadcommon

import (
	"context"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	configMapFilename           = "config.yaml"
	secretsMount                = "/app/secrets"
	KaiwoDownloadTypeLabelValue = "downloader"
)

var (
	DefaultDataMountPath = baseutils.GetEnv("DEFAULT_DATA_MOUNT_PATH", "/workload")
	DefaultHfMountPath   = baseutils.GetEnv("DEFAULT_HF_MOUNT_PATH", "/.cache/huggingface")
)

type DownloadJobConfigMapReconciler struct {
	ResourceReconcilerBase[*corev1.ConfigMap]
	StorageSpec *v1alpha1.StorageSpec
}

func NewDownloadJobConfigMapReconciler(objectKey client.ObjectKey, storageSpec *v1alpha1.StorageSpec) *DownloadJobConfigMapReconciler {
	reconciler := &DownloadJobConfigMapReconciler{
		ResourceReconcilerBase: ResourceReconcilerBase[*corev1.ConfigMap]{
			ObjectKey: objectKey,
		},
		StorageSpec: storageSpec,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *DownloadJobConfigMapReconciler) Build(ctx context.Context, _ client.Client) (*corev1.ConfigMap, error) {
	logger := log.FromContext(ctx)

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
		return nil, baseutils.LogErrorf(logger, "failed to marshal download job config to yaml", err)
	}
	objectKey := r.ObjectKey

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Data: map[string]string{
			configMapFilename: string(yamlData),
		},
	}

	return configMap, nil
}

func (r *DownloadJobConfigMapReconciler) GetEmptyObject() *corev1.ConfigMap {
	return &corev1.ConfigMap{}
}

type DownloadJobReconciler struct {
	ResourceReconcilerBase[*batchv1.Job]
	StorageSpec *v1alpha1.StorageSpec
	PvcBaseName string
	UserEnvVars []corev1.EnvVar
}

func NewDownloadJobReconciler(objectKey client.ObjectKey, storageSpec *v1alpha1.StorageSpec, pvcBaseName string, userEnvVars []corev1.EnvVar) *DownloadJobReconciler {
	reconciler := &DownloadJobReconciler{
		ResourceReconcilerBase: ResourceReconcilerBase[*batchv1.Job]{
			ObjectKey: objectKey,
		},
		StorageSpec: storageSpec,
		PvcBaseName: pvcBaseName,
		UserEnvVars: userEnvVars,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *DownloadJobReconciler) Build(_ context.Context, _ client.Client) (*batchv1.Job, error) {
	// logger := log.FromContext(ctx)

	configMount := "/config"

	downloadConfig := r.StorageSpec.CreateConfig()

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						// Use the same name as the job itself
						Name: r.ObjectKey.Name,
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
					ClaimName: baseutils.FormatNameWithPostfix(r.PvcBaseName, DataStoragePostfix),
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
				// logger.Info("Added secret volume source", "secret_name", ref.SecretName)
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
					ClaimName: baseutils.FormatNameWithPostfix(r.PvcBaseName, HfStoragePostfix),
				},
			},
		}
		volumes = append(volumes, volume)
		// logger.Info("HF caching enabled", "pvc_name", volume.PersistentVolumeClaim.ClaimName, "mount_path", downloadConfig.HfHome)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "hf",
			MountPath: downloadConfig.HfHome,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "HF_HOME",
			Value: r.StorageSpec.HuggingFace.MountPath,
		})
	}

	downloadJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels: map[string]string{
				KaiwoTypeLabel: KaiwoDownloadTypeLabelValue,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: baseutils.Pointer(int32(0)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Image: "ghcr.io/silogen/kaiwo-python:0.5",
							Name:  "data-downloader",
							Command: []string{
								"python",
								"-m",
								"kaiwo.downloader",
								"--log-level=DEBUG",
								filepath.Join(configMount, configMapFilename),
							},
							Env:          envs,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if err := addEnvVars(r.UserEnvVars, &downloadJob.Spec.Template); err != nil {
		return nil, fmt.Errorf("error adding env vars: %w", err)
	}

	return downloadJob, nil
}

func (r *DownloadJobReconciler) GetEmptyObject() *batchv1.Job {
	return &batchv1.Job{}
}

func (r *DownloadJobReconciler) ShouldContinue(ctx context.Context, actual *batchv1.Job) *ctrl.Result {
	logger := log.FromContext(ctx)
	if actual.Status.Succeeded >= 1 {
		baseutils.Debug(logger, "Download job succeeded")
		return nil
	} else if actual.Status.Failed >= 1 {
		baseutils.Debug(logger, "Download job failed")
		return &ctrl.Result{}
	} else {
		baseutils.Debug(logger, "Download job still in progress, requeuing until it is complete")
	}

	// Requeue after some time to check again if the job has completed
	return &ctrl.Result{RequeueAfter: DefaultRequeueDuration}
}
