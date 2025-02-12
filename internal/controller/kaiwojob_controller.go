/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"k8s.io/apimachinery/pkg/api/resource"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// KaiwoJobReconciler reconciles a KaiwoJob object
type KaiwoJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	defaultDataMountPath = "/workload"
	defaultHfMountPath   = "/.cache/huggingface"
)

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/finalizers,verbs=update

func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KaiwoJob instance
	var kaiwoJob kaiwov1alpha1.KaiwoJob
	if err := r.Get(ctx, req.NamespacedName, &kaiwoJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get KaiwoJob")
			return ctrl.Result{}, err
		}
		logger.Info("Job does not exist, it might have been deleted")
		return ctrl.Result{}, nil
	}

	if kaiwoJob.Spec.Storage.StorageEnabled {
		if kaiwoJob.Spec.Storage.StorageClassName == "" {
			return ctrl.Result{}, fmt.Errorf("storage class name is empty")
		}

		// Ensure the storage is created
		if err := r.reconcileStorage(ctx, kaiwoJob); err != nil {
			return ctrl.Result{}, err
		}

		// Ensure mount paths are set
		if kaiwoJob.Spec.Storage.Data.IsRequested() && kaiwoJob.Spec.Storage.Data.MountPath == "" {
			logger.Info("Data storage mount path not set, using default:" + defaultDataMountPath)
			kaiwoJob.Spec.Storage.Data.MountPath = defaultDataMountPath
		}
		if kaiwoJob.Spec.Storage.HuggingFace.IsRequested() && kaiwoJob.Spec.Storage.HuggingFace.MountPath == "" {
			logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
			kaiwoJob.Spec.Storage.HuggingFace.MountPath = defaultHfMountPath
		}

		// Ensure that data downloads have completed
		result, err := r.reconcileDownloadJob(ctx, kaiwoJob)
		if err != nil {
			return ctrl.Result{}, err
		}
		if result != nil {
			// Job ongoing, wait until it is finished
			return *result, nil
		}
	}

	if kaiwoJob.Spec.Image == "" {
		kaiwoJob.Spec.Image = baseutils.DefaultRayImage
		if err := r.Update(ctx, &kaiwoJob); err != nil {
			logger.Error(err, "Failed to update KaiwoJob with default image")
			return ctrl.Result{}, err
		}
	}

	if kaiwoJob.Spec.RayClusterSpec == nil || !kaiwoJob.Spec.Ray {
		return r.reconcileK8sJob(ctx, &kaiwoJob)
	} else if kaiwoJob.Spec.RayClusterSpec != nil || kaiwoJob.Spec.Ray {
		return r.reconcileRayJob(ctx, &kaiwoJob)
	}

	err := fmt.Errorf("KaiwoJob does not specify a valid Job or RayJob")
	logger.Error(err, "KaiwoJob is misconfigured", "KaiwoJob", kaiwoJob.Name)
	return ctrl.Result{}, err
}

func formatName(name string, postfix string) string {
	return baseutils.MakeRFC1123Compliant(fmt.Sprintf("%s-%s", name, postfix))
}

const (
	dataStoragePostfix = "data"
	hfStoragePostfix   = "hf"
)

func (r *KaiwoJobReconciler) reconcileStorage(ctx context.Context, kaiwoJob kaiwov1alpha1.KaiwoJob) error {
	logger := log.FromContext(ctx)

	createStorage := func(amount string, postfix string) error {
		pvcName := formatName(kaiwoJob.Name, postfix)
		logger.Info(fmt.Sprintf("Creating PVC %s", pvcName))
		storageQuantity := resource.MustParse(amount)

		dataPvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: kaiwoJob.Namespace}, dataPvc); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			logger.Info(fmt.Sprintf("PVC %s already exists", pvcName))
			return nil
		}

		dataPvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: kaiwoJob.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce, // TODO CHANGE !!!
				},
				StorageClassName: &kaiwoJob.Spec.Storage.StorageClassName,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: storageQuantity,
					},
				},
			},
		}

		// Ensure PVC is attached to the parent kaiwo job
		if err := ctrl.SetControllerReference(&kaiwoJob, dataPvc, r.Scheme); err != nil {
			logger.Error(err, "Failed to set PVC controller reference")
			return err
		}

		if err := r.Create(ctx, dataPvc); err != nil {
			return err
		}
		return nil
	}

	// Data storage
	if kaiwoJob.Spec.Storage.Data.IsRequested() {
		if err := createStorage(kaiwoJob.Spec.Storage.Data.StorageSize, dataStoragePostfix); err != nil {
			return err
		}
	}

	// HF storage
	if kaiwoJob.Spec.Storage.HuggingFace.IsRequested() {
		if err := createStorage(kaiwoJob.Spec.Storage.HuggingFace.StorageSize, hfStoragePostfix); err != nil {
			return err
		}
	}

	return nil
}

func (r *KaiwoJobReconciler) reconcileDownloadJob(ctx context.Context, kaiwoJob kaiwov1alpha1.KaiwoJob) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	downloadJobName := formatName(kaiwoJob.Name, "download")

	hasObjectStorageDownloads := len(kaiwoJob.Spec.Storage.Data.Download.S3) > 0 || len(kaiwoJob.Spec.Storage.Data.Download.GCS) > 0 || len(kaiwoJob.Spec.Storage.Data.Download.AzureBlob) > 0
	hasHfDownloads := len(kaiwoJob.Spec.Storage.HuggingFace.PreCacheRepos) > 0

	if hasObjectStorageDownloads || hasHfDownloads {
		logger.Info("Reconciling download job")
		// Only run if there is something to download
		downloadJob := batchv1.Job{}

		configMount := "/config"
		configFilename := "config.yaml"

		if err := r.Get(ctx, client.ObjectKey{Name: downloadJobName, Namespace: kaiwoJob.Namespace}, &downloadJob); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return nil, err
			}
			// Job does not exist, create it
			logger.Info("Creating download job")

			downloadConfig := kaiwoJob.Spec.Storage.CreateConfig()
			configMapName := formatName(kaiwoJob.Name, "download")

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
			logger.Info(fmt.Sprintf("!! %t", hasObjectStorageDownloads))
			if hasObjectStorageDownloads {
				volumes = append(volumes, corev1.Volume{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: formatName(kaiwoJob.Name, dataStoragePostfix),
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

				s3Downloads := kaiwoJob.Spec.Storage.Data.Download.S3
				for i := range s3Downloads {
					setSecretPath(&s3Downloads[i].EndpointUrl)
					setSecretPath(&s3Downloads[i].AccessKeyId)
					setSecretPath(&s3Downloads[i].SecretKey)
				}
				gcsDownloads := kaiwoJob.Spec.Storage.Data.Download.GCS
				for i := range gcsDownloads {
					setSecretPath(&gcsDownloads[i].ApplicationCredentials)
				}
				azureBlobDownloads := kaiwoJob.Spec.Storage.Data.Download.AzureBlob
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
					Namespace: kaiwoJob.Namespace,
				},
				Data: map[string]string{
					configFilename: string(yamlData),
				},
			}

			// Ensure config map is attached to the parent kaiwo job
			if err := ctrl.SetControllerReference(&kaiwoJob, configMap, r.Scheme); err != nil {
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
							ClaimName: formatName(kaiwoJob.Name, hfStoragePostfix),
						},
					},
				})
				volumeMounts = append(volumeMounts, corev1.VolumeMount{
					Name:      "hf",
					MountPath: downloadConfig.HfHome,
				})
				envs = append(envs, corev1.EnvVar{
					Name:  "HF_HOME",
					Value: kaiwoJob.Spec.Storage.HuggingFace.MountPath,
				})
			}

			downloadJob = batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      downloadJobName,
					Namespace: kaiwoJob.Namespace,
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
			if err := ctrl.SetControllerReference(&kaiwoJob, &downloadJob, r.Scheme); err != nil {
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

func (r *KaiwoJobReconciler) reconcileK8sJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingJob batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingJob)
	if err == nil {
		logger.Info("Kubernetes Job already exists", "Job", existingJob.Name)
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing Job")
		return ctrl.Result{}, err
	}

	jobSpec := kaiwoJob.Spec.JobSpec
	//if jobSpec == nil {
	//	logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob", kaiwoJob.Name)
	//	jobSpec = &batchv1.JobSpec{
	//		TTLSecondsAfterFinished: Int32Ptr(43200),
	//		Template: corev1.PodTemplateSpec{
	//			Spec: corev1.PodSpec{
	//				RestartPolicy: corev1.RestartPolicyNever,
	//			},
	//		},
	//	}
	//}

	if kaiwoJob.Labels == nil {
		kaiwoJob.Labels = make(map[string]string)
	}

	kaiwoJob.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(kaiwoJob.Spec.User)
	kaiwoJob.Labels[kaiwov1alpha1.QueueLabel] = kaiwoJob.Spec.ClusterQueue

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: *jobSpec,
	}

	//if err := FillPodSpec(ctx, r.Client, kaiwoJob, &job.Spec.Template.Spec); err != nil {
	//	logger.Error(err, "Failed to fill PodSpec")
	//	return ctrl.Result{}, err
	//}

	addStorageVolume := func(name string, claimName string) {
		logger.Info(fmt.Sprintf("Adding %s volume", name))
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	// Add volumes
	if kaiwoJob.Spec.Storage.Data.IsRequested() {
		addStorageVolume("data", formatName(kaiwoJob.Name, dataStoragePostfix))
	}

	if kaiwoJob.Spec.Storage.HuggingFace.IsRequested() {
		addStorageVolume("hf", formatName(kaiwoJob.Name, hfStoragePostfix))
	}

	addVolumeMount := func(container *corev1.Container, name string, path string) {
		logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}

	addEnvVar := func(container *corev1.Container, name string, value string) {
		logger.Info(fmt.Sprintf("Adding %s env var to %s", name, container.Name))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	addContainerInfo := func(container *corev1.Container) {
		if kaiwoJob.Spec.Storage.Data.IsRequested() {
			addVolumeMount(container, "data", kaiwoJob.Spec.Storage.Data.MountPath)
		}
		if kaiwoJob.Spec.Storage.HuggingFace.IsRequested() {
			addVolumeMount(container, "hf", kaiwoJob.Spec.Storage.HuggingFace.MountPath)
			addEnvVar(container, "HF_HOME", kaiwoJob.Spec.Storage.HuggingFace.MountPath)
		}
	}

	// Add volume mounts and env vars
	for i := range job.Spec.Template.Spec.Containers {
		addContainerInfo(&job.Spec.Template.Spec.Containers[i])
	}
	for i := range job.Spec.Template.Spec.InitContainers {
		addContainerInfo(&job.Spec.Template.Spec.InitContainers[i])
	}

	if err := ctrl.SetControllerReference(kaiwoJob, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "Failed to create Job")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created Kubernetes Job", "Job", job.Name)
	return ctrl.Result{}, nil
}

func (r *KaiwoJobReconciler) reconcileRayJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingRayJob rayv1.RayJob
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingRayJob)
	if err == nil {
		logger.Info("RayJob already exists", "RayJob", existingRayJob.Name)
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing RayJob")
		return ctrl.Result{}, err
	}

	rayCluster := kaiwoJob.Spec.RayClusterSpec
	if rayCluster == nil {
		logger.Info("RayClusterSpec is nil, using default RayClusterSpec", "KaiwoJob", kaiwoJob.Name)
		rayCluster = &rayv1.RayClusterSpec{
			RayVersion:              "2.9.0",
			EnableInTreeAutoscaling: BoolPtr(false),
			HeadGroupSpec: rayv1.HeadGroupSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
					},
				},
			},
			WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
				{
					Replicas:  Int32Ptr(2),
					GroupName: "default-worker-group",
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
						},
					},
				},
			},
		}
	}

	if err := FillRayClusterPodSpec(ctx, r.Client, kaiwoJob, rayCluster); err != nil {
		logger.Error(err, "Failed to fill RayClusterPodSpec")
		return ctrl.Result{}, err
	}
	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: rayv1.RayJobSpec{
			Entrypoint:     kaiwoJob.Spec.EntryPoint,
			RayClusterSpec: rayCluster,
		},
	}

	if err := ctrl.SetControllerReference(kaiwoJob, rayJob, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, rayJob); err != nil {
		logger.Error(err, "Failed to create RayJob")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created RayJob", "RayJob", rayJob.Name)
	return ctrl.Result{}, nil
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoJob{}).
		Named("kaiwojob").
		Complete(r)
}
