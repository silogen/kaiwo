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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		pattern  string
		expected bool
	}{
		// Basic wildcard matching
		{
			name:     "exact match",
			str:      "aim-llama",
			pattern:  "aim-llama",
			expected: true,
		},
		{
			name:     "wildcard at end",
			str:      "aim-llama-3",
			pattern:  "aim-*",
			expected: true,
		},
		{
			name:     "wildcard at start",
			str:      "meta-llama-3",
			pattern:  "*-llama-3",
			expected: true,
		},
		{
			name:     "wildcard in middle",
			str:      "aim-meta-llama-3",
			pattern:  "aim-*-3",
			expected: true,
		},
		{
			name:     "multiple wildcards",
			str:      "aim-meta-llama-3-8b",
			pattern:  "aim-*-*-8b",
			expected: true,
		},

		// Path-like patterns
		{
			name:     "repository with slash - match all",
			str:      "amdenterpriseai/aim-llama",
			pattern:  "amdenterpriseai/*",
			expected: true,
		},
		{
			name:     "repository with slash - specific pattern",
			str:      "amdenterpriseai/aim-llama-3",
			pattern:  "amdenterpriseai/aim-*",
			expected: true,
		},
		{
			name:     "repository with slash - wildcard namespace",
			str:      "amdenterpriseai/mixtral-8x22b",
			pattern:  "*/mixtral-*",
			expected: true,
		},

		// Non-matches
		{
			name:     "no match - different prefix",
			str:      "meta-llama",
			pattern:  "aim-*",
			expected: false,
		},
		{
			name:     "no match - different suffix",
			str:      "aim-llama-3",
			pattern:  "*-mistral",
			expected: false,
		},
		{
			name:     "no match - wrong namespace",
			str:      "other/aim-llama",
			pattern:  "amdenterpriseai/*",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty pattern matches only empty string",
			str:      "anything",
			pattern:  "",
			expected: false,
		},
		{
			name:     "empty string with wildcard pattern",
			str:      "",
			pattern:  "*",
			expected: true,
		},
		{
			name:     "single wildcard matches anything",
			str:      "literally-anything-here",
			pattern:  "*",
			expected: true,
		},

		// Multiple dashes (for base image exclusion)
		{
			name:     "multiple dashes required - matches",
			str:      "aim-meta-llama",
			pattern:  "aim-*-*",
			expected: true,
		},
		{
			name:     "multiple dashes required - doesn't match single dash",
			str:      "aim-base",
			pattern:  "aim-*-*",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesPattern(tt.str, tt.pattern)
			assert.Equal(t, tt.expected, result, "MatchesPattern(%q, %q)", tt.str, tt.pattern)
		})
	}
}

func TestFilterByPattern(t *testing.T) {
	tests := []struct {
		name            string
		items           []string
		includePatterns []string
		excludePatterns []string
		expected        []string
	}{
		{
			name: "include all aim models",
			items: []string{
				"amdenterpriseai/aim-llama-3",
				"amdenterpriseai/aim-mistral",
				"amdenterpriseai/test",
				"amdenterpriseai/aim-base",
			},
			includePatterns: []string{"amdenterpriseai/aim-*"},
			excludePatterns: nil,
			expected: []string{
				"amdenterpriseai/aim-llama-3",
				"amdenterpriseai/aim-mistral",
				"amdenterpriseai/aim-base",
			},
		},
		{
			name: "include all aim models except base",
			items: []string{
				"amdenterpriseai/aim-llama-3",
				"amdenterpriseai/aim-mistral",
				"amdenterpriseai/test",
				"amdenterpriseai/aim-base",
			},
			includePatterns: []string{"amdenterpriseai/aim-*"},
			excludePatterns: []string{"amdenterpriseai/aim-base"},
			expected: []string{
				"amdenterpriseai/aim-llama-3",
				"amdenterpriseai/aim-mistral",
			},
		},
		{
			name: "multiple include patterns",
			items: []string{
				"amdenterpriseai/aim-meta-llama",
				"amdenterpriseai/aim-mistralai-mistral",
				"amdenterpriseai/aim-qwen-qwen3",
				"amdenterpriseai/aim-base",
				"amdenterpriseai/test",
			},
			includePatterns: []string{
				"amdenterpriseai/aim-meta-*",
				"amdenterpriseai/aim-mistralai-*",
			},
			excludePatterns: nil,
			expected: []string{
				"amdenterpriseai/aim-meta-llama",
				"amdenterpriseai/aim-mistralai-mistral",
			},
		},
		{
			name: "multiple exclude patterns",
			items: []string{
				"amdenterpriseai/aim-model-v1",
				"amdenterpriseai/aim-model-v2-dev",
				"amdenterpriseai/aim-model-v3-experimental",
				"amdenterpriseai/aim-model-v4",
			},
			includePatterns: []string{"amdenterpriseai/aim-*"},
			excludePatterns: []string{"amdenterpriseai/*-dev", "amdenterpriseai/*-experimental"},
			expected: []string{
				"amdenterpriseai/aim-model-v1",
				"amdenterpriseai/aim-model-v4",
			},
		},
		{
			name: "no include patterns - include all",
			items: []string{
				"repo1/image1",
				"repo2/image2",
				"repo3/image3",
			},
			includePatterns: nil,
			excludePatterns: []string{"repo2/*"},
			expected: []string{
				"repo1/image1",
				"repo3/image3",
			},
		},
		{
			name: "exclude everything",
			items: []string{
				"repo/image1",
				"repo/image2",
			},
			includePatterns: []string{"repo/*"},
			excludePatterns: []string{"repo/*"},
			expected:        []string{},
		},
		{
			name:            "empty items list",
			items:           []string{},
			includePatterns: []string{"*"},
			excludePatterns: nil,
			expected:        []string{},
		},
		{
			name: "no patterns - return all",
			items: []string{
				"item1",
				"item2",
				"item3",
			},
			includePatterns: nil,
			excludePatterns: nil,
			expected: []string{
				"item1",
				"item2",
				"item3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterByPattern(tt.items, tt.includePatterns, tt.excludePatterns)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFilterTagsBySemver(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		constraints []string
		expected    []string
		expectError bool
	}{
		{
			name: "no constraints - include all valid semver",
			tags: []string{
				"1.0.0",
				"1.2.3",
				"2.0.0",
				"latest",
				"dev",
			},
			constraints: []string{},
			expected: []string{
				"1.0.0",
				"1.2.3",
				"2.0.0",
			},
		},
		{
			name: "v prefix handling",
			tags: []string{
				"v1.0.0",
				"v1.2.3",
				"v2.0.0",
			},
			constraints: []string{},
			// NOTE: Current implementation strips 'v' prefix when no constraints
			// This is inconsistent with the constrained case which preserves original tags
			expected: []string{
				"1.0.0",
				"1.2.3",
				"2.0.0",
			},
		},
		{
			name: "exact version constraint",
			tags: []string{
				"1.0.0",
				"1.2.3",
				"2.0.0",
			},
			constraints: []string{"1.2.3"},
			expected: []string{
				"1.2.3",
			},
		},
		{
			name: "greater than or equal",
			tags: []string{
				"0.8.0",
				"0.9.0",
				"1.0.0",
				"1.2.3",
				"2.0.0",
			},
			constraints: []string{">=1.0.0"},
			expected: []string{
				"1.0.0",
				"1.2.3",
				"2.0.0",
			},
		},
		{
			name: "less than constraint",
			tags: []string{
				"0.8.0",
				"0.9.0",
				"1.0.0",
				"2.0.0",
			},
			constraints: []string{"<1.0.0"},
			expected: []string{
				"0.8.0",
				"0.9.0",
			},
		},
		{
			name: "range constraint",
			tags: []string{
				"0.8.0",
				"1.0.0",
				"1.5.0",
				"2.0.0",
				"3.0.0",
			},
			constraints: []string{">=1.0.0 <2.0.0"},
			expected: []string{
				"1.0.0",
				"1.5.0",
			},
		},
		{
			name: "tilde constraint - patch updates",
			tags: []string{
				"1.2.0",
				"1.2.1",
				"1.2.9",
				"1.3.0",
				"2.0.0",
			},
			constraints: []string{"~1.2.0"},
			expected: []string{
				"1.2.0",
				"1.2.1",
				"1.2.9",
			},
		},
		{
			name: "caret constraint - minor and patch updates",
			tags: []string{
				"1.0.0",
				"1.2.0",
				"1.9.9",
				"2.0.0",
				"3.0.0",
			},
			constraints: []string{"^1.0.0"},
			expected: []string{
				"1.0.0",
				"1.2.0",
				"1.9.9",
			},
		},
		{
			name: "multiple OR constraints",
			tags: []string{
				"0.8.0",
				"1.0.0",
				"1.5.0",
				"2.0.0",
				"2.5.0",
			},
			constraints: []string{"~1.0.0", "~2.0.0"},
			expected: []string{
				"1.0.0",
				"2.0.0",
			},
		},
		{
			name: "prerelease versions",
			tags: []string{
				"1.0.0-alpha",
				"1.0.0-beta",
				"1.0.0",
				"1.0.1",
			},
			constraints: []string{">=1.0.0"},
			expected: []string{
				"1.0.0",
				"1.0.1",
			},
		},
		{
			name: "metadata in version",
			tags: []string{
				"1.0.0+build123",
				"1.0.0",
				"1.0.1",
			},
			constraints: []string{">=1.0.0"},
			expected: []string{
				"1.0.0+build123",
				"1.0.0",
				"1.0.1",
			},
		},
		{
			name: "invalid constraint",
			tags: []string{
				"1.0.0",
			},
			constraints: []string{"invalid-constraint"},
			expected:    nil,
			expectError: true,
		},
		{
			name: "complex real-world scenario",
			tags: []string{
				"0.8.0",
				"0.8.4",
				"0.8.4-preview",
				"0.9.0-rc1",
				"1.0.0",
				"latest",
				"dev",
				"main",
			},
			constraints: []string{">=0.8.0 <1.0.0"},
			expected: []string{
				"0.8.0",
				"0.8.4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FilterTagsBySemver(tt.tags, tt.constraints)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}

func TestIsDockerHub(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		expected bool
	}{
		{
			name:     "docker.io",
			registry: "docker.io",
			expected: true,
		},
		{
			name:     "index.docker.io",
			registry: "index.docker.io",
			expected: true,
		},
		{
			name:     "registry-1.docker.io",
			registry: "registry-1.docker.io",
			expected: true,
		},
		{
			name:     "uppercase docker.io",
			registry: "DOCKER.IO",
			expected: true,
		},
		{
			name:     "mixed case index.docker.io",
			registry: "Index.Docker.Io",
			expected: true,
		},
		{
			name:     "ghcr.io",
			registry: "ghcr.io",
			expected: false,
		},
		{
			name:     "gcr.io",
			registry: "gcr.io",
			expected: false,
		},
		{
			name:     "quay.io",
			registry: "quay.io",
			expected: false,
		},
		{
			name:     "custom registry",
			registry: "registry.example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDockerHub(tt.registry)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractNamespaceFromPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected string
	}{
		{
			name:     "simple namespace with wildcard",
			pattern:  "amdenterpriseai/aim-*",
			expected: "amdenterpriseai",
		},
		{
			name:     "namespace with multiple path segments",
			pattern:  "myorg/myteam/image-*",
			expected: "myorg",
		},
		{
			name:     "no namespace - just image pattern",
			pattern:  "aim-*",
			expected: "",
		},
		{
			name:     "wildcard in namespace",
			pattern:  "*/aim-llama",
			expected: "",
		},
		{
			name:     "no wildcard at all",
			pattern:  "amdenterpriseai/aim-llama",
			expected: "amdenterpriseai",
		},
		{
			name:     "empty pattern",
			pattern:  "",
			expected: "",
		},
		{
			name:     "just slash",
			pattern:  "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNamespaceFromPattern(tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}
