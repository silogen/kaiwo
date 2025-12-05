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
	"strings"

	"github.com/blang/semver/v4"
	"github.com/google/go-containerregistry/pkg/name"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

const (
	// DockerRegistry is the canonical docker.io registry name
	DockerRegistry      = "docker.io"
	dockerIndexRegistry = "index.docker.io"
)

// parsedImageFilter represents a parsed image filter that may contain:
// - Just a repository pattern (e.g., "silogen/aim-*")
// - A full image URI with registry (e.g., "ghcr.io/silogen/aim-llama:1.0.0")
// - A repository with tag (e.g., "silogen/aim-llama:1.0.0")
type parsedImageFilter struct {
	// registry override from the filter (e.g., "ghcr.io"), empty if not specified
	registry string
	// repository pattern for matching (e.g., "silogen/aim-*")
	repository string
	// tag override from the filter (e.g., "1.0.0"), empty if not specified
	tag string
	// hasWildcard indicates if the repository pattern contains wildcards
	hasWildcard bool
}

// parseImageFilter parses an image filter string into its components.
// Supports multiple formats:
// - "repo/name" or "repo/name*" - repository pattern only
// - "repo/name:tag" - repository with specific tag
// - "registry.com/repo/name" - full URI with registry
// - "registry.com/repo/name:tag" - full URI with registry and tag
// - "registry.com/repo/name*" - registry with wildcard pattern
func parseImageFilter(imageFilter string) parsedImageFilter {
	parsed := parsedImageFilter{
		hasWildcard: strings.Contains(imageFilter, "*"),
	}

	// If it contains wildcards, we still try to extract registry if present
	// Check if it starts with a registry (contains a dot before first slash)
	if parsed.hasWildcard {
		// Try to extract registry from patterns like "ghcr.io/repo/*"
		firstSlash := strings.Index(imageFilter, "/")
		if firstSlash > 0 {
			potentialRegistry := imageFilter[:firstSlash]
			// If it looks like a registry (contains a dot), extract it
			if strings.Contains(potentialRegistry, ".") {
				parsed.registry = potentialRegistry
				parsed.repository = imageFilter[firstSlash+1:]
				return parsed
			}
		}
		// No registry found, treat entire string as repository pattern
		parsed.repository = imageFilter
		return parsed
	}

	// Try parsing as a full image reference (registry/repo:tag)
	// This will handle cases like:
	// - ghcr.io/silogen/aim-llama:1.0.0
	// - silogen/aim-llama:1.0.0
	// - ghcr.io/silogen/aim-llama
	ref, err := name.ParseReference(imageFilter)
	if err != nil {
		// If parsing fails, treat it as a simple repository pattern
		parsed.repository = imageFilter
		return parsed
	}

	// Extract registry (if not docker.io which is the default)
	registry := ref.Context().RegistryStr()
	if registry != dockerIndexRegistry && registry != DockerRegistry {
		parsed.registry = registry
	}

	// Extract repository (namespace/name without registry)
	// For docker.io/library/ubuntu, this gives "library/ubuntu"
	// For ghcr.io/org/repo, this gives "org/repo"
	repoName := ref.Context().RepositoryStr()
	// Remove registry prefix if present
	if parsed.registry != "" {
		repoName = strings.TrimPrefix(repoName, parsed.registry+"/")
	}
	parsed.repository = repoName

	// Extract tag if present (only for tagged references)
	// Skip "latest" which is the default added by the parser
	if tagged, ok := ref.(name.Tag); ok {
		tag := tagged.TagStr()
		// Only set tag if it was explicitly specified (not the default "latest")
		// We detect this by checking if the original string contains a colon
		if strings.Contains(imageFilter, ":") {
			parsed.tag = tag
		}
	}

	return parsed
}

// matchesWildcard checks if a string matches a pattern with * wildcard support.
// Only * is supported (matches any sequence of characters).
func matchesWildcard(pattern, str string) bool {
	// Split pattern by *
	parts := strings.Split(pattern, "*")

	// If no wildcards, must be exact match
	if len(parts) == 1 {
		return pattern == str
	}

	// Check prefix (before first *)
	if !strings.HasPrefix(str, parts[0]) {
		return false
	}
	str = str[len(parts[0]):]

	// Check suffix (after last *)
	if !strings.HasSuffix(str, parts[len(parts)-1]) {
		return false
	}
	str = str[:len(str)-len(parts[len(parts)-1])]

	// Check middle parts appear in order
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(str, parts[i])
		if idx == -1 {
			return false
		}
		str = str[idx+len(parts[i]):]
	}

	return true
}

// MatchesFilters checks if an image matches any of the provided filters.
// Filters are combined with OR logic - if any filter matches, the image is included.
func MatchesFilters(
	img RegistryImage,
	filters []aimv1alpha1.ModelSourceFilter,
	globalVersions []string,
) bool {
	for _, filter := range filters {
		if matchesFilter(img, filter, globalVersions) {
			return true
		}
	}
	return false
}

// matchesFilter checks if an image matches a single filter.
// The filter is applied in several stages:
// 1. Parse the filter to extract registry, repository pattern, and tag overrides
// 2. Check registry match (if filter specifies a registry)
// 3. Wildcard pattern matching on repository name
// 4. Exclusion list (exact match)
// 5. Tag/version constraints (exact tag or semver)
func matchesFilter(
	img RegistryImage,
	filter aimv1alpha1.ModelSourceFilter,
	globalVersions []string,
) bool {
	// Parse the filter to handle full image URIs
	parsed := parseImageFilter(filter.Image)

	// 1. Check registry match if filter specifies a registry
	// If the filter has a registry (e.g., "ghcr.io/repo/name"), only match images from that registry
	if parsed.registry != "" && parsed.registry != img.Registry {
		return false
	}

	// 2. Wildcard pattern match on repository name (only * supported)
	if !matchesWildcard(parsed.repository, img.Repository) {
		return false
	}

	// 3. Check exclusions (exact match on repository)
	for _, exclude := range filter.Exclude {
		if img.Repository == exclude {
			return false
		}
	}

	// 4. Tag/Version constraints
	// If the filter specifies an exact tag (e.g., "repo/name:1.0.0"), use that
	// Otherwise, use version constraints from the filter or global versions
	if parsed.tag != "" {
		// Exact tag match required
		match := img.Tag == parsed.tag
		return match
	}

	// Use filter-specific versions if provided, otherwise use global versions
	versionConstraints := filter.Versions
	if len(versionConstraints) == 0 {
		versionConstraints = globalVersions
	}

	// If version constraints are specified, apply them
	if len(versionConstraints) > 0 {
		result := matchesSemver(img.Tag, versionConstraints)
		return result
	}

	// No version constraints - all versions match
	return true
}

// matchesSemver checks if a tag satisfies all provided semver constraints.
// Returns false if the tag is not a valid semver string (non-semver tags are skipped).
// The 'v' prefix is stripped automatically (v1.0.0 -> 1.0.0).
func matchesSemver(tag string, constraints []string) bool {
	// Strip 'v' prefix if present
	tagVersion := strings.TrimPrefix(tag, "v")

	// Parse the tag as semver
	parsedVersion, err := semver.Parse(tagVersion)
	if err != nil {
		// Not a valid semver tag - skip it (strict semver-only mode)
		return false
	}

	// Check all constraints - all must be satisfied
	for _, constraint := range constraints {
		// Normalize constraint to ensure proper semver format
		// ">=0.9" becomes ">=0.9.0" to match semantic versioning expectations
		normalizedConstraint := normalizeConstraint(constraint)

		versionRange, err := semver.ParseRange(normalizedConstraint)
		if err != nil {
			// Invalid constraint - skip this constraint and continue
			// This allows the image to pass if other constraints are valid
			continue
		}

		// Check if version satisfies this constraint
		if !versionRange(parsedVersion) {
			return false
		}
	}

	return true
}

// normalizeConstraint ensures semver constraints have proper patch version
// ">=0.9" -> ">=0.9.0", ">=1.2" -> ">=1.2.0", etc.
func normalizeConstraint(constraint string) string {
	// Extract operator and version parts
	for i, c := range constraint {
		if c >= '0' && c <= '9' {
			operator := constraint[:i]
			version := strings.TrimSpace(constraint[i:])

			// Count dots in version string
			dots := strings.Count(version, ".")

			// Add missing patch version
			if dots == 1 {
				version += ".0"
			}

			return operator + version
		}
	}
	return constraint
}

// FilterHasWildcard checks if a filter contains wildcard patterns.
func FilterHasWildcard(filter aimv1alpha1.ModelSourceFilter) bool {
	return strings.Contains(filter.Image, "*")
}

// isVersionConstraint checks if a version string is a constraint (like ">=1.0.0")
// rather than an exact version (like "1.0.0" or "0.9.0-rc2").
// Returns true for constraints, false for exact versions.
func isVersionConstraint(version string) bool {
	// Check for common constraint operators
	if strings.HasPrefix(version, ">=") ||
		strings.HasPrefix(version, "<=") ||
		strings.HasPrefix(version, ">") ||
		strings.HasPrefix(version, "<") ||
		strings.HasPrefix(version, "~") ||
		strings.HasPrefix(version, "^") ||
		strings.Contains(version, " || ") ||
		strings.Contains(version, " - ") {
		return true
	}
	return false
}
