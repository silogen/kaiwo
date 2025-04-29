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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Status string

const (
	// StatusNew indicates the resource has been created but not yet processed by the controller.
	StatusNew Status = ""
	// StatusPending indicates the resource is waiting for prerequisites (like download jobs or Kueue admission) to complete.
	StatusPending Status = "PENDING"
	// StatusStarting indicates the underlying workload (Job, Deployment, RayService) is being created or started.
	StatusStarting Status = "STARTING"
	// StatusReady indicates a KaiwoService is fully deployed and ready to serve requests (Deployment ready or RayService healthy). Not applicable to KaiwoJob.
	StatusReady Status = "READY"
	// StatusRunning indicates the workload pods are running. For KaiwoJob, this means the job has started execution. For KaiwoService, pods are up but may not yet be fully ready/healthy.
	StatusRunning Status = "RUNNING"
	// StatusComplete indicates a KaiwoJob has finished successfully.
	StatusComplete Status = "COMPLETE"
	// StatusFailed indicates the workload (KaiwoJob or KaiwoService) or its prerequisites (like download jobs) encountered an error and cannot proceed or recover.
	StatusFailed Status = "FAILED"
)

// CommonMetaSpec defines reusable metadata fields for workloads.
type CommonMetaSpec struct {
	// User specifies the owner or creator of the workload. It should typically be the user's email address. This value is primarily used for labeling (`kaiwo.silogen.ai/user`) the generated resources (like Pods, Jobs, Deployments) for identification and filtering (e.g., with `kaiwo list --user <email>`).
	//
	// In the future, if authentication is enabled, this must be the email address which is checked against authenticated user for match.
	User string `json:"user,omitempty"`

	// PodTemplateSpecLabels allows you to specify custom labels that will be added to the `template.metadata.labels` section of the generated Pods (within Jobs, Deployments, or RayCluster specs). Standard Kaiwo system labels (like `kaiwo.silogen.ai/user`, `kaiwo.silogen.ai/name`, etc.) are added automatically and take precedence if there are conflicts.
	PodTemplateSpecLabels map[string]string `json:"podTemplateSpecLabels,omitempty"`

	// Gpus specifies the total number of GPUs allocated to the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.
	// +kubebuilder:default=0
	Gpus int `json:"gpus,omitempty"`

	// GpuVendor specifies the GPU vendor (e.g., amd, nvidia, etc.). See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.
	// +kubebuilder:default=amd
	GpuVendor string `json:"gpuVendor,omitempty"`

	// Version allows you to specify an optional version string for the workload. This can be useful for tracking different iterations or configurations of the same logical workload. It does not directly affect resource creation but serves as metadata.
	Version string `json:"version,omitempty"`

	// Replicas specifies the number of replicas for the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.
	// +kubebuilder:default=1
	Replicas *int `json:"replicas,omitempty"`

	// GpusPerReplica specifies the number of GPUs allocated per replica. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.
	//
	// If you specify `gpusPerReplica`, you must also specify `replicas`.
	GpusPerReplica int `json:"gpusPerReplica,omitempty"`

	// Resources specify the default resource requirements applied for all pods inside the workflow.
	//
	// This field defines default Kubernetes `ResourceRequirements` (requests and limits for CPU,
	// memory, ephemeral-storage) applied to *all* containers (including init containers) within
	// the workload's pods.
	//
	// **Behavior:**
	//
	// These values act as **defaults**. If a container within the underlying Job, Deployment,
	// or Ray spec (if provided by the user) already defines a specific request or limit
	// (e.g., `memory` limit), the value from `resources` for that specific metric **will not** override it.
	//
	// **Interaction with GPU fields:** The GPU requests/limits (`amd.com/gpu` or `nvidia.com/gpu`)
	// are controlled exclusively by the `gpus`, `gpusPerReplica`, and `gpuVendor` fields
	// (and the associated calculation logic described above). Any GPU specifications within
	// the `resources` field are **ignored**.
	//
	// **Default CPU/Memory with GPUs:** When Kaiwo *generates* the underlying
	// Job/Deployment/RayCluster spec (i.e., the user did *not* provide `spec.job`,
	// `spec.deployment`, or `spec.rayService`/`spec.rayJob`), and GPUs are requested
	// (`gpusPerReplica` > 0), Kaiwo applies default CPU and Memory requests/limits
	// based on the GPU count (e.g., 4 CPU cores and 32Gi Memory per GPU).
	// These GPU-derived defaults *will* override any CPU/Memory settings defined in
	// the `resources` field in this specific scenario. If the user *does* provide
	// the underlying spec, these GPU-derived CPU/Memory defaults are not applied,
	// respecting the user's definition or the values from the `resources` field.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Image specifies the default container image to be used for the primary workload container(s).
	//
	// - If containers defined within the underlying Job, Deployment, or Ray spec do *not* specify an image, this image will be used.
	// - If this field is also empty, the latest tag of ghcr.io/silogen/rocm-ray is used
	Image string `json:"image,omitempty"`

	// ImagePullSecrets is a list of Kubernetes `LocalObjectReference` (containing just the secret `name`) referencing secrets needed to pull the container image(s). These are added to the `imagePullSecrets` field of the PodSpec for all generated pods.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Env is a list of Kubernetes `EnvVar` structs. These environment variables are added to the primary workload container(s) in the generated pods. They are appended to any environment variables already defined in the underlying Job, Deployment, or Ray spec.
	Env []corev1.EnvVar `json:"env,omitempty"`

	// SecretVolumes allows you to mount specific keys from Kubernetes Secrets as files into the workload containers.
	SecretVolumes []SecretVolume `json:"secretVolumes,omitempty"`

	// Ray determines whether the operator should use RayCluster for workload execution.
	// If `true`, Kaiwo will create Ray-specific resources.
	// If `false` (default), Kaiwo will create standard Kubernetes resources (BatchJob for `KaiwoJob`, Deployment for `KaiwoService`).
	// This setting dictates which underlying spec (`job`/`rayJob` or `deployment`/`rayService`) is primarily used.
	// +kubebuilder:default=false
	Ray bool `json:"ray,omitempty"`

	// Storage configures persistent storage using Kubernetes PersistentVolumeClaims (PVCs).
	//
	// Enabling `storage.data.download` or `storage.huggingFace.preCacheRepos` will cause Kaiwo to create a temporary Kubernetes Job (the "download job") before starting the main workload. This job runs a container that performs the downloads into the respective PVCs. The main workload only starts after the download job completes successfully.
	Storage *StorageSpec `json:"storage,omitempty"`

	// Dangerous, if when set to `true`, Kaiwo will *not* add the default `PodSecurityContext` (which normally sets `runAsUser: 1000`, `runAsGroup: 1000`, `fsGroup: 1000`) to the generated pods. Use this only if you need to run containers as root or a different specific user and understand the security implications.
	// +kubebuilder:default=false
	Dangerous bool `json:"dangerous,omitempty"`
}

type CommonStatusSpec struct {
	// StartTime records the timestamp when the first pod associated with the workload started running.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Conditions lists the observed conditions of the workload resource, following standard Kubernetes conventions. May include conditions reflecting the underlying Deployment or RayService state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status reflects the current high-level phase of the workload lifecycle (e.g., PENDING, STARTING, READY, FAILED).
	Status Status `json:"status,omitempty"`

	// Duration indicates how long the service has been running since StartTime, in seconds. Calculated periodically while running.
	Duration int64 `json:"duration,omitempty"`

	// ObservedGeneration records the `.metadata.generation` of the workload resource that was last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// StorageSpec defines the storage configuration for the workload.
type StorageSpec struct {
	// StorageEnabled must be `true` to enable the creation of any PersistentVolumeClaims defined within this spec. If `false`, `data` and `huggingFace` sections are ignored.
	StorageEnabled bool `json:"storageEnabled,omitempty"`

	// StorageClassName specifies the name of the Kubernetes `StorageClass` to use when creating PersistentVolumeClaims for `data` and `huggingFace` volumes. Must refer to an existing StorageClass in the cluster.
	StorageClassName string `json:"storageClassName,omitempty"`

	// AccessMode determines the access mode (e.g., `ReadWriteOnce`, `ReadWriteMany`, `ReadOnlyMany`) for the created PersistentVolumeClaims.
	//
	// In a multi-node setting, ReadWriteMany is generally required, as pods scheduled on different nodes cannot access ReadWriteOnce PVCs. This is true even when `replicas: 1` if you are using download jobs, as the download pod may get scheduled on a different pod than the main workload pod.
	// +kubebuilder:default=ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode"`

	// Data configures the main data PersistentVolumeClaim and optional pre-download tasks for it.
	Data *DataStorageSpec `json:"data,omitempty"`

	// HuggingFace configures a PersistentVolumeClaim specifically for caching Hugging Face models and datasets, with options for pre-caching.
	HuggingFace *HfStorageSpec `json:"huggingFace,omitempty"`
}

func (spec *StorageSpec) HasData() bool {
	return spec != nil && spec.Data != nil && spec.Data.StorageSize != ""
}

func (spec *StorageSpec) HasObjectStorageDownloads() bool {
	return spec.HasData() &&
		(len(spec.Data.Download.S3) > 0 ||
			len(spec.Data.Download.GCS) > 0 ||
			len(spec.Data.Download.AzureBlob) > 0 ||
			len(spec.Data.Download.Git) > 0)
}

func (spec *StorageSpec) HasHfDownloads() bool {
	return spec != nil && spec.HuggingFace != nil && len(spec.HuggingFace.PreCacheRepos) > 0
}

func (spec *StorageSpec) HasDownloads() bool {
	return spec != nil && (spec.HasObjectStorageDownloads() || spec.HasHfDownloads())
}

// DownloadTaskConfig is an internal structure used to configure the data download job. It aggregates download tasks from both the Data and HuggingFace specs.
type DownloadTaskConfig struct {
	// DownloadRoot specifies the common root directory within the download job's container where the 'data' PVC is mounted. Corresponds to `DataStorageSpec.MountPath`.
	DownloadRoot string `json:"downloadRoot" yaml:"downloadRoot"`

	// HfHome specifies the path within the download job's container where the Hugging Face cache PVC is mounted. Corresponds to `HfStorageSpec.MountPath` and sets the `$HF_HOME` environment variable.
	HfHome string `json:"hfHome" yaml:"hfHome"`

	// S3 lists S3 download tasks.
	S3 []S3DownloadItem `json:"s3,omitempty" yaml:"s3,omitempty"`

	// GCS lists Google Cloud Storage download tasks.
	GCS []GCSDownloadItem `json:"gcs,omitempty" yaml:"gcs,omitempty"`

	// HF lists Hugging Face pre-cache tasks.
	HF []HuggingFaceDownloadItem `json:"hf,omitempty" yaml:"hf,omitempty"`

	// AzureBlob lists Azure Blob Storage download tasks.
	AzureBlob []AzureBlobStorageDownloadItem `json:"azureBlob,omitempty" yaml:"azureBlob,omitempty"`

	// Git lists Git repository download tasks.
	Git []GitDownloadItem `json:"git,omitempty" yaml:"git,omitempty"`
}

// CreateConfig creates the config required for the data downloader
func (spec *StorageSpec) CreateConfig() DownloadTaskConfig {
	config := DownloadTaskConfig{}

	if spec.Data != nil {
		if spec.Data.StorageSize != "" {
			if spec.Data.MountPath != "" {
				config.DownloadRoot = spec.Data.MountPath
			} else {
				config.DownloadRoot = "/workload"
			}

			config.GCS = spec.Data.Download.GCS
			config.S3 = spec.Data.Download.S3
			config.AzureBlob = spec.Data.Download.AzureBlob
			config.Git = spec.Data.Download.Git
		}
	}
	if spec.HuggingFace != nil {
		if spec.HuggingFace.StorageSize != "" {
			config.HfHome = spec.HuggingFace.MountPath
			config.HF = spec.HuggingFace.PreCacheRepos
		}
	}

	return config
}

// DataStorageSpec configures the primary data volume for the workload.
type DataStorageSpec struct {
	// MountPath specifies the path inside the workload containers where the data PersistentVolumeClaim will be mounted.
	//+kubebuilder:default=/workload
	MountPath string `json:"mountPath,omitempty"`

	// StorageSize specifies the requested size for the data PersistentVolumeClaim (e.g., "100Gi", "1Ti"). If set, a PVC will be created.
	StorageSize string `json:"storageSize,omitempty"`

	// Download configures optional tasks to download data from various sources into the data volume *before* the main workload starts. See `ObjectStorageDownloadSpec`.
	Download ObjectStorageDownloadSpec `json:"download,omitempty"`
}

func (spec *DataStorageSpec) IsRequested() bool {
	return spec != nil && spec.StorageSize != ""
}

// ValueReference provides a way to reference sensitive values stored in Kubernetes Secrets, typically used for credentials needed by download tasks.
type ValueReference struct {
	// File specifies the expected path within the download job's container where the secret value will be mounted as a file. This path is usually automatically generated by the controller based on SecretName and SecretKey.
	File string `json:"file,omitempty"`

	// SecretName is the name of the Kubernetes Secret resource containing the value.
	SecretName string `json:"secretName,omitempty" yaml:"secretName,omitempty"`

	// SecretKey is the key within the specified Secret whose value should be used.
	SecretKey string `json:"secretKey,omitempty" yaml:"secretKey,omitempty"`
}

// S3DownloadItem defines parameters for downloading data from an S3-compatible object store.
type S3DownloadItem struct {
	// EndpointUrl specifies the S3 API endpoint URL (e.g., "https://s3.us-east-1.amazonaws.com" or a MinIO endpoint).
	EndpointUrl string `json:"endpointUrl" yaml:"endpointUrl"`

	// AccessKeyId optionally references a Kubernetes Secret containing the S3 access key ID. See `ValueReference`.
	AccessKeyId ValueReference `json:"accessKeyId,omitempty" yaml:"accessKeyId,omitempty"`

	// SecretKey optionally references a Kubernetes Secret containing the S3 secret access key. See `ValueReference`.
	SecretKey ValueReference `json:"secretKey,omitempty" yaml:"secretKey,omitempty"`

	// Buckets lists the S3 buckets and the specific files/folders to download from them. See `CloudDownloadBucket`.
	Buckets []CloudDownloadBucket `json:"buckets"`
}

// GCSDownloadItem defines parameters for downloading data from Google Cloud Storage.
type GCSDownloadItem struct {
	// ApplicationCredentials references a Kubernetes Secret containing the GCS service account key JSON file content. See `ValueReference`.
	ApplicationCredentials ValueReference `json:"applicationCredentials" yaml:"applicationCredentials"`

	// Buckets lists the GCS buckets and the specific files/folders to download from them. See `CloudDownloadBucket`.
	Buckets []CloudDownloadBucket `json:"buckets"`
}

// AzureBlobStorageDownloadItem defines parameters for downloading data from Azure Blob Storage.
type AzureBlobStorageDownloadItem struct {
	// ConnectionString references a Kubernetes Secret containing the Azure Storage connection string. See `ValueReference`.
	ConnectionString ValueReference `json:"connectionString" yaml:"connectionString"`

	// Containers lists the Azure Blob Storage containers and the specific files/folders to download from them. See `CloudDownloadBucket`.
	Containers []CloudDownloadBucket `json:"containers"`
}

// HuggingFaceDownloadItem defines parameters for pre-caching a Hugging Face repository or specific files from it.
type HuggingFaceDownloadItem struct {
	// RepoID is the Hugging Face Hub repository ID (e.g., "meta-llama/Llama-2-7b-chat-hf").
	RepoID string `json:"repoId" yaml:"repoId"`

	// Files is an optional list of specific files to download from the repository. If omitted, the entire repository is downloaded.
	Files []string `json:"files,omitempty"`
}

// GitDownloadItem defines parameters for cloning a Git repository or parts of it.
type GitDownloadItem struct {
	// Repository specifies the Git repository URL (e.g., "https://github.com/user/repo.git").
	Repository string `json:"repository" yaml:"repository,omitempty"`

	// Branch specifies the branch to clone. This is ignored if `commit` is specified.
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`

	// Commit specifies the exact commit hash to check out. This takes precedence over `branch`.
	Commit string `json:"commit,omitempty" yaml:"commit,omitempty"`

	// Username optionally references a Secret containing the Git username for authentication. See `ValueReference`.
	Username *ValueReference `json:"username,omitempty" yaml:"username,omitempty"`

	// Token optionally references a Secret containing the Git token (or password) for authentication. See `ValueReference`.
	Token *ValueReference `json:"token,omitempty" yaml:"token,omitempty"`

	// Path specifies a sub-path within the repository to copy. If omitted, the entire repository is copied.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// TargetPath specifies the destination path relative to the data volume's mount point (`DataStorageSpec.MountPath`) where the repository or `path` content should be copied.
	TargetPath string `json:"targetPath" yaml:"targetPath"`
}

// CloudDownloadBucket represents a specific bucket (S3, GCS) or container (Azure) to download from.
type CloudDownloadBucket struct {
	// Name is the name of the bucket or container.
	Name string `json:"name"`

	// Files lists specific files to download from this bucket/container.
	Files []CloudDownloadFile `json:"files,omitempty"`

	// Folders lists specific folders (prefixes) to download from this bucket/container.
	Folders []CloudDownloadFolder `json:"folders,omitempty"`
}

// CloudDownloadFolder specifies a folder (prefix) within a cloud bucket/container to download.
type CloudDownloadFolder struct {
	// Path is the source path (prefix) within the bucket/container (e.g., "data/images/").
	Path string `json:"path"`
	// TargetPath specifies the destination path relative to the data volume's mount point where the folder contents should be placed.
	TargetPath string `json:"targetPath" yaml:"targetPath"`
	// Glob provides an optional pattern (e.g., "*.jpg") to filter files within the source Path.
	Glob string `json:"glob,omitempty" yaml:"glob,omitempty"`
}

// CloudDownloadFile specifies a single file within a cloud bucket/container to download.
type CloudDownloadFile struct {
	// Path is the full path to the source file within the bucket/container (e.g., "models/model.bin").
	Path string `json:"path"`

	// TargetPath specifies the destination path, including the filename, relative to the data volume's mount point where the file should be saved.
	TargetPath string `json:"targetPath" yaml:"targetPath"`
}

// ObjectStorageDownloadSpec aggregates download tasks for various object storage and Git sources within the `DataStorageSpec`.
type ObjectStorageDownloadSpec struct {
	// S3 lists any S3 downloads
	S3 []S3DownloadItem `json:"s3,omitempty"`

	// GCS lists and Google Cloud Storage downloads
	GCS []GCSDownloadItem `json:"gcs,omitempty"`

	// AzureBlob lists any Azure Blob Storage downloads
	AzureBlob []AzureBlobStorageDownloadItem `json:"azureBlob,omitempty"`

	// Git lists any Git downloads
	Git []GitDownloadItem `json:"git,omitempty"`
}

// HfStorageSpec configures storage specifically for Hugging Face model caching.
type HfStorageSpec struct {
	// MountPath specifies the path inside workload containers where the Hugging Face cache PVC will be mounted.
	// This path is also automatically set as the `HF_HOME` environment variable in the containers.
	//+kubebuilder:default=/hf_cache
	MountPath string `json:"mountPath,omitempty"`

	// StorageSize specifies the requested size for the Hugging Face cache PersistentVolumeClaim (e.g., "50Gi", "200Gi"). If set, a PVC will be created.
	StorageSize string `json:"storageSize,omitempty"`

	// PreCacheRepos is a list of Hugging Face repositories to download into the cache volume *before* the main workload starts.
	PreCacheRepos []HuggingFaceDownloadItem `json:"preCacheRepos,omitempty"`
}

func (spec *HfStorageSpec) IsRequested() bool {
	return spec != nil && spec.StorageSize != ""
}

// SecretVolume defines how to mount a specific key from a Kubernetes Secret into the workload's containers.
type SecretVolume struct {
	// Name defines the name of the Kubernetes Volume that will be created. Should be unique within the pod.
	Name string `json:"name,omitempty"`

	// SecretName specifies the name of the Kubernetes Secret resource to mount from.
	SecretName string `json:"secretName,omitempty"`

	// Key specifies the key within the Secret whose value should be mounted. If omitted, the entire secret might be mounted as files (depending on Kubernetes behavior).
	Key string `json:"key,omitempty"`

	// SubPath defines the filename within the `MountPath` directory where the secret `Key`'s content will be placed. Useful for mounting a single secret key as a file.
	SubPath string `json:"subPath,omitempty"`

	// MountPath defines the directory path inside the container where the secret volume (or the `SubPath` file) should be mounted.
	MountPath string `json:"mountPath,omitempty"`
}

type KaiwoResourceUtilizationStatus string

const (
	KaiwoResourceUtilizationType                                = "ResourceUnderutilization"
	GpuResourceUtilizationNormal KaiwoResourceUtilizationStatus = "GpuUtilizationNormal"
	GpuResourceUtilizationLow    KaiwoResourceUtilizationStatus = "GpuUtilizationLow"
	CpuResourceUtilizationNormal KaiwoResourceUtilizationStatus = "CpuUtilizationNormal"
	CpuResourceUtilizationLow    KaiwoResourceUtilizationStatus = "CpuUtilizationLow"
	ResourceUtilizationUnknown   KaiwoResourceUtilizationStatus = "UtilizationUnknown"
)
