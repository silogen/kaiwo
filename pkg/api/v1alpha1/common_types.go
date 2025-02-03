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
	// Whether to enable persistent storage.
	StorageEnabled bool `json:"storageEnabled,omitempty"`

	// StorageClassName specifies the storage class used for PVC.
	StorageClassName string `json:"storageClassName,omitempty"`

	// Quantity of storage requested (e.g., "100Gi").
	StorageSize string `json:"storageSize,omitempty"`
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
