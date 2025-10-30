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

import "k8s.io/apimachinery/pkg/types"

// AIMResolutionScope describes the scope of a resolved reference.
// +kubebuilder:validation:Enum=Namespace;Cluster;Merged;Unknown
type AIMResolutionScope string

const (
	// AIMResolutionScopeNamespace denotes a namespace-scoped resource.
	AIMResolutionScopeNamespace AIMResolutionScope = "Namespace"
	// AIMResolutionScopeCluster denotes a cluster-scoped resource.
	AIMResolutionScopeCluster AIMResolutionScope = "Cluster"
	// AIMResolutionScopeMerged denotes that both cluster and namespace configs were merged.
	AIMResolutionScopeMerged AIMResolutionScope = "Merged"
	// AIMResolutionScopeUnknown denotes that the scope could not be determined.
	AIMResolutionScopeUnknown AIMResolutionScope = "Unknown"
)

// AIMResolvedReference captures metadata about a resolved reference.
type AIMResolvedReference struct {
	// Name is the resource name that satisfied the reference.
	Name string `json:"name,omitempty"`

	// Namespace identifies where the resource was found when namespace-scoped.
	// Empty indicates a cluster-scoped resource.
	Namespace string `json:"namespace,omitempty"`

	// Scope indicates whether the resolved resource was namespace or cluster scoped.
	Scope AIMResolutionScope `json:"scope,omitempty"`

	// Kind is the fully-qualified kind of the resolved reference, when known.
	// +optional
	Kind string `json:"kind,omitempty"`

	// UID captures the unique identifier of the resolved reference, when known.
	// +optional
	UID types.UID `json:"uid,omitempty"`
}

// AIMServiceResolvedTemplate retains the historical name while reusing the shared structure.
type AIMServiceResolvedTemplate struct {
	AIMResolvedReference `json:",inline"`
}

// AIMServiceTemplateScope is retained for backwards compatibility with existing consumers.
// +kubebuilder:validation:Enum=Namespace;Cluster;Unknown
type AIMServiceTemplateScope string

const (
	// AIMServiceTemplateScopeNamespace denotes a namespace-scoped template.
	AIMServiceTemplateScopeNamespace AIMServiceTemplateScope = AIMServiceTemplateScope(AIMResolutionScopeNamespace)
	// AIMServiceTemplateScopeCluster denotes a cluster-scoped template.
	AIMServiceTemplateScopeCluster AIMServiceTemplateScope = AIMServiceTemplateScope(AIMResolutionScopeCluster)
	// AIMServiceTemplateScopeUnknown denotes that the scope could not be resolved.
	AIMServiceTemplateScopeUnknown AIMServiceTemplateScope = AIMServiceTemplateScope(AIMResolutionScopeUnknown)
)
