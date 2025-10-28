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
	"errors"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

const (
	// LabelAutoCreated marks models that were automatically created from service image references
	LabelAutoCreated = "aim.silogen.ai/auto-created"

	maxModelNameLength = 63
	hashSuffixLength   = 8
)

var (
	// invalidNameChars matches characters that aren't valid in Kubernetes names
	invalidNameChars = regexp.MustCompile(`[^a-z0-9-]`)
	// multiDashes matches multiple consecutive dashes
	multiDashes = regexp.MustCompile(`-+`)

	// ErrMultipleModelsFound is returned when multiple models exist with the same image URI
	ErrMultipleModelsFound = errors.New("multiple models found with the same image")
)

// ResolveOrCreateModelFromImage searches for existing models matching the image URI,
// or creates a new one if none exists. Returns the model name and scope.
func ResolveOrCreateModelFromImage(
	ctx context.Context,
	k8sClient client.Client,
	serviceNamespace string,
	imageURI string,
	runtimeConfig *aimv1alpha1.AIMRuntimeConfigSpec,
) (modelName string, scope TemplateScope, err error) {
	if imageURI == "" {
		return "", TemplateScopeNone, fmt.Errorf("image URI is empty")
	}

	// Search for existing models with this image
	models, err := findModelsWithImage(ctx, k8sClient, serviceNamespace, imageURI)
	if err != nil {
		return "", TemplateScopeNone, fmt.Errorf("failed to search for models: %w", err)
	}

	switch len(models) {
	case 0:
		// No models found - create one
		return createModelForImage(ctx, k8sClient, serviceNamespace, imageURI, runtimeConfig)
	case 1:
		// Single match - use it
		return models[0].Name, models[0].Scope, nil
	default:
		// Multiple matches - error
		names := make([]string, len(models))
		for i, m := range models {
			if m.Scope == TemplateScopeNamespace {
				names[i] = fmt.Sprintf("%s/%s (namespace)", serviceNamespace, m.Name)
			} else {
				names[i] = fmt.Sprintf("%s (cluster)", m.Name)
			}
		}
		return "", TemplateScopeNone, fmt.Errorf("%w with image %q: %s", ErrMultipleModelsFound, imageURI, strings.Join(names, ", "))
	}
}

// ModelReference represents a found model
type ModelReference struct {
	Name  string
	Scope TemplateScope
}

// findModelsWithImage searches for AIMModel and AIMClusterModel resources with the specified image
func findModelsWithImage(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	imageURI string,
) ([]ModelReference, error) {
	var results []ModelReference

	// Search namespace-scoped models
	if namespace != "" {
		var modelList aimv1alpha1.AIMModelList
		if err := k8sClient.List(ctx, &modelList, client.InNamespace(namespace)); err != nil {
			return nil, fmt.Errorf("failed to list AIMModels: %w", err)
		}
		for i := range modelList.Items {
			if modelList.Items[i].Spec.Image == imageURI {
				results = append(results, ModelReference{
					Name:  modelList.Items[i].Name,
					Scope: TemplateScopeNamespace,
				})
			}
		}
	}

	// Search cluster-scoped models
	var clusterModelList aimv1alpha1.AIMClusterModelList
	if err := k8sClient.List(ctx, &clusterModelList); err != nil {
		return nil, fmt.Errorf("failed to list AIMClusterModels: %w", err)
	}
	for i := range clusterModelList.Items {
		if clusterModelList.Items[i].Spec.Image == imageURI {
			results = append(results, ModelReference{
				Name:  clusterModelList.Items[i].Name,
				Scope: TemplateScopeCluster,
			})
		}
	}

	return results, nil
}

// createModelForImage creates a new AIMModel or AIMClusterModel for the given image
func createModelForImage(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	imageURI string,
	runtimeConfig *aimv1alpha1.AIMRuntimeConfigSpec,
) (modelName string, scope TemplateScope, err error) {
	// Generate model name from image URI
	modelName = generateModelName(imageURI)

	// Determine scope from runtime config
	creationScope := "Cluster" // default
	if runtimeConfig != nil && runtimeConfig.Model != nil && runtimeConfig.Model.CreationScope != "" {
		creationScope = runtimeConfig.Model.CreationScope
	}

	// Set runtime config name (use default if not specified)
	runtimeConfigName := "default"
	if runtimeConfig != nil {
		// Runtime config was resolved, but we need the name that was used to resolve it
		// For now, use default - the controller will re-resolve it
		runtimeConfigName = "default"
	}

	if creationScope == "Namespace" {
		// Create namespace-scoped model
		model := &aimv1alpha1.AIMModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      modelName,
				Namespace: namespace,
				Labels: map[string]string{
					LabelAutoCreated: "true",
				},
			},
			Spec: aimv1alpha1.AIMModelSpec{
				Image:             imageURI,
				RuntimeConfigName: runtimeConfigName,
				Resources:         corev1.ResourceRequirements{},
			},
		}

		if err := k8sClient.Create(ctx, model); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Race condition - another controller created it
				return modelName, TemplateScopeNamespace, nil
			}
			return "", TemplateScopeNone, fmt.Errorf("failed to create AIMModel: %w", err)
		}

		return modelName, TemplateScopeNamespace, nil
	}

	// Create cluster-scoped model (default)
	clusterModel := &aimv1alpha1.AIMClusterModel{
		ObjectMeta: metav1.ObjectMeta{
			Name: modelName,
			Labels: map[string]string{
				LabelAutoCreated: "true",
			},
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image:             imageURI,
			RuntimeConfigName: runtimeConfigName,
			Resources:         corev1.ResourceRequirements{},
		},
	}

	if err := k8sClient.Create(ctx, clusterModel); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition - another controller created it
			return modelName, TemplateScopeCluster, nil
		}
		return "", TemplateScopeNone, fmt.Errorf("failed to create AIMClusterModel: %w", err)
	}

	return modelName, TemplateScopeCluster, nil
}

// generateModelName creates a Kubernetes-valid name from an image URI.
// Format: <truncated-image-name>-<tag>-<hash>
//
// The name is constructed to ensure uniqueness while preserving readability:
// - Image name and tag are included for human readability
// - If the combined name would exceed 63 characters, the image name is truncated
// - A hash suffix ensures uniqueness even when truncation occurs
//
// Examples:
//
//	ghcr.io/silogen/llama-3-8b:v1.2.0 -> llama-3-8b-v1.2.0-a1b2c3d4
//	registry.example.com/models/mistral:latest -> mistral-latest-e5f6g7h8
func generateModelName(imageURI string) string {
	const hashLength = 8

	// Extract image name and tag from URI
	// Example: ghcr.io/silogen/llama-3-8b:v1.2.0
	parts := strings.Split(imageURI, "/")
	lastPart := parts[len(parts)-1]

	var imageName, imageTag string

	// Handle digest-based references (@sha256:...)
	if strings.Contains(lastPart, "@") {
		imageName = strings.Split(lastPart, "@")[0]
		// For digest references, use a short digest portion as "tag"
		digestParts := strings.Split(lastPart, "@")
		if len(digestParts) > 1 {
			digest := digestParts[1]
			// Extract just the hash algorithm and first few chars (e.g., sha256:abc...)
			if colonIdx := strings.Index(digest, ":"); colonIdx != -1 && colonIdx+7 <= len(digest) {
				imageTag = digest[colonIdx+1 : colonIdx+7] // First 6 chars of digest
			}
		}
	} else if strings.Contains(lastPart, ":") {
		// Handle tag-based references (:tag)
		tagParts := strings.Split(lastPart, ":")
		imageName = tagParts[0]
		imageTag = tagParts[1]
	} else {
		// No tag specified
		imageName = lastPart
		imageTag = "latest" // Implicit latest tag
	}

	// Sanitize image name and tag
	imageName = sanitizeNameComponent(imageName)
	imageTag = sanitizeNameComponent(imageTag)

	// If empty after sanitization, use generic name
	if imageName == "" {
		imageName = "model"
	}
	if imageTag == "" {
		imageTag = "notag"
	}

	// Compute hash of full URI for uniqueness
	hash := sha256.Sum256([]byte(imageURI))
	hashSuffix := fmt.Sprintf("%x", hash[:])[:hashLength]

	// Build suffix: -<tag>-<hash>
	suffix := "-" + imageTag + "-" + hashSuffix

	// Calculate how much space is available for the image name
	maxImageNameLength := maxModelNameLength - len(suffix)
	if maxImageNameLength < 1 {
		maxImageNameLength = 1
	}

	// Truncate image name if needed
	if len(imageName) > maxImageNameLength {
		imageName = imageName[:maxImageNameLength]
		imageName = strings.TrimRight(imageName, "-")
	}

	return imageName + suffix
}

// sanitizeNameComponent sanitizes a name component for Kubernetes resource names
func sanitizeNameComponent(s string) string {
	s = strings.ToLower(s)
	s = invalidNameChars.ReplaceAllString(s, "-")
	s = multiDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
