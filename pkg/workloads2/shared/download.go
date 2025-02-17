package workloadshared

import (
	"fmt"
	"path/filepath"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DownloadJobConfigMapCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	StorageSpec *kaiwov1alpha1.StorageSpec
}

const (
	configMapFilename = "config.yaml"
	secretsMount      = "/app/secrets"
)

func (cmd *DownloadJobConfigMapCommand) Build() (client.Object, error) {
	// Update secret paths
	setSecretPath := func(ref *kaiwov1alpha1.ValueReference) {
		if ref.File == "" {
			// Path where the secret will be mounted on in the primary container
			ref.File = filepath.Join(secretsMount, ref.SecretName, ref.SecretKey)
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
		return nil, fmt.Errorf("failed to marshal download job config to yaml: %w", err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "download"),
			Namespace: cmd.Owner.GetNamespace(),
		},
		Data: map[string]string{
			configMapFilename: string(yamlData),
		},
	}
	cmd.State.DownloadJobConfigMap = configMap
	return configMap, nil
}

type DownloadJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	StorageSpec *kaiwov1alpha1.StorageSpec
}

func (cmd *DownloadJobCommand) Build() (client.Object, error) {
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
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "data"),
				},
			},
		})
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
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      secretVolume.Name,
				MountPath: secretsMount + "/" + secretName,
			})
			volumes = append(volumes, *secretVolume)
		}
	}

	if cmd.StorageSpec.HasHfDownloads() {
		volumes = append(volumes, corev1.Volume{
			Name: "hf",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "hf"),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "hf",
			MountPath: downloadConfig.HfHome,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "HF_HOME",
			Value: cmd.StorageSpec.HuggingFace.MountPath,
		})
	}

	downloadJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), "download"),
			Namespace: cmd.Owner.GetNamespace(),
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

func (cmd *DownloadJobCommand) GetCurrentReconcileResult() *ctrl.Result {
	// If job has finished, return
	if cmd.State.DownloadJob.Status.Succeeded >= 1 {
		// logger.Info("Download job completed successfully")
		return nil
	} else if cmd.State.DownloadJob.Status.Failed >= 1 {
		// logger.Info("Download job failed")
		// return nil, fmt.Errorf("download job failed")
	} else {
		// logger.Info("Download job is running, waiting for it to finish")
	}

	// Requeue after some time to check again if the job has completed
	return &ctrl.Result{RequeueAfter: 5 * time.Second}
}
