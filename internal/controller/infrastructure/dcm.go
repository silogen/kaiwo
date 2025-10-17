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

package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
)

const (
	// DCMConfigMapName is the name of the DCM ConfigMap.
	DCMConfigMapName = "config-manager-config"

	// DCMConfigMapNamespace is the namespace where the DCM ConfigMap is located.
	DCMConfigMapNamespace = "kube-amd-gpu"

	// DCMConfigMapDataKey is the key in the ConfigMap data containing the DCM configuration.
	DCMConfigMapDataKey = "config.json"

	// DCMNodeLabelKey is the label key used by DCM to select partition profiles.
	DCMNodeLabelKey = "dcm.amd.com/gpu-config-profile"

	// TaintKey is the taint key used to drain nodes before partitioning.
	TaintKey = "amd-dcm"

	// TaintValue is the taint value.
	TaintValue = "up"
)

// GetDCMConfigMap fetches the DCM ConfigMap.
func GetDCMConfigMap(ctx context.Context, c client.Client) (*corev1.ConfigMap, error) {
	logger := log.FromContext(ctx)
	var cm corev1.ConfigMap
	err := c.Get(ctx, types.NamespacedName{
		Name:      DCMConfigMapName,
		Namespace: DCMConfigMapNamespace,
	}, &cm)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DCM ConfigMap not found", "namespace", DCMConfigMapNamespace, "name", DCMConfigMapName)
			return nil, fmt.Errorf("DCM ConfigMap not found in namespace %s", DCMConfigMapNamespace)
		}
		return nil, fmt.Errorf("failed to get DCM ConfigMap: %w", err)
	}

	return &cm, nil
}

// EnsureDCMProfileInConfigMap ensures the profile from PartitioningProfile is present in the DCM ConfigMap.
// Returns the profile name that should be used in the node label.
func EnsureDCMProfileInConfigMap(
	ctx context.Context,
	c client.Client,
	profile *infrastructurev1alpha1.PartitioningProfile,
) (string, error) {
	logger := log.FromContext(ctx)

	// For Phase 1, we assume the profile is already in the ConfigMap
	// and return the profile name that should be used
	profileName := profile.Name

	logger.V(1).Info("Using DCM profile", "profileName", profileName)

	// TODO Phase 2: Actually merge the profile into the DCM ConfigMap
	// For now, we just validate that the ConfigMap exists
	_, err := GetDCMConfigMap(ctx, c)
	if err != nil {
		return "", err
	}

	return profileName, nil
}

// ApplyProfileToNode applies a DCM profile to a node by setting the appropriate label.
func ApplyProfileToNode(ctx context.Context, c client.Client, nodeName string, profileName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Check if already labeled correctly
	if currentProfile, exists := node.Labels[DCMNodeLabelKey]; exists && currentProfile == profileName {
		logger.V(1).Info("Node already has correct DCM profile label",
			"node", nodeName, "profileName", profileName)
		return nil
	}

	// Apply the label
	node.Labels[DCMNodeLabelKey] = profileName

	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to label node %s with DCM profile %s: %w", nodeName, profileName, err)
	}

	logger.Info("Applied DCM profile label to node", "node", nodeName, "profileName", profileName)
	return nil
}

// RemoveProfileFromNode removes the DCM profile label from a node.
func RemoveProfileFromNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			return nil // Node already gone, nothing to do
		}
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	if node.Labels == nil || node.Labels[DCMNodeLabelKey] == "" {
		return nil // Label not set, nothing to do
	}

	delete(node.Labels, DCMNodeLabelKey)

	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to remove DCM profile label from node %s: %w", nodeName, err)
	}

	logger.Info("Removed DCM profile label from node", "node", nodeName)
	return nil
}

// ResolveOperatorPayload resolves the operator payload from a PartitioningProfile.
// Returns the payload as a JSON map.
func ResolveOperatorPayload(
	ctx context.Context,
	c client.Client,
	payloadRef infrastructurev1alpha1.OperatorPayloadReference,
) (map[string]interface{}, error) {
	switch payloadRef.Kind {
	case "Inline":
		if payloadRef.Inline == nil {
			return nil, fmt.Errorf("inline payload is nil")
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(payloadRef.Inline.Raw, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal inline payload: %w", err)
		}
		return payload, nil

	case "ConfigMap":
		var cm corev1.ConfigMap
		if err := c.Get(ctx, types.NamespacedName{
			Name:      payloadRef.Name,
			Namespace: payloadRef.Namespace,
		}, &cm); err != nil {
			return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", payloadRef.Namespace, payloadRef.Name, err)
		}

		// Try to find the payload in the ConfigMap data
		// Look for common keys: config.json, dcm-config, profile
		for _, key := range []string{"config.json", "dcm-config", "profile"} {
			if data, ok := cm.Data[key]; ok {
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ConfigMap data[%s]: %w", key, err)
				}
				return payload, nil
			}
		}
		return nil, fmt.Errorf("ConfigMap %s/%s does not contain expected keys (config.json, dcm-config, profile)", payloadRef.Namespace, payloadRef.Name)

	case "Secret":
		var secret corev1.Secret
		if err := c.Get(ctx, types.NamespacedName{
			Name:      payloadRef.Name,
			Namespace: payloadRef.Namespace,
		}, &secret); err != nil {
			return nil, fmt.Errorf("failed to get Secret %s/%s: %w", payloadRef.Namespace, payloadRef.Name, err)
		}

		// Try to find the payload in the Secret data
		for _, key := range []string{"config.json", "dcm-config", "profile"} {
			if data, ok := secret.Data[key]; ok {
				var payload map[string]interface{}
				if err := json.Unmarshal(data, &payload); err != nil {
					return nil, fmt.Errorf("failed to unmarshal Secret data[%s]: %w", key, err)
				}
				return payload, nil
			}
		}
		return nil, fmt.Errorf("secret %s/%s does not contain expected keys (config.json, dcm-config, profile)", payloadRef.Namespace, payloadRef.Name)

	default:
		return nil, fmt.Errorf("unsupported payload reference kind: %s", payloadRef.Kind)
	}
}
