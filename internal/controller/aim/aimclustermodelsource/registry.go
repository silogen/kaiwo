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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
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
		reg = "docker.io"
	}

	// Route to appropriate implementation based on registry
	if reg == "docker.io" || strings.Contains(reg, "hub.docker.com") {
		return rc.listDockerHubImages(ctx, spec)
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

		// For each repository, fetch tags
		for _, repo := range repos {
			tags, err := rc.fetchImageTags(ctx, repo, spec.ImagePullSecrets)
			if err != nil {
				// Log but continue - some repos might be inaccessible
				continue
			}

			for _, tag := range tags {
				allImages = append(allImages, RegistryImage{
					Registry:   "docker.io",
					Repository: repo,
					Tag:        tag,
				})
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

	// For each repository, fetch tags
	for _, repo := range repos {
		fullRepo := fmt.Sprintf("%s/%s", spec.Registry, repo)
		tags, err := rc.fetchImageTags(ctx, fullRepo, spec.ImagePullSecrets)
		if err != nil {
			// Log but continue - some repos might be inaccessible
			continue
		}

		for _, tag := range tags {
			allImages = append(allImages, RegistryImage{
				Registry:   spec.Registry,
				Repository: repo,
				Tag:        tag,
			})
		}
	}

	return allImages, nil
}
