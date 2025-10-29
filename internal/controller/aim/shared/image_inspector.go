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
	ctrl "sigs.k8s.io/controller-runtime"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// ImageRegistryError wraps registry access errors with categorization
type ImageRegistryError struct {
	Type    ImagePullErrorType // From template.go
	Message string
	Cause   error
}

func (e *ImageRegistryError) Error() string {
	return e.Message
}

func (e *ImageRegistryError) Unwrap() error {
	return e.Cause
}

// categorizeRegistryError analyzes a registry error to determine its type
func categorizeRegistryError(err error) ImagePullErrorType {
	if err == nil {
		return ImagePullErrorGeneric
	}

	errMsg := strings.ToLower(err.Error())

	// Check for authentication/authorization errors
	authIndicators := []string{
		"unauthorized",
		"authentication required",
		"authentication failed",
		"401",
		"403",
		"forbidden",
		"denied",
		"permission denied",
		"access denied",
		"credentials",
		"authentication",
	}
	for _, indicator := range authIndicators {
		if strings.Contains(errMsg, indicator) {
			return ImagePullErrorAuth
		}
	}

	// Check for not-found errors
	notFoundIndicators := []string{
		"not found",
		"404",
		"manifest unknown",
		"name unknown",
		"image not found",
		"no such",
	}
	for _, indicator := range notFoundIndicators {
		if strings.Contains(errMsg, indicator) {
			return ImagePullErrorNotFound
		}
	}

	return ImagePullErrorGeneric
}

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
//     Registry access errors are wrapped in ImageRegistryError for categorization.
func InspectImage(
	ctx context.Context,
	imageURI string,
	imagePullSecrets []corev1.LocalObjectReference,
	clientset kubernetes.Interface,
	namespace string,
) (*aimv1alpha1.ImageMetadata, error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Inspecting image", "imageURI", imageURI)

	// Parse the image reference
	ref, err := name.ParseReference(imageURI)
	if err != nil {
		logger.Error(err, "Failed to parse image reference", "imageURI", imageURI)
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

		logger.Info("Using image pull secrets", "secrets", secretNames, "namespace", namespace)

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
		logger.Info("Using default keychain (no image pull secrets provided)")
		// Fall back to default keychain (uses docker config, etc.)
		keychain = authn.DefaultKeychain
	}

	// Fetch the image descriptor
	logger.Info("Fetching image descriptor from registry", "imageURI", imageURI)
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		errType := categorizeRegistryError(err)
		logger.Error(err, "Failed to fetch image descriptor",
			"imageURI", imageURI,
			"errorType", errType,
			"errorMessage", err.Error())
		return nil, &ImageRegistryError{
			Type:    errType,
			Message: fmt.Sprintf("failed to fetch image %q: %v", imageURI, err),
			Cause:   err,
		}
	}
	logger.Info("Successfully fetched image descriptor", "imageURI", imageURI, "digest", desc.Digest.String())

	// Get the image
	img, err := desc.Image()
	if err != nil {
		errType := categorizeRegistryError(err)
		logger.Error(err, "Failed to get image from descriptor", "imageURI", imageURI, "errorType", errType)
		return nil, &ImageRegistryError{
			Type:    errType,
			Message: fmt.Sprintf("failed to get image from descriptor: %v", err),
			Cause:   err,
		}
	}
	logger.Info("Successfully retrieved image", "imageURI", imageURI)

	// Get the config file which contains labels
	configFile, err := img.ConfigFile()
	if err != nil {
		errType := categorizeRegistryError(err)
		logger.Error(err, "Failed to get image config file", "imageURI", imageURI, "errorType", errType)
		return nil, &ImageRegistryError{
			Type:    errType,
			Message: fmt.Sprintf("failed to get image config: %v", err),
			Cause:   err,
		}
	}

	labelCount := len(configFile.Config.Labels)
	logger.Info("Successfully retrieved image config", "imageURI", imageURI, "labelCount", labelCount)

	// Log some key labels for debugging
	if labelCount > 0 {
		logger.Info("Image labels found",
			"canonicalName", configFile.Config.Labels["org.amd.silogen.model.canonicalName"],
			"hasRecommendedDeployments", configFile.Config.Labels["org.amd.silogen.model.recommendedDeployments"] != "")
	} else {
		logger.Info("No labels found in image config", "imageURI", imageURI)
	}

	// Extract metadata from labels
	metadata, err := parseImageLabels(configFile.Config.Labels)
	if err != nil {
		logger.Error(err, "Failed to parse image labels", "imageURI", imageURI, "labelCount", labelCount)
		return nil, fmt.Errorf("failed to parse image labels: %w", err)
	}

	logger.Info("Successfully extracted image metadata", "imageURI", imageURI,
		"canonicalName", metadata.Model.CanonicalName,
		"recommendedDeploymentCount", len(metadata.Model.RecommendedDeployments))

	return metadata, nil
}

// MetadataFormatError indicates the image metadata is malformed and cannot be processed.
type MetadataFormatError struct {
	Reason  string
	Message string
}

func (e *MetadataFormatError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "image metadata malformed"
}

func newMetadataFormatError(reason, message string) *MetadataFormatError {
	if reason == "" {
		reason = "MetadataMalformed"
	}
	return &MetadataFormatError{
		Reason:  reason,
		Message: message,
	}
}

// parseImageLabels extracts structured metadata from OCI and AMD Silogen image labels.
func parseImageLabels(labels map[string]string) (*aimv1alpha1.ImageMetadata, error) {
	// Allow images with no labels - metadata will be minimal but valid
	// Downstream logic will detect missing recommendedDeployments if needed

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

	// Note: canonicalName is optional. If missing, downstream logic will handle it.
	// This allows images without full metadata to be processed gracefully.

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

// parseRecommendedDeployments parses deployment configurations from a label value.
// The input can be either:
// - A JSON array: [{"gpuModel": "MI300X", "gpuCount": 1, ...}, {...}]
// - Comma-separated objects: {"gpuModel": "MI300X", "gpuCount": 1, ...}, {...}
// - Python-style dicts with single quotes: {'gpuModel': 'MI300X', 'gpuCount': 1, ...}
//
// This function normalizes Python-style notation to JSON before parsing.
func parseRecommendedDeployments(jsonStr string) ([]aimv1alpha1.RecommendedDeployment, error) {
	var deployments []aimv1alpha1.RecommendedDeployment

	// Normalize the input:
	// 1. Replace single quotes with double quotes (Python -> JSON)
	// 2. Wrap with brackets if it's comma-separated objects
	trimmed := strings.TrimSpace(jsonStr)

	// Replace single quotes with double quotes for Python-style dict notation
	normalized := strings.ReplaceAll(trimmed, "'", "\"")

	// If the string doesn't start with '[', assume it's comma-separated objects
	// and wrap it with brackets to make it a valid JSON array
	if !strings.HasPrefix(normalized, "[") {
		normalized = "[" + normalized + "]"
	}

	// Parse as JSON array
	err := json.Unmarshal([]byte(normalized), &deployments)
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
