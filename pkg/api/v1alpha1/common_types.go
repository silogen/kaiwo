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
)

// Common labels used across resources.
const (
	UserLabel  = "kaiwo/user"
	QueueLabel = "kueue.x-k8s.io/queue-name"
)

type Status string

const (
	StatusNew      Status = ""
	StatusPending  Status = "PENDING"
	StatusStarting Status = "STARTING"
	StatusReady    Status = "READY"
	StatusRunning  Status = "RUNNING"
	StatusComplete Status = "COMPLETE"
	StatusFailed   Status = "FAILED"
)

// CommonMetaSpec defines reusable metadata fields for workloads.
type CommonMetaSpec struct {
	// Annotations provides additional metadata for the workload.
	Annotations map[string]string `json:"annotations,omitempty"`

	// User specifies the owner or creator of the workload.
	// If authentication is enabled, this must be email address which is checked against authenticated user for match.
	User *string `json:"user,omitempty"`

	PodTemplateSpecLabels map[string]string `json:"podTemplateSpecLabels,omitempty"`

	// Gpus specifies the total number of GPUs allocated to the workload.
	// Default is 0.
	// +kubebuilder:default=0
	Gpus *int `json:"gpus,omitempty"`

	// GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).
	// Default is AMD.
	// +kubebuilder:default=AMD
	GpuVendor *string `json:"gpuVendor,omitempty"`

	// Version is an optional field specifying the version of the workload.
	Version *string `json:"version,omitempty"`

	// Replicas specifies the number of replicas for the workload.
	// If greater than one, the workload must use Ray.
	// Default is 0.
	// +kubebuilder:default=1
	Replicas *int `json:"replicas,omitempty"`

	// GpusPerReplica specifies the number of GPUs allocated per replica.
	GpusPerReplica *int `json:"gpus-per-replica,omitempty"`

	// Resources specify the default resource requirements applied for all pods inside the workflow
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Image defines the container image used for the workload.
	Image *string `json:"image,omitempty"`

	// ImagePullSecrets contains the list of secrets used to pull the container image.
	ImagePullSecrets *[]corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Env specifies the environment variables to be passed to the container.
	Env *[]corev1.EnvVar `json:"env,omitempty"`

	// SecretVolumes list the secret volumes that should be mounted
	SecretVolumes *[]SecretVolume `json:"secretVolumes,omitempty"`

	// Ray determines whether the operator should use RayCluster for workload execution.
	// Default is false.
	// +kubebuilder:default=false
	Ray *bool `json:"ray,omitempty"`

	// Storage configuration for the workload.
	Storage *StorageSpec `json:"storage,omitempty"`

	// Dangerous disables adding the default security context to the containers
	// +kubebuilder:default=false
	Dangerous *bool `json:"dangerous,omitempty"`
}

// StorageSpec defines the storage configuration for the workload.
type StorageSpec struct {
	// StorageEnabled tells whether to enable persistent storage.
	StorageEnabled bool `json:"storageEnabled,omitempty"`

	// StorageClassName specifies the storage class used for PVC.
	StorageClassName string `json:"storageClassName,omitempty"`

	// AccessMode determines the access mode for the storage
	// +kubebuilder:default=ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode"`

	// Data specifies the main workload PVC and optional object storage pre-downloads
	Data *DataStorageSpec `json:"data,omitempty"`

	// HuggingFace specifies any hugging face models that should be cached before the workload starts
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

type DownloadTaskConfig struct {
	// DownloadRoot specifies the common root for all the downloads
	DownloadRoot string `json:"downloadRoot" yaml:"downloadRoot"`

	// HfHome specifies path for $HF_HOME env variable
	HfHome string `json:"hfHome" yaml:"hfHome"`

	S3        []S3DownloadItem               `json:"s3,omitempty" yaml:"s3,omitempty"`
	GCS       []GCSDownloadItem              `json:"gcs,omitempty" yaml:"gcs,omitempty"`
	HF        []HuggingFaceDownloadItem      `json:"hf,omitempty" yaml:"hf,omitempty"`
	AzureBlob []AzureBlobStorageDownloadItem `json:"azureBlob,omitempty" yaml:"azureBlob,omitempty"`
	Git       []GitDownloadItem              `json:"git,omitempty" yaml:"git,omitempty"`
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

type DataStorageSpec struct {
	// MountPath specifies where the data PVC will be mounted to in each pod
	MountPath string `json:"mountPath,omitempty"`

	// StorageSize specifies the amount of storage allocated to the data PVC
	StorageSize string `json:"storageSize,omitempty"`

	// Download optional object storage pre-downloads
	Download ObjectStorageDownloadSpec `json:"download,omitempty"`
}

func (spec *DataStorageSpec) IsRequested() bool {
	return spec != nil && spec.StorageSize != ""
}

type ValueReference struct {
	// File determines the location of the secret value mounted on the disk
	File string `json:"file,omitempty"`

	// SecretName is the name of the secret where the value is kept
	SecretName string `json:"secretName,omitempty" yaml:"secretName,omitempty"`

	// SecretKey is the name of the key within the secret where the value is kept
	SecretKey string `json:"secretKey,omitempty" yaml:"secretKey,omitempty"`
}

type S3DownloadItem struct {
	// EndpointUrl is the endpoint of the S3 API
	EndpointUrl ValueReference `json:"endpointUrl" yaml:"endpointUrl"`

	// AccessKeyId
	AccessKeyId ValueReference        `json:"accessKeyId" yaml:"accessKeyId"`
	SecretKey   ValueReference        `json:"secretKey" yaml:"secretKey"`
	Buckets     []CloudDownloadBucket `json:"buckets"`
}

type GCSDownloadItem struct {
	ApplicationCredentials ValueReference        `json:"applicationCredentials" yaml:"applicationCredentials"`
	Buckets                []CloudDownloadBucket `json:"buckets"`
}

type AzureBlobStorageDownloadItem struct {
	ConnectionString ValueReference        `json:"connectionString" yaml:"connectionString"`
	Containers       []CloudDownloadBucket `json:"containers"`
}

type HuggingFaceDownloadItem struct {
	RepoID string   `json:"repoId" yaml:"repoId"`
	Files  []string `json:"files"`
}

type GitDownloadItem struct {
	Repository string `json:"repository" yaml:"repository,omitempty"`

	// Branch specifies the branch to use, ignored if commit is given
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`

	// Commit specifies the commit to use, prioritized over branch
	Commit   string          `json:"commit,omitempty" yaml:"commit,omitempty"`
	Username *ValueReference `json:"username,omitempty" yaml:"username,omitempty"`
	Token    *ValueReference `json:"token,omitempty" yaml:"token,omitempty"`

	// Path denotes the path within the repository to copy. If not given, whole repository is copied
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// TargetPath denotes where the path is copied to, relative to the data mount directory
	TargetPath string `json:"targetPath" yaml:"targetPath"`
}

type CloudDownloadBucket struct {
	Name    string                `json:"name"`
	Files   []CloudDownloadFile   `json:"files,omitempty"`
	Folders []CloudDownloadFolder `json:"folders,omitempty"`
}

type CloudDownloadFolder struct {
	Path       string `json:"path"`
	TargetPath string `json:"targetPath" yaml:"targetPath"`
	Glob       string `json:"glob,omitempty" yaml:"glob,omitempty"`
}

type CloudDownloadFile struct {
	Path       string `json:"path"`
	TargetPath string `json:"targetPath" yaml:"targetPath"`
}

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

type HfStorageSpec struct {
	// MountPath specifies where the data HF will be mounted to in each pod.
	// This is also used to set the HF_HOME environmental variable into each container.
	MountPath string `json:"mountPath,omitempty"`

	// StorageSize specifies the amount of storage allocated to the HF PVC
	StorageSize string `json:"storageSize,omitempty"`

	// PreCacheRepos is a list of repositories (and their files) that should be cached before the workload starts
	PreCacheRepos []HuggingFaceDownloadItem `json:"preCacheRepos,omitempty"`
}

func (spec *HfStorageSpec) IsRequested() bool {
	return spec != nil && spec.StorageSize != ""
}

type SecretVolume struct {
	Name       string `json:"name,omitempty"`
	SecretName string `json:"secretName,omitempty"`
	Key        string `json:"key,omitempty"`
	SubPath    string `json:"subPath,omitempty"`
	MountPath  string `json:"mountPath,omitempty"`
}
