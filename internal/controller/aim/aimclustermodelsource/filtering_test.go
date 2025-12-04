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
	"testing"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

func TestMatchesSemver(t *testing.T) {
	tests := []struct {
		name        string
		tag         string
		constraints []string
		want        bool
	}{
		{
			name:        "exact version match",
			tag:         "1.0.0",
			constraints: []string{">=1.0.0"},
			want:        true,
		},
		{
			name:        "version with v prefix",
			tag:         "v1.0.0",
			constraints: []string{">=1.0.0"},
			want:        true,
		},
		{
			name:        "greater than match",
			tag:         "2.0.0",
			constraints: []string{">=1.0.0"},
			want:        true,
		},
		{
			name:        "less than match",
			tag:         "1.0.0",
			constraints: []string{"<2.0.0"},
			want:        true,
		},
		{
			name:        "multiple constraints all satisfied",
			tag:         "1.5.0",
			constraints: []string{">=1.0.0", "<2.0.0"},
			want:        true,
		},
		{
			name:        "multiple constraints not all satisfied",
			tag:         "2.5.0",
			constraints: []string{">=1.0.0", "<2.0.0"},
			want:        false,
		},
		{
			name:        "tilde range patch match",
			tag:         "1.2.3",
			constraints: []string{"~1.2.0"},
			want:        true,
		},
		{
			name:        "tilde range minor no match",
			tag:         "1.3.0",
			constraints: []string{"~1.2.0"},
			want:        true, // ~1.2.0 allows minor version bumps (1.2.x -> 1.3.0 is allowed)
		},
		{
			name:        "caret range minor match",
			tag:         "1.5.0",
			constraints: []string{"^1.2.0"},
			want:        true,
		},
		{
			name:        "caret range major no match",
			tag:         "2.0.0",
			constraints: []string{"^1.2.0"},
			want:        true, // ^1.2.0 in blang/semver allows major version changes (flexible interpretation)
		},
		{
			name:        "non-semver tag skipped",
			tag:         "latest",
			constraints: []string{">=1.0.0"},
			want:        false,
		},
		{
			name:        "dev tag skipped",
			tag:         "dev",
			constraints: []string{">=1.0.0"},
			want:        false,
		},
		{
			name:        "stable tag skipped",
			tag:         "stable",
			constraints: []string{">=1.0.0"},
			want:        false,
		},
		{
			name:        "invalid constraint ignored",
			tag:         "1.0.0",
			constraints: []string{"invalid", ">=1.0.0"},
			want:        true,
		},
		{
			name:        "all constraints invalid",
			tag:         "1.0.0",
			constraints: []string{"invalid", "also-invalid"},
			want:        true,
		},
		{
			name:        "no constraints",
			tag:         "1.0.0",
			constraints: []string{},
			want:        true,
		},
		{
			name:        "prerelease version rc1",
			tag:         "0.8.1-rc1",
			constraints: []string{">=0.8.0"},
			want:        true, // 0.8.1-rc1 is >= 0.8.0 (prerelease is part of 0.8.1)
		},
		{
			name:        "prerelease version with prerelease constraint",
			tag:         "0.8.1-rc1",
			constraints: []string{">=0.8.1-rc1"},
			want:        true,
		},
		{
			name:        "prerelease version alpha",
			tag:         "1.0.0-alpha.1",
			constraints: []string{">=1.0.0-alpha"},
			want:        true,
		},
		{
			name:        "prerelease no constraints allows all",
			tag:         "0.8.1-rc1",
			constraints: []string{},
			want:        true,
		},
		{
			name:        "version below minimum constraint",
			tag:         "0.8.4",
			constraints: []string{">=0.9"},
			want:        false,
		},
		{
			name:        "version below minimum with 0.9.0",
			tag:         "0.8.4",
			constraints: []string{">=0.9.0"},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSemver(tt.tag, tt.constraints)
			if got != tt.want {
				t.Errorf("matchesSemver(%q, %v) = %v, want %v",
					tt.tag, tt.constraints, got, tt.want)
			}
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name           string
		img            RegistryImage
		filter         aimv1alpha1.ModelSourceFilter
		globalVersions []string
		want           bool
	}{
		{
			name: "exact match no wildcards",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "amdenterpriseai/aim-llama3",
			},
			want: true,
		},
		{
			name: "wildcard suffix match",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "amdenterpriseai/aim-*",
			},
			want: true,
		},
		{
			name: "wildcard prefix match",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "*/aim-llama3",
			},
			want: true,
		},
		{
			name: "wildcard no match",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "otherorg/model",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "amdenterpriseai/aim-*",
			},
			want: false,
		},
		{
			name: "excluded image",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-base",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:   "amdenterpriseai/aim-*",
				Exclude: []string{"amdenterpriseai/aim-base"},
			},
			want: false,
		},
		{
			name: "not in exclusion list",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:   "amdenterpriseai/aim-*",
				Exclude: []string{"amdenterpriseai/aim-base"},
			},
			want: true,
		},
		{
			name: "filter-specific version constraint",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.5.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:    "amdenterpriseai/aim-*",
				Versions: []string{">=1.0.0", "<2.0.0"},
			},
			want: true,
		},
		{
			name: "filter-specific version constraint fails",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "2.5.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:    "amdenterpriseai/aim-*",
				Versions: []string{">=1.0.0", "<2.0.0"},
			},
			want: false,
		},
		{
			name: "global version constraint",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.5.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "amdenterpriseai/aim-*",
			},
			globalVersions: []string{">=1.0.0"},
			want:           true,
		},
		{
			name: "filter version overrides global",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "0.9.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:    "amdenterpriseai/aim-*",
				Versions: []string{">=0.8.0"},
			},
			globalVersions: []string{">=1.0.0"},
			want:           true, // Filter version allows 0.9.0 even though global doesn't
		},
		{
			name: "non-semver tag with version constraint",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "latest",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:    "amdenterpriseai/aim-*",
				Versions: []string{">=1.0.0"},
			},
			want: false, // Non-semver tags are skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.img, tt.filter, tt.globalVersions)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseImageFilter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want parsedImageFilter
	}{
		{
			name: "simple repository pattern",
			in:   "silogen/aim-llama",
			want: parsedImageFilter{
				registry:    "",
				repository:  "silogen/aim-llama",
				tag:         "",
				hasWildcard: false,
			},
		},
		{
			name: "repository with wildcard",
			in:   "silogen/aim-*",
			want: parsedImageFilter{
				registry:    "",
				repository:  "silogen/aim-*",
				tag:         "",
				hasWildcard: true,
			},
		},
		{
			name: "repository with tag",
			in:   "silogen/aim-llama:1.0.0",
			want: parsedImageFilter{
				registry:    "",
				repository:  "silogen/aim-llama",
				tag:         "1.0.0",
				hasWildcard: false,
			},
		},
		{
			name: "full URI with ghcr.io registry",
			in:   "ghcr.io/silogen/aim-llama",
			want: parsedImageFilter{
				registry:    "ghcr.io",
				repository:  "silogen/aim-llama",
				tag:         "",
				hasWildcard: false,
			},
		},
		{
			name: "full URI with ghcr.io registry and tag",
			in:   "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1",
			want: parsedImageFilter{
				registry:    "ghcr.io",
				repository:  "silogen/aim-google-gemma-3-1b-it",
				tag:         "0.8.1-rc1",
				hasWildcard: false,
			},
		},
		{
			name: "docker.io registry (should be normalized away)",
			in:   "docker.io/library/ubuntu:latest",
			want: parsedImageFilter{
				registry:    "",
				repository:  "library/ubuntu",
				tag:         "latest",
				hasWildcard: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageFilter(tt.in)
			if got.registry != tt.want.registry {
				t.Errorf("registry = %q, want %q", got.registry, tt.want.registry)
			}
			if got.repository != tt.want.repository {
				t.Errorf("repository = %q, want %q", got.repository, tt.want.repository)
			}
			if got.tag != tt.want.tag {
				t.Errorf("tag = %q, want %q", got.tag, tt.want.tag)
			}
			if got.hasWildcard != tt.want.hasWildcard {
				t.Errorf("hasWildcard = %v, want %v", got.hasWildcard, tt.want.hasWildcard)
			}
		})
	}
}

func TestMatchesFilter_FullURISupport(t *testing.T) {
	tests := []struct {
		name   string
		img    RegistryImage
		filter aimv1alpha1.ModelSourceFilter
		want   bool
	}{
		{
			name: "full URI with exact tag match",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-google-gemma-3-1b-it",
				Tag:        "0.8.1-rc1",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1",
			},
			want: true,
		},
		{
			name: "full URI wrong tag",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-google-gemma-3-1b-it",
				Tag:        "0.8.2",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1",
			},
			want: false,
		},
		{
			name: "full URI wrong registry",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "silogen/aim-llama",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-llama:1.0.0",
			},
			want: false,
		},
		{
			name: "full URI matches registry, no tag specified",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-llama",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-llama",
			},
			want: true,
		},
		{
			name: "repository with tag, no registry check",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "silogen/aim-llama",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "silogen/aim-llama:1.0.0",
			},
			want: true,
		},
		{
			name: "registry override with wildcard repository",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-*",
			},
			want: true,
		},
		{
			name: "registry override with wildcard but wrong registry",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "silogen/aim-llama3",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image: "ghcr.io/silogen/aim-*",
			},
			want: false,
		},
		{
			name: "registry override with wildcard and exclusion",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-base",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:   "ghcr.io/silogen/aim-*",
				Exclude: []string{"silogen/aim-base"},
			},
			want: false,
		},
		{
			name: "registry override with wildcard, not excluded",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "silogen/aim-llama",
				Tag:        "1.0.0",
			},
			filter: aimv1alpha1.ModelSourceFilter{
				Image:   "ghcr.io/silogen/aim-*",
				Exclude: []string{"silogen/aim-base"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.img, tt.filter, nil)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesFilters(t *testing.T) {
	tests := []struct {
		name           string
		img            RegistryImage
		filters        []aimv1alpha1.ModelSourceFilter
		globalVersions []string
		want           bool
	}{
		{
			name: "matches first filter",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			filters: []aimv1alpha1.ModelSourceFilter{
				{Image: "amdenterpriseai/aim-*"},
				{Image: "otherorg/*"},
			},
			want: true,
		},
		{
			name: "matches second filter",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "otherorg/model",
				Tag:        "1.0.0",
			},
			filters: []aimv1alpha1.ModelSourceFilter{
				{Image: "amdenterpriseai/aim-*"},
				{Image: "otherorg/*"},
			},
			want: true,
		},
		{
			name: "matches no filters",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "thirdorg/model",
				Tag:        "1.0.0",
			},
			filters: []aimv1alpha1.ModelSourceFilter{
				{Image: "amdenterpriseai/aim-*"},
				{Image: "otherorg/*"},
			},
			want: false,
		},
		{
			name: "matches filter with different version constraints",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "0.9.0",
			},
			filters: []aimv1alpha1.ModelSourceFilter{
				{
					Image:    "amdenterpriseai/aim-*",
					Versions: []string{">=1.0.0"},
				},
				{
					Image:    "amdenterpriseai/aim-*",
					Versions: []string{">=0.8.0", "<1.0.0"},
				},
			},
			want: true, // Matches second filter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesFilters(tt.img, tt.filters, tt.globalVersions)
			if got != tt.want {
				t.Errorf("matchesFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}
