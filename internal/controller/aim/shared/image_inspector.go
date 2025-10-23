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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// InspectImage extracts metadata from a container image using the provided image pull secrets.
// It uses go-containerregistry to authenticate and fetch image labels, then parses them into
// the ImageMetadata structure.
//
// Parameters:
//   - ctx: Context for the operation
//   - imageURI: Full container image reference (e.g., "registry.example.com/repo/image:tag")
//   - imagePullSecrets: Kubernetes image pull secrets for authentication
//   - clientset: Kubernetes clientset for accessing secrets
//   - namespace: Namespace where the secrets are located
//
// Returns:
//   - *ImageMetadata: Extracted metadata if successful
//   - error: Any error encountered during inspection (authentication, network, parsing, etc.)
func InspectImage(
	ctx context.Context,
	imageURI string,
	imagePullSecrets []corev1.LocalObjectReference,
	clientset kubernetes.Interface,
	namespace string,
) (*aimv1alpha1.ImageMetadata, error) {
	// Parse the image reference
	ref, err := name.ParseReference(imageURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageURI, err)
	}

	// Build keychain for authentication
	var keychain authn.Keychain
	if clientset != nil && namespace != "" && len(imagePullSecrets) > 0 {
		// Convert LocalObjectReference to secret names
		secretNames := make([]string, len(imagePullSecrets))
		for i, secret := range imagePullSecrets {
			secretNames[i] = secret.Name
		}

		// Create k8s keychain with the provided secrets
		kc, err := k8schain.New(ctx, clientset, k8schain.Options{
			Namespace:          namespace,
			ImagePullSecrets:   secretNames,
			ServiceAccountName: "",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create k8s keychain: %w", err)
		}
		keychain = kc
	} else {
		// Fall back to default keychain (uses docker config, etc.)
		keychain = authn.DefaultKeychain
	}

	// Fetch the image descriptor
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image %q: %w", imageURI, err)
	}

	// Get the image
	img, err := desc.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to get image from descriptor: %w", err)
	}

	// Get the config file which contains labels
	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get image config: %w", err)
	}

	// Extract metadata from labels
	metadata, err := parseImageLabels(configFile.Config.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image labels: %w", err)
	}

	return metadata, nil
}

// parseImageLabels extracts structured metadata from OCI and AMD Silogen image labels.
func parseImageLabels(labels map[string]string) (*aimv1alpha1.ImageMetadata, error) {
	if len(labels) == 0 {
		return nil, fmt.Errorf("no labels found in image")
	}

	metadata := &aimv1alpha1.ImageMetadata{
		OCI:   &aimv1alpha1.OCIMetadata{},
		Model: &aimv1alpha1.ModelMetadata{},
	}

	// Parse OCI standard labels
	metadata.OCI.Title = labels["org.opencontainers.image.title"]
	metadata.OCI.Description = labels["org.opencontainers.image.description"]
	metadata.OCI.Licenses = labels["org.opencontainers.image.licenses"]
	metadata.OCI.Vendor = labels["org.opencontainers.image.vendor"]
	metadata.OCI.Authors = labels["org.opencontainers.image.authors"]
	metadata.OCI.Source = labels["org.opencontainers.image.source"]
	metadata.OCI.Documentation = labels["org.opencontainers.image.documentation"]
	metadata.OCI.Created = labels["org.opencontainers.image.created"]
	metadata.OCI.Revision = labels["org.opencontainers.image.revision"]
	metadata.OCI.Version = labels["org.opencontainers.image.version"]

	// Parse AMD Silogen model labels
	metadata.Model.CanonicalName = labels["org.amd.silogen.model.canonicalName"]
	metadata.Model.Source = labels["org.amd.silogen.model.source"]
	metadata.Model.Title = labels["org.amd.silogen.title"]
	metadata.Model.DescriptionFull = labels["org.amd.silogen.description.full"]
	metadata.Model.ReleaseNotes = labels["org.amd.silogen.release.notes"]

	// Parse comma-separated fields
	if tags := labels["org.amd.silogen.model.tags"]; tags != "" {
		metadata.Model.Tags = parseCommaSeparated(tags)
	}
	if versions := labels["org.amd.silogen.model.versions"]; versions != "" {
		metadata.Model.Versions = parseCommaSeparated(versions)
	}
	if variants := labels["org.amd.silogen.model.variants"]; variants != "" {
		metadata.Model.Variants = parseCommaSeparated(variants)
	}

	// Parse boolean HF token required
	if hfTokenStr := labels["org.amd.silogen.hfToken.required"]; hfTokenStr != "" {
		hfTokenRequired, err := strconv.ParseBool(hfTokenStr)
		if err == nil {
			metadata.Model.HFTokenRequired = hfTokenRequired
		}
	}

	// Parse recommended deployments JSON array
	if deploymentsJSON := labels["org.amd.silogen.model.recommendedDeployments"]; deploymentsJSON != "" {
		deployments, err := parseRecommendedDeployments(deploymentsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to parse recommended deployments: %w", err)
		}
		metadata.Model.RecommendedDeployments = deployments
	}

	return metadata, nil
}

// parseCommaSeparated splits a comma-separated string into a slice of trimmed strings.
func parseCommaSeparated(input string) []string {
	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseRecommendedDeployments parses a JSON array of deployment configurations.
// The input is expected to be a JSON array of objects like:
// [{"gpuModel": "MI300X", "gpuCount": 1, "precision": "fp8", "metric": "latency", "description": "..."}]
func parseRecommendedDeployments(jsonStr string) ([]aimv1alpha1.RecommendedDeployment, error) {
	// The label might be formatted as a Python dict string, try to handle both JSON and Python-style
	// For now, we'll parse it as JSON array
	var deployments []aimv1alpha1.RecommendedDeployment

	// Try to parse as JSON array
	err := json.Unmarshal([]byte(jsonStr), &deployments)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployments as JSON array: %w", err)
	}

	return deployments, nil
}

// GetImageConfigLabels is a helper function that retrieves just the labels from an image
// without parsing them into structured metadata. Useful for debugging.
func GetImageConfigLabels(ctx context.Context, imageURI string, keychain authn.Keychain) (map[string]string, error) {
	ref, err := name.ParseReference(imageURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference: %w", err)
	}

	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	img, err := desc.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file: %w", err)
	}

	return configFile.Config.Labels, nil
}
