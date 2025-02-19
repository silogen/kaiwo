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

package workloadshared

import (
	"context"
	"path/filepath"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

type DownloadJobConfigMapCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	StorageSpec *kaiwov1alpha1.StorageSpec
}

func NewDownloadJobConfigMapCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], storageSpec *kaiwov1alpha1.StorageSpec) *DownloadJobConfigMapCommand {
	cmd := &DownloadJobConfigMapCommand{
		CommandBase: base,
		StorageSpec: storageSpec,
	}
	cmd.Self = cmd
	return cmd
}

const (
	configMapFilename = "config.yaml"
	secretsMount      = "/app/secrets"
)

var (
	DefaultDataMountPath = baseutils.GetEnv("DEFAULT_DATA_MOUNT_PATH", "/workload")
	DefaultHfMountPath   = baseutils.GetEnv("DEFAULT_HF_MOUNT_PATH", "/.cache/huggingface")
)

func (cmd *DownloadJobConfigMapCommand) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	logger := log.FromContext(ctx)

	// Update secret paths
	setSecretPath := func(ref *kaiwov1alpha1.ValueReference) {
		if ref.File == "" {
			// Path where the secret will be mounted on in the primary container
			ref.File = filepath.Join(secretsMount, ref.SecretName, ref.SecretKey)
			logger.Info("Setting secret path", "ref", ref)
		} else {
			logger.Info("Ignoring secret path, as file is set", "ref", ref)
		}
	}

	// Set secret paths

	s3Downloads := cmd.StorageSpec.Data.Download.S3
	for i := range cmd.StorageSpec.Data.Download.S3 {
		setSecretPath(&s3Downloads[i].EndpointUrl)
		setSecretPath(&s3Downloads[i].AccessKeyId)
		setSecretPath(&s3Downloads[i].SecretKey)
	}
	gcsDownloads := cmd.StorageSpec.Data.Download.GCS
	for i := range gcsDownloads {
		setSecretPath(&gcsDownloads[i].ApplicationCredentials)
	}
	azureBlobDownloads := cmd.StorageSpec.Data.Download.AzureBlob
	for i := range azureBlobDownloads {
		setSecretPath(&azureBlobDownloads[i].ConnectionString)
	}

	downloadConfig := cmd.StorageSpec.CreateConfig()
	yamlData, err := yaml.Marshal(downloadConfig)
	if err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to marshal download job config to yaml", err)
	}
	objectKey := cmd.GetObjectKey()

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Data: map[string]string{
			configMapFilename: string(yamlData),
		},
	}
	cmd.State.DownloadJobConfigMap = configMap
	return configMap, nil
}

func (cmd *DownloadJobConfigMapCommand) GetEmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

func (cmd *DownloadJobConfigMapCommand) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: cmd.Owner.GetNamespace(),
		Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "download"),
	}
}

type DownloadJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	StorageSpec *kaiwov1alpha1.StorageSpec
}

func NewDownloadJobCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], storageSpec *kaiwov1alpha1.StorageSpec) *DownloadJobCommand {
	cmd := &DownloadJobCommand{
		CommandBase: base,
		StorageSpec: storageSpec,
	}
	cmd.Self = cmd
	return cmd
}

func (cmd *DownloadJobCommand) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	logger := log.FromContext(ctx)

	configMount := "/config"

	downloadConfig := cmd.StorageSpec.CreateConfig()

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmd.State.DownloadJobConfigMap.Name,
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

	if cmd.StorageSpec.HasObjectStorageDownloads() {
		volume := corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "data"),
				},
			},
		}
		logger.Info("Object storage downloads enabled", "pvc_name", volume.PersistentVolumeClaim.ClaimName, "mount_path", downloadConfig.DownloadRoot)

		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "data",
			MountPath: downloadConfig.DownloadRoot,
		})

		secretVolumes := map[string]*corev1.Volume{}

		// Closure to add a new secret volume if required, and to add the specific secret to the secret volume items list
		addSecret := func(ref *kaiwov1alpha1.ValueReference) {
			if _, ok := secretVolumes[ref.SecretName]; !ok {
				secretVolumes[ref.SecretName] = &corev1.Volume{
					Name: "secret-" + ref.SecretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: ref.SecretName,
						},
					},
				}
				logger.Info("Added secret volume source", "secret_name", ref.SecretName)
			}

			path := ref.SecretKey

			secretVolumes[ref.SecretName].Secret.Items = append(secretVolumes[ref.SecretName].Secret.Items, corev1.KeyToPath{
				Key:  ref.SecretKey,
				Path: path,
			})
		}

		// Set secret paths

		s3Downloads := cmd.StorageSpec.Data.Download.S3
		for i := range s3Downloads {
			addSecret(&s3Downloads[i].EndpointUrl)
			addSecret(&s3Downloads[i].AccessKeyId)
			addSecret(&s3Downloads[i].SecretKey)
		}
		gcsDownloads := cmd.StorageSpec.Data.Download.GCS
		for i := range gcsDownloads {
			addSecret(&gcsDownloads[i].ApplicationCredentials)
		}
		azureBlobDownloads := cmd.StorageSpec.Data.Download.AzureBlob
		for i := range azureBlobDownloads {
			addSecret(&azureBlobDownloads[i].ConnectionString)
		}

		for secretName, secretVolume := range secretVolumes {
			volumeMount := corev1.VolumeMount{
				Name:      "secret-" + secretName,
				MountPath: secretsMount + "/" + secretName,
			}
			volumeMounts = append(volumeMounts, volumeMount)
			volumes = append(volumes, *secretVolume)
			logger.Info("Added secret mount", "secret_name", secretName, "mount_path", volumeMount.MountPath)
		}
	}

	if cmd.StorageSpec.HasHfDownloads() {
		volume := corev1.Volume{
			Name: "hf",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "hf"),
				},
			},
		}
		volumes = append(volumes, volume)
		logger.Info("HF caching enabled", "pvc_name", volume.PersistentVolumeClaim.ClaimName, "mount_path", downloadConfig.HfHome)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "hf",
			MountPath: downloadConfig.HfHome,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "HF_HOME",
			Value: cmd.StorageSpec.HuggingFace.MountPath,
		})
	}

	objectKey := cmd.GetObjectKey()

	downloadJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Image: "ghcr.io/silogen/kaiwo-python:0.2",
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

	cmd.State.DownloadJob = downloadJob
	return downloadJob, nil
}

func (cmd *DownloadJobCommand) GetEmptyObject() client.Object {
	return &batchv1.Job{}
}

func (cmd *DownloadJobCommand) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: cmd.Owner.GetNamespace(),
		Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "download"),
	}
}

func (cmd *DownloadJobCommand) GetCurrentReconcileResult(ctx context.Context) *ctrl.Result {
	logger := log.FromContext(ctx)
	// If job has finished, return
	if cmd.State.DownloadJob.Status.Succeeded >= 1 {
		logger.Info("Download job completed successfully")
		return nil
	} else if cmd.State.DownloadJob.Status.Failed >= 1 {
		// TODO Update status?
		logger.Info("Download job failed")
		return &ctrl.Result{}
	} else {
		logger.Info("Download job still in progress, requeuing until it is complete")
	}

	// Requeue after some time to check again if the job has completed
	return &ctrl.Result{RequeueAfter: 5 * time.Second}
}
