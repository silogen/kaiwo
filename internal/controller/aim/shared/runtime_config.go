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
// Namespace config completely overrides cluster config when present (no merging).
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

// ResolveRuntimeConfig resolves runtime config using complete override semantics.
// Namespace config completely replaces cluster config when present - no field-level merging.
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

	// Namespace config completely overrides cluster config (no merging)
	if resolution.NamespaceConfig != nil {
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
		// Convert cluster config spec to namespace config spec format
		resolution.EffectiveSpec = aimv1alpha1.AIMRuntimeConfigSpec{
			AIMRuntimeConfigCommon: resolution.ClusterConfig.Spec.AIMRuntimeConfigCommon,
			// Credentials fields remain empty when using cluster config
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

// NormalizeRuntimeConfigName returns the effective name to use for lookups when the user omits the field.
func NormalizeRuntimeConfigName(name string) string {
	if name == "" {
		return DefaultRuntimeConfigName
	}
	return name
}
