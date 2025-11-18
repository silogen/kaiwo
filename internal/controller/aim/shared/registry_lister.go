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
	"fmt"
	"path"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ListRepositories lists all repositories from a container registry.
// Note: Not all registries support the catalog endpoint. Some (like Docker Hub)
// may require additional permissions or may not support it at all.
//
// Parameters:
//   - ctx: Context for the operation
//   - registryURL: Registry hostname (e.g., "ghcr.io", "docker.io")
//   - clientset: Kubernetes clientset for accessing secrets
//   - namespace: Namespace where secrets are located
//   - imagePullSecrets: Kubernetes image pull secrets for authentication
//
// Returns:
//   - []string: List of repository names
//   - error: Any error encountered while listing repositories
func ListRepositories(
	ctx context.Context,
	registryURL string,
	clientset kubernetes.Interface,
	namespace string,
	imagePullSecrets []corev1.LocalObjectReference,
) ([]string, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Build keychain for authentication
	keychain, err := BuildKeychain(ctx, clientset, namespace, imagePullSecrets)
	if err != nil {
		return nil, err
	}

	// Parse registry reference
	registry, err := name.NewRegistry(registryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse registry %q: %w", registryURL, err)
	}

	logger.V(1).Info("Listing repositories from registry", "registry", registryURL)

	// List repositories using the catalog endpoint
	repos, err := remote.Catalog(ctx, registry, remote.WithAuthFromKeychain(keychain))
	if err != nil {
		errType := categorizeRegistryError(err)
		if errType == ImagePullErrorAuth || errType == ImagePullErrorNotFound {
			logger.Info("Failed to list repositories from registry",
				"registry", registryURL,
				"errorType", errType,
				"error", err.Error())
		} else {
			logger.Error(err, "Failed to list repositories from registry",
				"registry", registryURL,
				"errorType", errType)
		}
		return nil, &ImageRegistryError{
			Type:    errType,
			Message: fmt.Sprintf("failed to list repositories from registry %q: %v", registryURL, err),
			Cause:   err,
		}
	}

	logger.V(1).Info("Successfully listed repositories", "registry", registryURL, "count", len(repos))
	return repos, nil
}

// ListTags lists all tags for a given repository in a container registry.
//
// Parameters:
//   - ctx: Context for the operation
//   - repositoryName: Full repository name (e.g., "ghcr.io/silogen/aim-models/llama")
//   - clientset: Kubernetes clientset for accessing secrets
//   - namespace: Namespace where secrets are located
//   - imagePullSecrets: Kubernetes image pull secrets for authentication
//
// Returns:
//   - []string: List of tag names
//   - error: Any error encountered while listing tags
func ListTags(
	ctx context.Context,
	repositoryName string,
	clientset kubernetes.Interface,
	namespace string,
	imagePullSecrets []corev1.LocalObjectReference,
) ([]string, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Build keychain for authentication
	keychain, err := BuildKeychain(ctx, clientset, namespace, imagePullSecrets)
	if err != nil {
		return nil, err
	}

	// Parse repository reference
	repo, err := name.NewRepository(repositoryName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository %q: %w", repositoryName, err)
	}

	logger.V(1).Info("Listing tags for repository", "repository", repositoryName)

	// List tags
	tags, err := remote.List(repo, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		errType := categorizeRegistryError(err)
		if errType == ImagePullErrorAuth || errType == ImagePullErrorNotFound {
			logger.Info("Failed to list tags for repository",
				"repository", repositoryName,
				"errorType", errType,
				"error", err.Error())
		} else {
			logger.Error(err, "Failed to list tags for repository",
				"repository", repositoryName,
				"errorType", errType)
		}
		return nil, &ImageRegistryError{
			Type:    errType,
			Message: fmt.Sprintf("failed to list tags for repository %q: %v", repositoryName, err),
			Cause:   err,
		}
	}

	logger.V(1).Info("Successfully listed tags", "repository", repositoryName, "count", len(tags))
	return tags, nil
}

// MatchesPattern checks if a string matches a glob-style wildcard pattern.
// Supports '*' as a wildcard that matches any characters (including none).
//
// Examples:
//   - MatchesPattern("aim-models/llama-3", "aim-models/*") -> true
//   - MatchesPattern("aim-models/llama-3", "llama-*") -> false
//   - MatchesPattern("llama-3-8b", "llama-*") -> true
//   - MatchesPattern("mistral/mixtral-8x22b", "*/mixtral-*") -> true
func MatchesPattern(str, pattern string) bool {
	// Use path.Match for glob-style matching
	// path.Match uses '/' as separator but also works for general strings
	matched, err := path.Match(pattern, str)
	if err != nil {
		// Pattern is invalid, treat as no match
		return false
	}
	return matched
}

// FilterByPattern filters a list of strings based on include and exclude patterns.
// Include patterns are processed first, then exclude patterns are applied to the results.
//
// Parameters:
//   - items: List of strings to filter
//   - includePatterns: Patterns that items must match (empty means include all)
//   - excludePatterns: Patterns to exclude from results
//
// Returns:
//   - []string: Filtered list of strings
func FilterByPattern(items []string, includePatterns, excludePatterns []string) []string {
	var result []string

	for _, item := range items {
		// Check include patterns
		included := len(includePatterns) == 0 // If no include patterns, include all by default
		for _, pattern := range includePatterns {
			if MatchesPattern(item, pattern) {
				included = true
				break
			}
		}

		if !included {
			continue
		}

		// Check exclude patterns
		excluded := false
		for _, pattern := range excludePatterns {
			if MatchesPattern(item, pattern) {
				excluded = true
				break
			}
		}

		if !excluded {
			result = append(result, item)
		}
	}

	return result
}

// FilterTagsBySemver filters tags based on semantic version constraints.
// Non-semver tags are skipped (not included in the result).
//
// Parameters:
//   - tags: List of tag names
//   - constraints: List of semver constraint strings (e.g., ">=1.0.0", "~1.2.0")
//     Empty list means include all valid semver tags
//
// Returns:
//   - []string: Filtered list of tags that match the constraints
//   - error: Error if any constraint string is invalid
func FilterTagsBySemver(tags []string, constraints []string) ([]string, error) {
	// If no constraints, include all valid semver tags
	if len(constraints) == 0 {
		var result []string
		for _, tag := range tags {
			// Try to parse as semver, include if valid
			tag = strings.TrimPrefix(tag, "v") // Handle v1.2.3 format
			if _, err := semver.NewVersion(tag); err == nil {
				result = append(result, tag)
			}
		}
		return result, nil
	}

	// Parse all constraints
	var parsedConstraints []*semver.Constraints
	for _, c := range constraints {
		constraint, err := semver.NewConstraint(c)
		if err != nil {
			return nil, fmt.Errorf("invalid semver constraint %q: %w", c, err)
		}
		parsedConstraints = append(parsedConstraints, constraint)
	}

	// Filter tags
	var result []string
	for _, tag := range tags {
		// Try to parse tag as semver
		tagWithoutV := strings.TrimPrefix(tag, "v")
		version, err := semver.NewVersion(tagWithoutV)
		if err != nil {
			// Skip non-semver tags
			continue
		}

		// Check if version matches any constraint
		matches := false
		for _, constraint := range parsedConstraints {
			if constraint.Check(version) {
				matches = true
				break
			}
		}

		if matches {
			result = append(result, tag)
		}
	}

	return result, nil
}
