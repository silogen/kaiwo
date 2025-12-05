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

package aimclustermodelsource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	dockerHost = "docker.io"
	ghcrHost   = "ghcr.io"
)

// RegistryClient provides methods for listing images from container registries.
type RegistryClient struct {
	clientset       kubernetes.Interface
	secretNamespace string
	httpClient      *http.Client
}

// NewRegistryClient creates a new registry client.
func NewRegistryClient(clientset kubernetes.Interface, secretNamespace string) *RegistryClient {
	return &RegistryClient{
		clientset:       clientset,
		secretNamespace: secretNamespace,
		httpClient:      &http.Client{},
	}
}

// ListImages discovers all images matching the spec's filters from the configured registry.
func (rc *RegistryClient) ListImages(
	ctx context.Context,
	spec aimv1alpha1.AIMClusterModelSourceSpec,
) ([]RegistryImage, error) {
	reg := spec.Registry
	if reg == "" {
		reg = dockerHost
	}

	// Route to appropriate implementation based on registry
	if reg == dockerHost || strings.Contains(reg, "hub.docker.com") {
		return rc.listDockerHubImages(ctx, spec)
	}
	if reg == ghcrHost || strings.Contains(reg, "ghcr.io") {
		return rc.listGitHubContainerRegistryImages(ctx, spec)
	}
	return rc.listRegistryV2Images(ctx, spec)
}

// listDockerHubImages uses the Docker Hub API to list repositories and tags.
func (rc *RegistryClient) listDockerHubImages(
	ctx context.Context,
	spec aimv1alpha1.AIMClusterModelSourceSpec,
) ([]RegistryImage, error) {
	var allImages []RegistryImage

	// Extract namespaces from filter patterns
	namespaces := extractDockerHubNamespaces(spec.Filters)

	for _, namespace := range namespaces {
		// Fetch repositories for this namespace
		repos, err := rc.fetchDockerHubRepositories(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch repos for namespace %s: %w", namespace, err)
		}

		// For each repository, check if it matches any filter pattern
		for _, repo := range repos {
			// Check if this repository matches any filter pattern
			matchesAnyFilter := false
			for _, filter := range spec.Filters {
				// Parse filter to get the repository pattern
				parsed := parseImageFilter(filter.Image)
				// Match against the full repo path
				if matchesWildcard(parsed.repository, repo) {
					matchesAnyFilter = true
					break
				}
			}

			if !matchesAnyFilter {
				continue
			}

			tags, err := rc.fetchImageTags(ctx, repo, spec.ImagePullSecrets)
			if err != nil {
				// Log but continue - some repos might be inaccessible
				continue
			}

			// Filter tags by version constraints and exclusions
			for _, tag := range tags {
				img := RegistryImage{
					Registry:   dockerHost,
					Repository: repo,
					Tag:        tag,
				}

				// Check if this tag matches version constraints and exclusions
				if MatchesFilters(img, spec.Filters, spec.Versions) {
					allImages = append(allImages, img)
				}
			}
		}
	}

	return allImages, nil
}

// extractDockerHubNamespaces extracts unique namespaces from filter patterns.
// For example, "amdenterpriseai/aim-*" -> "amdenterpriseai"
func extractDockerHubNamespaces(filters []aimv1alpha1.ModelSourceFilter) []string {
	nsMap := make(map[string]bool)

	for _, filter := range filters {
		// Split by / to get namespace
		parts := strings.Split(filter.Image, "/")
		if len(parts) >= 1 {
			// Remove wildcards from namespace
			namespace := strings.TrimRight(parts[0], "*")
			if namespace != "" && namespace != "*" {
				nsMap[namespace] = true
			}
		}
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}
	return namespaces
}

// fetchDockerHubRepositories fetches all repositories for a namespace using Docker Hub API.
func (rc *RegistryClient) fetchDockerHubRepositories(ctx context.Context, namespace string) ([]string, error) {
	var repos []string
	nextURL := fmt.Sprintf("https://hub.docker.com/v2/namespaces/%s/repositories", namespace)

	for nextURL != "" {
		var result struct {
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
			Next string `json:"next"`
		}

		if err := rc.fetchJSON(ctx, nextURL, &result); err != nil {
			return nil, err
		}

		for _, r := range result.Results {
			repos = append(repos, fmt.Sprintf("%s/%s", namespace, r.Name))
		}

		nextURL = result.Next
	}

	return repos, nil
}

// fetchJSON performs an HTTP GET and decodes the JSON response.
func (rc *RegistryClient) fetchJSON(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return nil
}

// fetchImageTags fetches all tags for a repository using go-containerregistry.
func (rc *RegistryClient) fetchImageTags(
	ctx context.Context,
	repository string,
	imagePullSecrets []corev1.LocalObjectReference,
) ([]string, error) {
	keychain, err := baseutils.BuildKeychain(ctx, rc.clientset, rc.secretNamespace, imagePullSecrets)
	if err != nil {
		return nil, err
	}

	repoRef, err := name.NewRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("invalid repository %s: %w", repository, err)
	}

	tags, err := remote.List(repoRef, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for %s: %w", repository, err)
	}

	return tags, nil
}

// listRegistryV2Images uses the Registry v2 API to list repositories and tags.
func (rc *RegistryClient) listRegistryV2Images(
	ctx context.Context,
	spec aimv1alpha1.AIMClusterModelSourceSpec,
) ([]RegistryImage, error) {
	var allImages []RegistryImage

	// Build keychain
	keychain, err := baseutils.BuildKeychain(ctx, rc.clientset, rc.secretNamespace, spec.ImagePullSecrets)
	if err != nil {
		return nil, err
	}

	// Get catalog
	registryRef, err := name.NewRegistry(spec.Registry)
	if err != nil {
		return nil, fmt.Errorf("invalid registry %s: %w", spec.Registry, err)
	}

	repos, err := remote.Catalog(ctx, registryRef, remote.WithAuthFromKeychain(keychain))
	if err != nil {
		return nil, fmt.Errorf("failed to list catalog for %s: %w", spec.Registry, err)
	}

	// For each repository, check if it matches any filter pattern
	for _, repo := range repos {
		// Check if this repository matches any filter pattern
		matchesAnyFilter := false
		for _, filter := range spec.Filters {
			// Parse filter to get the repository pattern
			parsed := parseImageFilter(filter.Image)
			// Match against the repo path (without registry prefix)
			if matchesWildcard(parsed.repository, repo) {
				matchesAnyFilter = true
				break
			}
		}

		if !matchesAnyFilter {
			continue
		}

		fullRepo := fmt.Sprintf("%s/%s", spec.Registry, repo)
		tags, err := rc.fetchImageTags(ctx, fullRepo, spec.ImagePullSecrets)
		if err != nil {
			// Log but continue - some repos might be inaccessible
			continue
		}

		// Filter tags by version constraints and exclusions
		for _, tag := range tags {
			img := RegistryImage{
				Registry:   spec.Registry,
				Repository: repo,
				Tag:        tag,
			}

			// Check if this tag matches version constraints and exclusions
			if MatchesFilters(img, spec.Filters, spec.Versions) {
				allImages = append(allImages, img)
			}
		}
	}

	return allImages, nil
}

// listGitHubContainerRegistryImages uses the GitHub API to list packages for organizations.
// GHCR does not support the Docker catalog API, so we use GitHub's REST API instead.
func (rc *RegistryClient) listGitHubContainerRegistryImages(
	ctx context.Context,
	spec aimv1alpha1.AIMClusterModelSourceSpec,
) ([]RegistryImage, error) {
	var allImages []RegistryImage

	// Extract organizations from filter patterns
	orgs := extractGitHubOrgs(spec.Filters)
	if len(orgs) == 0 {
		return nil, fmt.Errorf("no GitHub organizations found in filters")
	}

	// Extract GitHub token from image pull secrets
	token, err := rc.extractGitHubToken(ctx, spec.ImagePullSecrets)
	if err != nil {
		return nil, fmt.Errorf("failed to extract GitHub token: %w", err)
	}

	// For each organization, fetch packages
	for _, org := range orgs {
		packages, err := rc.fetchGitHubPackages(ctx, token, org)
		if err != nil {
			// Log but continue - some orgs might be inaccessible
			continue
		}

		// For each package, check if it matches any filter pattern
		for _, pkg := range packages {
			fullRepo := fmt.Sprintf("%s/%s", org, pkg)

			// Check if this package matches any filter pattern
			matchesAnyFilter := false
			for _, filter := range spec.Filters {
				// Parse filter to get the repository pattern
				parsed := parseImageFilter(filter.Image)
				// Match against the full repo path (org/package)
				if matchesWildcard(parsed.repository, fullRepo) {
					matchesAnyFilter = true
					break
				}
			}

			if !matchesAnyFilter {
				continue
			}

			fullRepoWithRegistry := fmt.Sprintf("%s/%s", ghcrHost, fullRepo)
			tags, err := rc.fetchImageTags(ctx, fullRepoWithRegistry, spec.ImagePullSecrets)
			if err != nil {
				// Log but continue - some packages might be inaccessible
				continue
			}

			// Filter tags by version constraints
			for _, tag := range tags {
				img := RegistryImage{
					Registry:   ghcrHost,
					Repository: fullRepo,
					Tag:        tag,
				}

				// Check if this tag matches version constraints
				if MatchesFilters(img, spec.Filters, spec.Versions) {
					allImages = append(allImages, img)
				}
			}
		}
	}

	return allImages, nil
}

// extractGitHubOrgs extracts unique GitHub organizations from filter patterns.
// For example, "silogen/aim-*" -> "silogen"
func extractGitHubOrgs(filters []aimv1alpha1.ModelSourceFilter) []string {
	orgMap := make(map[string]bool)

	for _, filter := range filters {
		// Remove leading slash if present (can happen with some parsing)
		image := strings.TrimPrefix(filter.Image, "/")

		// Split by / to get org
		parts := strings.Split(image, "/")
		if len(parts) >= 1 {
			// Remove wildcards and registry prefix
			org := strings.TrimRight(parts[0], "*")
			// Skip if it looks like a registry (contains a dot) or is empty/wildcard
			if org != "" && org != "*" && !strings.Contains(org, ".") {
				orgMap[org] = true
			}
		}
	}

	orgs := make([]string, 0, len(orgMap))
	for org := range orgMap {
		orgs = append(orgs, org)
	}
	return orgs
}

// extractGitHubToken extracts the GitHub token from Kubernetes image pull secrets.
// It looks for credentials for ghcr.io in the Docker config JSON.
func (rc *RegistryClient) extractGitHubToken(ctx context.Context, imagePullSecrets []corev1.LocalObjectReference) (string, error) {
	if rc.clientset == nil || rc.secretNamespace == "" || len(imagePullSecrets) == 0 {
		return "", fmt.Errorf("no image pull secrets configured")
	}

	// Try each secret until we find one with ghcr.io credentials
	for _, secretRef := range imagePullSecrets {
		secret, err := rc.clientset.CoreV1().Secrets(rc.secretNamespace).Get(ctx, secretRef.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		// Parse .dockerconfigjson
		dockerConfigJSON, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}

		var dockerConfig struct {
			Auths map[string]struct {
				Auth     string `json:"auth"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"auths"`
		}

		if err := json.Unmarshal(dockerConfigJSON, &dockerConfig); err != nil {
			continue
		}

		// Look for ghcr.io credentials
		ghcrAuth, ok := dockerConfig.Auths[ghcrHost]
		if !ok {
			continue
		}

		// If password is set directly, use it
		if ghcrAuth.Password != "" {
			return ghcrAuth.Password, nil
		}

		// Otherwise decode base64 auth string (format: "username:password")
		if ghcrAuth.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(ghcrAuth.Auth)
			if err != nil {
				continue
			}

			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("no ghcr.io credentials found in image pull secrets")
}

// fetchGitHubPackages fetches all container packages for a GitHub organization using the GitHub API.
func (rc *RegistryClient) fetchGitHubPackages(ctx context.Context, token, org string) ([]string, error) {
	var packages []string
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/orgs/%s/packages?package_type=container&per_page=%d&page=%d",
			org, perPage, page)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := rc.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch packages: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("unexpected status %d from GitHub API", resp.StatusCode)
		}

		var result []struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to decode JSON response: %w", err)
		}
		_ = resp.Body.Close()

		for _, pkg := range result {
			packages = append(packages, pkg.Name)
		}

		// If we got fewer than perPage results, we're done
		if len(result) < perPage {
			break
		}

		page++
	}

	return packages, nil
}

// FetchImagesUsingTagsList queries specific repositories using the tags list API when:
// - Filters have exact repository names (no wildcards)
// - Version constraints are specified (can be ranges)
// This allows version ranges to work on registries like ghcr.io that don't support catalog API.
func FetchImagesUsingTagsList(ctx context.Context, client *RegistryClient, spec aimv1alpha1.AIMClusterModelSourceSpec) []RegistryImage {
	var allImages []RegistryImage

	// Determine the registry to use
	registry := spec.Registry
	if registry == "" {
		registry = dockerHost
	}

	for _, filter := range spec.Filters {
		// Parse filter to check if it's suitable for tags list API
		parsed := parseImageFilter(filter.Image)

		// Skip wildcards - these need catalog API
		if parsed.hasWildcard {
			continue
		}

		// Skip if explicit tag is already in the filter - handled by ExtractStaticImages
		if parsed.tag != "" {
			continue
		}

		// Determine which versions to use
		versions := filter.Versions
		if len(versions) == 0 {
			versions = spec.Versions
		}

		// Skip if no version constraints - would need catalog to discover tags
		if len(versions) == 0 {
			continue
		}
		// Determine the registry for this filter
		filterRegistry := parsed.registry
		if filterRegistry == "" {
			filterRegistry = registry
		}

		// Build full repository reference
		var fullRepo string
		if filterRegistry == dockerHost {
			fullRepo = parsed.repository
		} else {
			fullRepo = fmt.Sprintf("%s/%s", filterRegistry, parsed.repository)
		}

		// Fetch all tags for this repository
		tags, err := client.fetchImageTags(ctx, fullRepo, spec.ImagePullSecrets)
		if err != nil {
			// Failed to fetch tags - skip this filter
			continue
		}

		// Filter tags by version constraints and build RegistryImage list
		matchedCount := 0
		for _, tag := range tags {
			img := RegistryImage{
				Registry:   filterRegistry,
				Repository: parsed.repository,
				Tag:        tag,
			}

			// Check if this tag matches the version constraints and other filter criteria
			matches := MatchesFilters(img, []aimv1alpha1.ModelSourceFilter{filter}, versions)
			if matches {
				allImages = append(allImages, img)
				matchedCount++
			}
		}
	}

	return allImages
}
