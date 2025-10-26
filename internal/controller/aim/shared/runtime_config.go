/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package shared

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// RuntimeConfigResolution captures the resolved runtime configuration.
// When both namespace and cluster configs exist, they are merged with namespace config taking precedence.
type RuntimeConfigResolution struct {
	// Name is the runtime config name requested by the consumer.
	Name string

	// Namespace is the consumer namespace used when searching for AIMRuntimeConfig.
	Namespace string

	ClusterConfig           *aimv1alpha1.AIMClusterRuntimeConfig
	NamespaceConfig         *aimv1alpha1.AIMRuntimeConfig
	ClusterConfigNotFound   bool
	NamespaceConfigNotFound bool

	EffectiveSpec aimv1alpha1.AIMRuntimeConfigSpec
	ResolvedRef   *aimv1alpha1.AIMResolvedRuntimeConfig
}

// ErrRuntimeConfigNotFound indicates that neither namespace nor cluster runtime config could be located.
var ErrRuntimeConfigNotFound = errors.New("runtime config not found")

// ResolveRuntimeConfig resolves runtime config with field-level merging.
// When both cluster and namespace configs exist, cluster config is used as base
// and namespace config fields override/merge on top.
// When configName is empty, the default runtime config name is used.
func ResolveRuntimeConfig(ctx context.Context, k8sClient client.Client, namespace, configName string) (*RuntimeConfigResolution, error) {
	name := NormalizeRuntimeConfigName(configName)

	resolution := &RuntimeConfigResolution{
		Name:      name,
		Namespace: namespace,
	}

	if namespace != "" {
		var nsCfg aimv1alpha1.AIMRuntimeConfig
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &nsCfg)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get AIMRuntimeConfig %q in namespace %q: %w", name, namespace, err)
			}
			resolution.NamespaceConfigNotFound = true
		} else {
			resolution.NamespaceConfig = &nsCfg
		}
	}

	var clusterCfg aimv1alpha1.AIMClusterRuntimeConfig
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, &clusterCfg)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get AIMClusterRuntimeConfig %q: %w", name, err)
		}
		resolution.ClusterConfigNotFound = true
	} else {
		resolution.ClusterConfig = &clusterCfg
	}

	if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil && name != DefaultRuntimeConfigName {
		return nil, fmt.Errorf("runtime config %q not found: %w", name, ErrRuntimeConfigNotFound)
	}

	// Merge cluster and namespace configs
	if resolution.NamespaceConfig != nil && resolution.ClusterConfig != nil {
		// Both exist: merge with namespace taking precedence
		resolution.EffectiveSpec = mergeRuntimeConfigSpecs(resolution.ClusterConfig.Spec, resolution.NamespaceConfig.Spec)
		resolution.ResolvedRef = &aimv1alpha1.AIMResolvedRuntimeConfig{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:      resolution.NamespaceConfig.Name,
				Namespace: resolution.NamespaceConfig.Namespace,
				Scope:     aimv1alpha1.AIMResolutionScopeMerged,
				Kind:      "AIMRuntimeConfig",
				UID:       resolution.NamespaceConfig.UID,
			},
		}
	} else if resolution.NamespaceConfig != nil {
		// Only namespace config exists
		resolution.EffectiveSpec = resolution.NamespaceConfig.Spec
		resolution.ResolvedRef = &aimv1alpha1.AIMResolvedRuntimeConfig{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:      resolution.NamespaceConfig.Name,
				Namespace: resolution.NamespaceConfig.Namespace,
				Scope:     aimv1alpha1.AIMResolutionScopeNamespace,
				Kind:      "AIMRuntimeConfig",
				UID:       resolution.NamespaceConfig.UID,
			},
		}
	} else if resolution.ClusterConfig != nil {
		// Only cluster config exists
		resolution.EffectiveSpec = aimv1alpha1.AIMRuntimeConfigSpec{
			AIMRuntimeConfigCommon: resolution.ClusterConfig.Spec.AIMRuntimeConfigCommon,
			// Credentials fields remain empty when using cluster config only
		}
		resolution.ResolvedRef = &aimv1alpha1.AIMResolvedRuntimeConfig{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:  resolution.ClusterConfig.Name,
				Scope: aimv1alpha1.AIMResolutionScopeCluster,
				Kind:  "AIMClusterRuntimeConfig",
				UID:   resolution.ClusterConfig.UID,
			},
		}
	}

	return resolution, nil
}

// mergeRuntimeConfigSpecs merges cluster and namespace runtime config specs.
// Cluster config is the base, namespace config fields override.
func mergeRuntimeConfigSpecs(clusterSpec aimv1alpha1.AIMClusterRuntimeConfigSpec, namespaceSpec aimv1alpha1.AIMRuntimeConfigSpec) aimv1alpha1.AIMRuntimeConfigSpec {
	merged := aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: clusterSpec.AIMRuntimeConfigCommon,
		// Start with namespace credentials (cluster configs don't have credentials)
		AIMRuntimeConfigCredentials: namespaceSpec.AIMRuntimeConfigCredentials,
	}

	// Override common fields from namespace config if set
	if namespaceSpec.DefaultStorageClassName != "" {
		merged.DefaultStorageClassName = namespaceSpec.DefaultStorageClassName
	}

	// Merge routing configuration
	merged.Routing = mergeRoutingConfig(clusterSpec.Routing, namespaceSpec.Routing)

	return merged
}

// mergeRoutingConfig merges cluster and namespace routing configurations.
// Cluster routing is the base, namespace routing fields override.
func mergeRoutingConfig(clusterRouting, namespaceRouting *aimv1alpha1.AIMRuntimeRoutingConfig) *aimv1alpha1.AIMRuntimeRoutingConfig {
	if namespaceRouting == nil && clusterRouting == nil {
		return nil
	}

	if namespaceRouting == nil {
		// Only cluster routing exists, return a copy
		return clusterRouting.DeepCopy()
	}

	if clusterRouting == nil {
		// Only namespace routing exists, return a copy
		return namespaceRouting.DeepCopy()
	}

	// Both exist: merge with namespace taking precedence
	merged := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      clusterRouting.Enabled,
		GatewayRef:   clusterRouting.GatewayRef,
		PathTemplate: clusterRouting.PathTemplate,
	}

	// Override with namespace values if set
	if namespaceRouting.Enabled != nil {
		merged.Enabled = namespaceRouting.Enabled
	}
	if namespaceRouting.GatewayRef != nil {
		merged.GatewayRef = namespaceRouting.GatewayRef
	}
	if namespaceRouting.PathTemplate != "" {
		merged.PathTemplate = namespaceRouting.PathTemplate
	}

	return merged
}

// NormalizeRuntimeConfigName returns the effective name to use for lookups when the user omits the field.
func NormalizeRuntimeConfigName(name string) string {
	if name == "" {
		return DefaultRuntimeConfigName
	}
	return name
}
