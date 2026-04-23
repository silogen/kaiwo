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

package controller

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

func TestParseGpuSamples_Gauge(t *testing.T) {
	input := `
# TYPE gpu_gfx_activity gauge
gpu_gfx_activity{namespace="prod",pod="trainer-0",gpu_id="0"} 88.5
`
	family := mustParseMetricFamily(t, input)
	samples := parseGpuSamples(family)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].namespace != "prod" || samples[0].podName != "trainer-0" || samples[0].gpuID != "0" {
		t.Fatalf("unexpected labels: %+v", samples[0])
	}
	if samples[0].utilization != 88.5 {
		t.Fatalf("expected utilization 88.5, got %v", samples[0].utilization)
	}
}

func TestParseGpuSamples_Untyped(t *testing.T) {
	input := `
# TYPE gpu_gfx_activity untyped
gpu_gfx_activity{namespace="prod",pod="trainer-0",gpu_id="0"} 100
`
	family := mustParseMetricFamily(t, input)
	samples := parseGpuSamples(family)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].utilization != 100 {
		t.Fatalf("expected utilization 100, got %v", samples[0].utilization)
	}
}

func TestParseGpuSamples_NoTypeLineUsesUntyped(t *testing.T) {
	// No # TYPE: Prometheus text parser treats samples as UNTYPED.
	input := `gpu_gfx_activity{namespace="prod",pod="trainer-0",gpu_id="1"} 42
`
	family := mustParseMetricFamily(t, input)
	samples := parseGpuSamples(family)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].utilization != 42 {
		t.Fatalf("expected utilization 42, got %v", samples[0].utilization)
	}
}

func TestParseGpuSamples_AlternatePodLabels(t *testing.T) {
	input := `
# TYPE gpu_gfx_activity gauge
gpu_gfx_activity{k8s_pod_name="trainer-0",kubernetes_namespace="prod",gpu_id="0"} 55
`
	family := mustParseMetricFamily(t, input)
	samples := parseGpuSamples(family)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].namespace != "prod" || samples[0].podName != "trainer-0" {
		t.Fatalf("unexpected ns/pod: %+v", samples[0])
	}
}

func mustParseMetricFamily(t *testing.T, text string) *dto.MetricFamily {
	t.Helper()
	parser := expfmt.TextParser{}
	families, err := parser.TextToMetricFamilies(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	mf := families["gpu_gfx_activity"]
	if mf == nil {
		t.Fatal("missing gpu_gfx_activity family")
	}
	return mf
}
