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

package helpers

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMergeEnvVars_BasicOverride(t *testing.T) {
	defaults := []corev1.EnvVar{
		{Name: "VAR1", Value: "default1"},
		{Name: "VAR2", Value: "default2"},
	}
	overrides := []corev1.EnvVar{
		{Name: "VAR2", Value: "override2"},
		{Name: "VAR3", Value: "override3"},
	}

	result := MergeEnvVars(defaults, overrides)

	resultMap := make(map[string]string)
	for _, env := range result {
		resultMap[env.Name] = env.Value
	}

	if resultMap["VAR1"] != "default1" {
		t.Errorf("expected VAR1=default1, got %s", resultMap["VAR1"])
	}
	if resultMap["VAR2"] != "override2" {
		t.Errorf("expected VAR2=override2, got %s", resultMap["VAR2"])
	}
	if resultMap["VAR3"] != "override3" {
		t.Errorf("expected VAR3=override3, got %s", resultMap["VAR3"])
	}
}

func TestMergeEnvVars_JSONDeepMerge(t *testing.T) {
	defaults := []corev1.EnvVar{
		{Name: "MY_JSON_VAR", Value: `{"config": {"a": "from_default", "b": "from_default"}}`},
	}
	overrides := []corev1.EnvVar{
		{Name: "MY_JSON_VAR", Value: `{"config": {"b": "from_override", "c": "from_override"}}`},
	}

	result := MergeEnvVars(defaults, overrides, "MY_JSON_VAR")

	var resultJSON map[string]any
	if err := json.Unmarshal([]byte(result[0].Value), &resultJSON); err != nil {
		t.Fatalf("failed to parse merged JSON: %v", err)
	}

	config := resultJSON["config"].(map[string]any)
	if config["a"] != "from_default" {
		t.Errorf("expected a='from_default', got %v", config["a"])
	}
	if config["b"] != "from_override" {
		t.Errorf("expected b='from_override', got %v", config["b"])
	}
	if config["c"] != "from_override" {
		t.Errorf("expected c='from_override', got %v", config["c"])
	}
}

func TestDeepMergeMap(t *testing.T) {
	dst := map[string]any{
		"outer": map[string]any{
			"keep":     "dst_value",
			"override": "dst_value",
		},
	}
	src := map[string]any{
		"outer": map[string]any{
			"override": "src_value",
			"add":      "src_value",
		},
	}

	DeepMergeMap(dst, src)

	outer := dst["outer"].(map[string]any)
	if outer["keep"] != "dst_value" {
		t.Errorf("expected keep='dst_value', got %v", outer["keep"])
	}
	if outer["override"] != "src_value" {
		t.Errorf("expected override='src_value', got %v", outer["override"])
	}
	if outer["add"] != "src_value" {
		t.Errorf("expected add='src_value', got %v", outer["add"])
	}
}
