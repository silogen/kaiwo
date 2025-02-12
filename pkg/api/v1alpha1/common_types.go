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

// CommonMetaSpec defines reusable metadata fields for workloads.
type CommonMetaSpec struct {
	// Name is the name of the workload.
	Name string `json:"name,omitempty"`

	// Namespace defines the namespace in which the workload is deployed.
	Namespace string `json:"namespace,omitempty"`

	// Labels is a map of key-value pairs used for organizing workloads.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations provides additional metadata for the workload.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// StorageSpec defines the storage configuration for the workload.
type StorageSpec struct {
	// StorageEnabled tells whether to enable persistent storage.
	StorageEnabled bool `json:"storageEnabled,omitempty"`

	// StorageClassName specifies the storage class used for PVC.
	StorageClassName string `json:"storageClassName,omitempty"`

	// Data specifies the main workload PVC and optional object storage pre-downloads
	Data DataStorageSpec `json:"data,omitempty"`

	// HuggingFace specifies any hugging face models that should be cached before the workload starts
	HuggingFace HfStorageSpec `json:"huggingFace,omitempty"`
}

type DownloadTaskConfig struct {
	// DownloadRoot specifies the common root for all the downloads
	DownloadRoot string `json:"downloadRoot" yaml:"downloadRoot"`

	// HfHome specifies
	HfHome string `json:"hfHome" yaml:"hfHome"`

	S3        []S3DownloadItem               `json:"s3,omitempty" yaml:"s3,omitempty"`
	GCS       []GCSDownloadItem              `json:"gcs,omitempty" yaml:"gcs,omitempty"`
	HF        []HuggingFaceDownloadItem      `json:"hf,omitempty" yaml:"hf,omitempty"`
	AzureBlob []AzureBlobStorageDownloadItem `json:"azureBlob,omitempty" yaml:"azureBlob,omitempty"`
}

// CreateConfig creates the config required for the data downloader
func (s *StorageSpec) CreateConfig() DownloadTaskConfig {
	config := DownloadTaskConfig{}

	if s.Data.StorageSize != "" {
		if s.Data.MountPath != "" {
			config.DownloadRoot = s.Data.MountPath
		} else {
			config.DownloadRoot = "/workload"
		}

		config.GCS = s.Data.Download.GCS
		config.S3 = s.Data.Download.S3
		config.AzureBlob = s.Data.Download.AzureBlob

	}

	if s.HuggingFace.StorageSize != "" {
		config.HfHome = s.HuggingFace.MountPath
		config.HF = s.HuggingFace.PreCacheRepos
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
	return spec.StorageSize != ""
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
}

type HfStorageSpec struct {
	// MountPath specifies where the data HF will be mounted to in each pod
	MountPath string `json:"mountPath,omitempty"`

	// StorageSize specifies the amount of storage allocated to the HF PVC
	StorageSize string `json:"storageSize,omitempty"`

	// PreCacheRepos is a list of repositories (and their files) that should be cached before the workload starts
	PreCacheRepos []HuggingFaceDownloadItem `json:"preCacheRepos,omitempty"`
}

func (spec *HfStorageSpec) IsRequested() bool {
	return spec.StorageSize != ""
}

// ConfigSource defines the source of configuration files for the workload.
type ConfigSource struct {
	// Type specifies the type of configuration source. Valid values are:
	// - "ConfigMap": Configuration is stored in a Kubernetes ConfigMap.
	// - "S3": Configuration is stored in an S3-compatible object storage bucket.
	Type string `json:"type,omitempty"`

	// ConfigMapName specifies the name of the ConfigMap to be mounted.
	// This field is only relevant if Type is "ConfigMap".
	ConfigMapName string `json:"configMapName,omitempty"`

	// S3BucketName specifies the name of the S3 bucket where configuration files are stored.
	// This field is only relevant if Type is "S3".
	S3BucketName string `json:"s3BucketName,omitempty"`

	// S3Path specifies an optional path within the S3 bucket where the configuration files reside.
	// This field is only relevant if Type is "S3".
	S3Path string `json:"s3Path,omitempty"`

	// S3Credentials references a Kubernetes Secret containing credentials for accessing the S3 bucket.
	// The Secret should have standard AWS credential keys (e.g., "AWS_ACCESS_KEY_ID" and "AWS_SECRET_ACCESS_KEY").
	// This field is only relevant if Type is "S3".
	S3Credentials *corev1.SecretReference `json:"s3Credentials,omitempty"`
}

type ConfigSourceItem struct {
	Path  string `json:"mountPath,omitempty"`
	Value string `json:"value,omitempty"`
}
