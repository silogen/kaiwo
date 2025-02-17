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

package controllerutils

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var (
	defaultDataMountPath = baseutils.GetEnv("DEFAULT_DATA_MOUNT_PATH", "/workload")
	defaultHfMountPath   = baseutils.GetEnv("DEFAULT_HF_MOUNT_PATH", "/.cache/huggingface")
)

// ReconcileDownloadJob ensures that if there is data to download to the PVC(s) before the main workload runs,
// that the job is scheduled and has completed successfully before the main workload should be scheduled
func ReconcileDownloadJob(r client.Client, s *runtime.Scheme, ctx context.Context, owner client.Object, spec *kaiwov1alpha1.StorageSpec) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !spec.StorageEnabled {
		return nil, nil
	}

	// Ensure mount paths are set
	if spec.Data.IsRequested() && spec.Data.MountPath == "" {
		logger.Info("Data storage mount path not set, using default:" + defaultDataMountPath)
		spec.Data.MountPath = defaultDataMountPath
	}
	if spec.HuggingFace.IsRequested() && spec.HuggingFace.MountPath == "" {
		logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
		spec.HuggingFace.MountPath = defaultHfMountPath
	}

	downloadJobName := baseutils.FormatNameWithPostfix(owner.GetName(), "download")

	hasObjectStorageDownloads := len(spec.Data.Download.S3) > 0 || len(spec.Data.Download.GCS) > 0 || len(spec.Data.Download.AzureBlob) > 0
	hasHfDownloads := len(spec.HuggingFace.PreCacheRepos) > 0

	if hasObjectStorageDownloads || hasHfDownloads {
		logger.Info("Reconciling download job")
		// Only run if there is something to download
		downloadJob := batchv1.Job{}

		configMount := "/config"
		configFilename := "config.yaml"

		if err := r.Get(ctx, client.ObjectKey{Name: downloadJobName, Namespace: owner.GetNamespace()}, &downloadJob); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return nil, err
			}
			// Job does not exist, create it
			logger.Info("Creating download job")

			downloadConfig := spec.CreateConfig()
			configMapName := downloadJobName

			volumes := []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMapName,
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

			if hasObjectStorageDownloads {
				volumes = append(volumes, corev1.Volume{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: baseutils.FormatNameWithPostfix(owner.GetName(), dataStoragePostfix),
						},
					},
				})
				volumeMounts = append(volumeMounts, corev1.VolumeMount{
					Name:      "data",
					MountPath: downloadConfig.DownloadRoot,
				})

				secretVolumes := map[string]*corev1.Volume{}
				secretsMount := "/app/secrets"

				// Closure to add a new secret volume if required, and to add the specific secret to the secret volume items list
				setSecretPath := func(ref *kaiwov1alpha1.ValueReference) {
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

					// Path where the secret will be mounted on in the primary container
					ref.File = filepath.Join(secretsMount, ref.SecretName, path)
				}

				// Set secret paths

				s3Downloads := spec.Data.Download.S3
				for i := range s3Downloads {
					setSecretPath(&s3Downloads[i].EndpointUrl)
					setSecretPath(&s3Downloads[i].AccessKeyId)
					setSecretPath(&s3Downloads[i].SecretKey)
				}
				gcsDownloads := spec.Data.Download.GCS
				for i := range gcsDownloads {
					setSecretPath(&gcsDownloads[i].ApplicationCredentials)
				}
				azureBlobDownloads := spec.Data.Download.AzureBlob
				for i := range azureBlobDownloads {
					setSecretPath(&azureBlobDownloads[i].ConnectionString)
				}

				for secretName, secretVolume := range secretVolumes {
					volumeMounts = append(volumeMounts, corev1.VolumeMount{
						Name:      secretVolume.Name,
						MountPath: secretsMount + "/" + secretName,
					})
					volumes = append(volumes, *secretVolume)
				}
			}

			yamlData, err := yaml.Marshal(downloadConfig)
			if err != nil {
				return nil, err
			}

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					configFilename: string(yamlData),
				},
			}

			// Ensure config map is attached to the parent kaiwo job
			if err := ctrl.SetControllerReference(owner, configMap, s); err != nil {
				logger.Error(err, "Failed to set config map controller reference")
				return nil, err
			}

			if err := r.Create(ctx, configMap); err != nil {
				return nil, err
			}

			if hasHfDownloads {
				volumes = append(volumes, corev1.Volume{
					Name: "hf",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: baseutils.FormatNameWithPostfix(owner.GetName(), hfStoragePostfix),
						},
					},
				})
				volumeMounts = append(volumeMounts, corev1.VolumeMount{
					Name:      "hf",
					MountPath: downloadConfig.HfHome,
				})
				envs = append(envs, corev1.EnvVar{
					Name:  "HF_HOME",
					Value: spec.HuggingFace.MountPath,
				})
			}

			downloadJob = batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      downloadJobName,
					Namespace: owner.GetNamespace(),
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{ // TODO: Consider making these env variables
									Image: "ghcr.io/silogen/kaiwo-python:0.2",
									Name:  "data-downloader",
									Command: []string{
										"python",
										"-m",
										"kaiwo.downloader",
										"--log-level=DEBUG",
										filepath.Join(configMount, configFilename),
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

			// Ensure download job is attached to the parent kaiwo job
			if err := ctrl.SetControllerReference(owner, &downloadJob, s); err != nil {
				logger.Error(err, "Failed to set download job controller reference")
				return nil, err
			}

			if err := r.Create(ctx, &downloadJob); err != nil {
				return nil, err
			}

			// Requeue after some time to check if the job has completed
			return &ctrl.Result{RequeueAfter: 5 * time.Second}, nil

		}

		// If job has finished, return
		if downloadJob.Status.Succeeded >= 1 {
			logger.Info("Download job completed successfully")
			return nil, nil
		} else if downloadJob.Status.Failed >= 1 {
			logger.Info("Download job failed")
			return nil, fmt.Errorf("download job failed")
		}

		// Requeue after some time to check again if the job has completed
		logger.Info("Download job is running, waiting for it to finish")
		return &ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else {
		logger.Info("No download job to reconcile")
	}
	return nil, nil
}
