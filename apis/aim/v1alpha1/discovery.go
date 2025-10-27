// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// This file includes the structs required to the status and output of the discovery job

// ProfileDiscoveryStatusEnum defines the discovery status for a specific profile.
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;NotAvailable
type ProfileDiscoveryStatusEnum string

const (
	// ProfileDiscoveryStatusPending indicates discovery has not started for this profile.
	ProfileDiscoveryStatusPending ProfileDiscoveryStatusEnum = "Pending"

	// ProfileDiscoveryStatusRunning indicates discovery is currently in progress.
	ProfileDiscoveryStatusRunning ProfileDiscoveryStatusEnum = "Running"

	// ProfileDiscoveryStatusCompleted indicates discovery completed successfully.
	ProfileDiscoveryStatusCompleted ProfileDiscoveryStatusEnum = "Completed"

	// ProfileDiscoveryStatusFailed indicates discovery failed for this profile.
	ProfileDiscoveryStatusFailed ProfileDiscoveryStatusEnum = "Failed"

	// ProfileDiscoveryStatusNotAvailable indicates the GPU type for this profile doesn't exist in the cluster.
	ProfileDiscoveryStatusNotAvailable ProfileDiscoveryStatusEnum = "NotAvailable"
)

// AIMDiscoveryProfile is the object that is discovered when performing the AIM image dry run
type AIMDiscoveryProfile struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EngineArgs *apiextensionsv1.JSON `json:"engine_args,omitempty"`

	EnvVars map[string]string `json:"env_vars,omitempty"`

	Metadata AIMDiscoveryProfileMetadata `json:"metadata,omitempty"`
}

type AIMDiscoveryProfileMetadata struct {
	Engine    string       `json:"engine,omitempty"`
	GPU       string       `json:"gpu,omitempty"`
	GPUCount  int32        `json:"gpu_count,omitempty"`
	Metric    AIMMetric    `json:"metric,omitempty"`
	Precision AIMPrecision `json:"precision,omitempty"`
}

type AIMModelSource struct {
	// Name is the name of the model
	// +optional
	Name string `json:"name,omitempty"`

	// SourceURI is the source where the model should be downloaded from
	SourceURI string `json:"sourceUri"`

	// Size is the amount of storage that the source expects
	Size resource.Quantity `json:"size"`
}
