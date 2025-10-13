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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// RuntimeConfigResolution captures the merged runtime configuration.
type RuntimeConfigResolution struct {
	// Name is the runtime config name requested by the consumer.
	Name string

	// Namespace is the consumer namespace used when searching for AIMRuntimeConfig.
	Namespace string

	ClusterConfig           *aimv1alpha1.AIMClusterRuntimeConfig
	NamespaceConfig         *aimv1alpha1.AIMRuntimeConfig
	ClusterConfigNotFound   bool
	NamespaceConfigNotFound bool

	EffectiveSpec   aimv1alpha1.AIMRuntimeConfigSpec
	EffectiveStatus *aimv1alpha1.AIMEffectiveRuntimeConfig
}

// ErrRuntimeConfigNotFound indicates that neither namespace nor cluster runtime config could be located.
var ErrRuntimeConfigNotFound = stderrors.New("runtime config not found")

// ResolveRuntimeConfig merges namespace and cluster runtime configs with namespace precedence.
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
				return nil, errors.Wrapf(err, "failed to get AIMRuntimeConfig %q in namespace %q", name, namespace)
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
			return nil, errors.Wrapf(err, "failed to get AIMClusterRuntimeConfig %q", name)
		}
		resolution.ClusterConfigNotFound = true
	} else {
		resolution.ClusterConfig = &clusterCfg
	}

	if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil && name != DefaultRuntimeConfigName {
		return nil, errors.Wrapf(ErrRuntimeConfigNotFound, "runtime config %q not found", name)
	}

	resolution.EffectiveSpec = mergeRuntimeConfigs(resolution.ClusterConfig, resolution.NamespaceConfig)

	effectiveStatus, err := buildEffectiveRuntimeConfigStatus(resolution.ClusterConfig, resolution.NamespaceConfig, resolution.EffectiveSpec)
	if err != nil {
		return nil, err
	}
	resolution.EffectiveStatus = effectiveStatus

	return resolution, nil
}

// NormalizeRuntimeConfigName returns the effective name to use for lookups when the user omits the field.
func NormalizeRuntimeConfigName(name string) string {
	if name == "" {
		return DefaultRuntimeConfigName
	}
	return name
}

func mergeRuntimeConfigs(clusterCfg *aimv1alpha1.AIMClusterRuntimeConfig, namespaceCfg *aimv1alpha1.AIMRuntimeConfig) aimv1alpha1.AIMRuntimeConfigSpec {
	var effective aimv1alpha1.AIMRuntimeConfigSpec

	if clusterCfg != nil {
		effective.AIMRuntimeConfigCommon = copyRuntimeConfigCommon(clusterCfg.Spec.AIMRuntimeConfigCommon)
	}

	if namespaceCfg != nil {
		nsSpec := namespaceCfg.Spec

		if nsSpec.DefaultStorageClassName != "" {
			effective.DefaultStorageClassName = nsSpec.DefaultStorageClassName
		}

		if nsSpec.ServiceAccountName != "" {
			effective.ServiceAccountName = nsSpec.ServiceAccountName
		}

		if len(nsSpec.ImagePullSecrets) > 0 {
			effective.ImagePullSecrets = mergeImagePullSecrets(effective.ImagePullSecrets, nsSpec.ImagePullSecrets)
		}
	}

	return effective
}

func copyRuntimeConfigCommon(common aimv1alpha1.AIMRuntimeConfigCommon) aimv1alpha1.AIMRuntimeConfigCommon {
	result := aimv1alpha1.AIMRuntimeConfigCommon{
		DefaultStorageClassName: common.DefaultStorageClassName,
	}
	return result
}

func mergeImagePullSecrets(base []corev1.LocalObjectReference, overrides []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(base) == 0 {
		return append([]corev1.LocalObjectReference(nil), overrides...)
	}

	result := append([]corev1.LocalObjectReference(nil), base...)
	index := make(map[string]int, len(result))
	for i, secret := range result {
		index[secret.Name] = i
	}

	for _, secret := range overrides {
		if pos, ok := index[secret.Name]; ok {
			result[pos] = secret
			continue
		}
		index[secret.Name] = len(result)
		result = append(result, secret)
	}

	return result
}

func buildEffectiveRuntimeConfigStatus(clusterCfg *aimv1alpha1.AIMClusterRuntimeConfig, namespaceCfg *aimv1alpha1.AIMRuntimeConfig, effective aimv1alpha1.AIMRuntimeConfigSpec) (*aimv1alpha1.AIMEffectiveRuntimeConfig, error) {
	status := &aimv1alpha1.AIMEffectiveRuntimeConfig{}

	if namespaceCfg != nil {
		status.NamespaceRef = &aimv1alpha1.AIMRuntimeConfigReference{
			Name:      namespaceCfg.Name,
			Namespace: namespaceCfg.Namespace,
			UID:       namespaceCfg.UID,
			Kind:      "AIMRuntimeConfig",
		}
	}

	if clusterCfg != nil {
		status.ClusterRef = &aimv1alpha1.AIMRuntimeConfigReference{
			Name: clusterCfg.Name,
			UID:  clusterCfg.UID,
			Kind: "AIMClusterRuntimeConfig",
		}
	}

	hashBytes, err := json.Marshal(effective)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal effective runtime config")
	}
	sum := sha256.Sum256(hashBytes)
	status.Hash = hex.EncodeToString(sum[:])

	if status.NamespaceRef == nil && status.ClusterRef == nil && status.Hash == "" {
		return nil, nil
	}

	return status, nil
}
