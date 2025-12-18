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

package aim

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
)

func TestBuildMergedEnvVars_SystemDefaults(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
	}

	envVars := buildMergedEnvVars(service, obs)

	envMap := envVarsToMap(envVars)

	// Verify system defaults
	if envMap["AIM_CACHE_PATH"] != AIMCacheBasePath {
		t.Errorf("expected AIM_CACHE_PATH=%s, got %s", AIMCacheBasePath, envMap["AIM_CACHE_PATH"])
	}
}

func TestBuildMergedEnvVars_ProfileID(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	profileID := "custom-profile-id"
	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			ProfileId: profileID,
		},
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	if envMap["AIM_PROFILE_ID"] != profileID {
		t.Errorf("expected AIM_PROFILE_ID=%s, got %s", profileID, envMap["AIM_PROFILE_ID"])
	}
}

func TestBuildMergedEnvVars_MetricAndPrecision(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	metric := aimv1alpha1.AIMMetric("throughput")
	precision := aimv1alpha1.AIMPrecision("fp16")

	templateSpecCommon := aimv1alpha1.AIMServiceTemplateSpecCommon{
		ModelName: "test-model",
		AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
			Metric:    &metric,
			Precision: &precision,
		},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: templateSpecCommon,
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	if envMap["AIM_METRIC"] != string(metric) {
		t.Errorf("expected AIM_METRIC=%s, got %s", metric, envMap["AIM_METRIC"])
	}

	if envMap["AIM_PRECISION"] != string(precision) {
		t.Errorf("expected AIM_PRECISION=%s, got %s", precision, envMap["AIM_PRECISION"])
	}
}

func TestBuildMergedEnvVars_TemplateEnvVars(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	templateSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		Env: []corev1.EnvVar{
			{Name: "TEMPLATE_VAR", Value: "template_value"},
		},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
		TemplateSpec:       templateSpec,
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	if envMap["TEMPLATE_VAR"] != "template_value" {
		t.Errorf("expected TEMPLATE_VAR=template_value, got %s", envMap["TEMPLATE_VAR"])
	}
}

func TestBuildMergedEnvVars_ProfileEnvVars(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	templateStatus := &aimv1alpha1.AIMServiceTemplateStatus{
		Profile: aimv1alpha1.AIMProfile{
			EnvVars: map[string]string{
				"VLLM_USE_V1":               "0",
				"PYTORCH_TUNABLEOP_ENABLED": "1",
			},
		},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
		TemplateStatus:     templateStatus,
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	if envMap["VLLM_USE_V1"] != "0" {
		t.Errorf("expected VLLM_USE_V1=0, got %s", envMap["VLLM_USE_V1"])
	}

	if envMap["PYTORCH_TUNABLEOP_ENABLED"] != "1" {
		t.Errorf("expected PYTORCH_TUNABLEOP_ENABLED=1, got %s", envMap["PYTORCH_TUNABLEOP_ENABLED"])
	}
}

func TestBuildMergedEnvVars_UserOverrides(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
				Env: []corev1.EnvVar{
					{Name: "USER_VAR", Value: "user_value"},
					{Name: "AIM_CACHE_PATH", Value: "/custom/cache/path"},
				},
			},
		},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	// User env vars should have highest precedence
	if envMap["AIM_CACHE_PATH"] != "/custom/cache/path" {
		t.Errorf("expected user override AIM_CACHE_PATH=/custom/cache/path, got %s", envMap["AIM_CACHE_PATH"])
	}

	if envMap["USER_VAR"] != "user_value" {
		t.Errorf("expected USER_VAR=user_value, got %s", envMap["USER_VAR"])
	}
}

func TestBuildMergedEnvVars_Precedence(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
				Env: []corev1.EnvVar{
					{Name: "COMMON_VAR", Value: "from_user"},
				},
			},
		},
	}

	templateSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		Env: []corev1.EnvVar{
			{Name: "COMMON_VAR", Value: "from_template"},
		},
	}

	templateStatus := &aimv1alpha1.AIMServiceTemplateStatus{
		Profile: aimv1alpha1.AIMProfile{
			EnvVars: map[string]string{
				"COMMON_VAR": "from_profile",
			},
		},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
		TemplateSpec:       templateSpec,
		TemplateStatus:     templateStatus,
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	// User env vars should win (highest precedence)
	if envMap["COMMON_VAR"] != "from_user" {
		t.Errorf("expected COMMON_VAR=from_user (user precedence), got %s", envMap["COMMON_VAR"])
	}
}

func TestBuildMergedEnvVars_KVCache(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{},
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{},
		KVCacheConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kvcache-config",
				Namespace: "default",
			},
		},
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	// Verify KVCache env vars are set
	if envMap["LMCACHE_USE_EXPERIMENTAL"] != "True" {
		t.Errorf("expected LMCACHE_USE_EXPERIMENTAL=True, got %s", envMap["LMCACHE_USE_EXPERIMENTAL"])
	}

	if envMap["LMCACHE_CONFIG_FILE"] != "/lmcache/lmcache_config.yaml" {
		t.Errorf("expected LMCACHE_CONFIG_FILE=/lmcache/lmcache_config.yaml, got %s", envMap["LMCACHE_CONFIG_FILE"])
	}

	if envMap["PYTHONHASHSEED"] != "0" {
		t.Errorf("expected PYTHONHASHSEED=0, got %s", envMap["PYTHONHASHSEED"])
	}
}

func TestBuildMergedEnvVars_AllSources(t *testing.T) {
	metric := aimv1alpha1.AIMMetric("latency")
	precision := aimv1alpha1.AIMPrecision("fp8")

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
				Env: []corev1.EnvVar{
					{Name: "USER_VAR", Value: "user_value"},
				},
			},
		},
	}

	templateSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		Env: []corev1.EnvVar{
			{Name: "TEMPLATE_VAR", Value: "template_value"},
		},
	}

	templateStatus := &aimv1alpha1.AIMServiceTemplateStatus{
		Profile: aimv1alpha1.AIMProfile{
			EnvVars: map[string]string{
				"PROFILE_VAR": "profile_value",
			},
		},
	}

	templateSpecCommon := aimv1alpha1.AIMServiceTemplateSpecCommon{
		ModelName: "test-model",
		AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
			Metric:    &metric,
			Precision: &precision,
		},
		ProfileId: "test-profile",
	}

	obs := &shared.ServiceObservation{
		TemplateSpecCommon: templateSpecCommon,
		TemplateSpec:       templateSpec,
		TemplateStatus:     templateStatus,
	}

	envVars := buildMergedEnvVars(service, obs)
	envMap := envVarsToMap(envVars)

	// Verify all sources are present
	expectedVars := map[string]string{
		"AIM_CACHE_PATH": AIMCacheBasePath,
		"AIM_PROFILE_ID": "test-profile",
		"AIM_METRIC":     "latency",
		"AIM_PRECISION":  "fp8",
		"TEMPLATE_VAR":   "template_value",
		"PROFILE_VAR":    "profile_value",
		"USER_VAR":       "user_value",
	}

	for key, expectedValue := range expectedVars {
		if actualValue, exists := envMap[key]; !exists {
			t.Errorf("expected env var %s to exist", key)
		} else if actualValue != expectedValue {
			t.Errorf("expected %s=%s, got %s", key, expectedValue, actualValue)
		}
	}
}

// envVarsToMap converts a slice of EnvVar to a map for easier testing
func envVarsToMap(envVars []corev1.EnvVar) map[string]string {
	result := make(map[string]string)
	for _, env := range envVars {
		result[env.Name] = env.Value
	}
	return result
}
