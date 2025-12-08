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
	"fmt"
	"strings"
	"testing"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

func TestGenerateTemplateName(t *testing.T) {
	tests := []struct {
		name       string
		imageName  string
		deployment aimv1alpha1.RecommendedDeployment
		wantName   string // Expected exact name (deterministic)
	}{
		{
			name:      "simple short name",
			imageName: "llama-3-8b",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				GPUCount:  1,
				Metric:    "latency",
				Precision: "fp8",
			},
			wantName: "llama-3-8b-1x-mi300x-lat-fp8-27c0",
		},
		{
			name:      "very long image name gets truncated",
			imageName: "very-long-model-name-that-exceeds-kubernetes-limits-llama-3-1-70b-instruct",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				GPUCount:  8,
				Metric:    "throughput",
				Precision: "fp16",
			},
			wantName: "very-long-model-name-that-exceeds-kuber-8x-mi300x-thr-fp16-225e",
		},
		{
			name:      "metric shorthand - latency",
			imageName: "model",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				GPUCount:  1,
				Metric:    "latency",
				Precision: "fp8",
			},
			wantName: "model-1x-mi300x-lat-fp8-12d7",
		},
		{
			name:      "metric shorthand - throughput",
			imageName: "model",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				GPUCount:  2,
				Metric:    "throughput",
				Precision: "fp16",
			},
			wantName: "model-2x-mi300x-thr-fp16-884b",
		},
		{
			name:      "gpu count and model format",
			imageName: "test-model",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI325X",
				GPUCount:  4,
				Metric:    "latency",
				Precision: "fp8",
			},
			wantName: "test-model-4x-mi325x-lat-fp8-70e6",
		},
		{
			name:      "only gpu model without count",
			imageName: "test-model",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				Metric:    "latency",
				Precision: "fp8",
			},
			wantName: "test-model-mi300x-lat-fp8-465c",
		},
		{
			name:      "minimal deployment info",
			imageName: "simple-model",
			deployment: aimv1alpha1.RecommendedDeployment{
				Metric: "latency",
			},
			wantName: "simple-model-lat-c9f9",
		},
		{
			name:      "extremely long name with max truncation",
			imageName: "this-is-an-extremely-long-model-name-that-will-definitely-exceed-kubernetes-limits-and-cause-severe-truncation",
			deployment: aimv1alpha1.RecommendedDeployment{
				GPUModel:  "MI300X",
				GPUCount:  8,
				Metric:    "throughput",
				Precision: "fp16",
			},
			wantName: "this-is-an-extremely-long-model-name-th-8x-mi300x-thr-fp16-e207",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTemplateName(tt.imageName, tt.deployment)

			// Check exact expected name (deterministic)
			if got != tt.wantName {
				t.Errorf("generateTemplateName() = %q, want %q", got, tt.wantName)
			}

			// Verify length constraint (should always be <= 63)
			if len(got) > 63 {
				t.Errorf("generateTemplateName() length = %d, exceeds 63 chars: %s", len(got), got)
			}

			// Verify RFC1123 compliance
			if strings.ToLower(got) != got {
				t.Errorf("generateTemplateName() = %s is not lowercase", got)
			}
			if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
				t.Errorf("generateTemplateName() = %s starts or ends with hyphen", got)
			}
		})
	}
}

func TestGenerateTemplateName_Uniqueness(t *testing.T) {
	// Test that different profiles produce different names even with long image names
	longImageName := "very-long-model-name-that-will-be-truncated-but-should-still-be-unique"

	deployment1 := aimv1alpha1.RecommendedDeployment{
		GPUModel:  "MI300X",
		GPUCount:  1,
		Metric:    "latency",
		Precision: "fp8",
	}

	deployment2 := aimv1alpha1.RecommendedDeployment{
		GPUModel:  "MI300X",
		GPUCount:  1,
		Metric:    "throughput",
		Precision: "fp8",
	}

	deployment3 := aimv1alpha1.RecommendedDeployment{
		GPUModel:  "MI300X",
		GPUCount:  1,
		Metric:    "latency",
		Precision: "fp16",
	}

	deployment4 := aimv1alpha1.RecommendedDeployment{
		GPUModel:  "MI325X",
		GPUCount:  1,
		Metric:    "latency",
		Precision: "fp8",
	}

	name1 := generateTemplateName(longImageName, deployment1)
	name2 := generateTemplateName(longImageName, deployment2)
	name3 := generateTemplateName(longImageName, deployment3)
	name4 := generateTemplateName(longImageName, deployment4)

	// All names should be different
	names := map[string]bool{
		name1: true,
		name2: true,
		name3: true,
		name4: true,
	}

	if len(names) != 4 {
		t.Errorf("Expected 4 unique names, got %d. Names: %s, %s, %s, %s", len(names), name1, name2, name3, name4)
	}

	// All should contain different distinguishing parts
	if !strings.Contains(name1, "lat") {
		t.Errorf("name1 should contain 'lat': %s", name1)
	}
	if !strings.Contains(name2, "thr") {
		t.Errorf("name2 should contain 'thr': %s", name2)
	}
	if !strings.Contains(name3, "fp16") {
		t.Errorf("name3 should contain 'fp16': %s", name3)
	}
	if !strings.Contains(name4, "mi325x") {
		t.Errorf("name4 should contain 'mi325x': %s", name4)
	}
}

func TestGenerateTemplateName_ExtremelyLongNames(t *testing.T) {
	// Test with an extremely long image name that will definitely be truncated
	extremelyLongImage := "this-is-an-extremely-long-model-name-that-will-definitely-exceed-kubernetes-limits-and-cause-severe-truncation"

	profiles := []struct {
		name      string
		gpuModel  string
		gpuCount  int32
		metric    string
		precision string
	}{
		{"profile1", "MI300X", 1, "latency", "fp8"},
		{"profile2", "MI300X", 1, "latency", "fp16"},
		{"profile3", "MI300X", 1, "throughput", "fp8"},
		{"profile4", "MI300X", 2, "latency", "fp8"},
		{"profile5", "MI325X", 1, "latency", "fp8"},
		{"profile6", "MI300X", 8, "throughput", "fp16"},
	}

	generatedNames := make(map[string]string)

	for _, p := range profiles {
		deployment := aimv1alpha1.RecommendedDeployment{
			GPUModel:  p.gpuModel,
			GPUCount:  p.gpuCount,
			Metric:    p.metric,
			Precision: p.precision,
		}

		name := generateTemplateName(extremelyLongImage, deployment)

		// Check length
		if len(name) > 63 {
			t.Errorf("Profile %s: name length %d exceeds 63 chars: %s", p.name, len(name), name)
		}

		// Check for collisions
		if existingProfile, found := generatedNames[name]; found {
			t.Errorf("Collision detected! Profile %s generated same name as %s: %s", p.name, existingProfile, name)
		}
		generatedNames[name] = p.name

		// Verify profile parameters are present (except for extremely truncated image part)
		expectedGPU := fmt.Sprintf("%dx-%s", p.gpuCount, strings.ToLower(p.gpuModel))
		if !strings.Contains(name, expectedGPU) {
			t.Errorf("Profile %s: name missing GPU info %s: %s", p.name, expectedGPU, name)
		}

		expectedMetric := getMetricShorthand(p.metric)
		if !strings.Contains(name, expectedMetric) {
			t.Errorf("Profile %s: name missing metric %s: %s", p.name, expectedMetric, name)
		}

		expectedPrecision := strings.ToLower(p.precision)
		if !strings.Contains(name, expectedPrecision) {
			t.Errorf("Profile %s: name missing precision %s: %s", p.name, expectedPrecision, name)
		}
	}

	// Verify all names are unique
	if len(generatedNames) != len(profiles) {
		t.Errorf("Expected %d unique names, got %d", len(profiles), len(generatedNames))
	}
}

func TestGenerateTemplateName_HashStability(t *testing.T) {
	// Test that the same inputs always produce the same name (deterministic hashing)
	imageName := "test-model"
	deployment := aimv1alpha1.RecommendedDeployment{
		GPUModel:  "MI300X",
		GPUCount:  1,
		Metric:    "latency",
		Precision: "fp8",
	}

	name1 := generateTemplateName(imageName, deployment)
	name2 := generateTemplateName(imageName, deployment)
	name3 := generateTemplateName(imageName, deployment)

	if name1 != name2 || name2 != name3 {
		t.Errorf("generateTemplateName() is not deterministic: %s, %s, %s", name1, name2, name3)
	}
}
