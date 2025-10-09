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

package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// Profile represents the discovery profile payload emitted by the runtime discovery job.
type Profile struct {
	EngineArgs map[string]any `json:"engine_args"`
}

// TemplateState captures the resolved data required to materialize runtimes and services from a template.
type TemplateState struct {
	Name              string
	Namespace         string
	SpecCommon        aimv1alpha1.AIMServiceTemplateSpecCommon
	Image             string
	ImagePullSecrets  []corev1.LocalObjectReference
	RuntimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec
	Status            *aimv1alpha1.AIMServiceTemplateStatus
	EngineArgs        []string
	ModelStorageURI   string
}

// NewTemplateState constructs a TemplateState from the provided base values.
// Callers populate the struct with template-derived data before invoking this helper.
func NewTemplateState(base TemplateState) TemplateState {
	base.ImagePullSecrets = copyPullSecrets(base.ImagePullSecrets)

	if base.Status != nil {
		base.Status = base.Status.DeepCopy()
		base.EngineArgs = ExtractEngineArgs(base.Status.Profile)
		base.ModelStorageURI = ExtractPrimaryModelURI(base.Status.ModelSources)
	}

	return base
}

// ServiceAccountName returns the resolved service account for resources derived from this template.
func (s TemplateState) ServiceAccountName() string {
	return s.RuntimeConfigSpec.ServiceAccountName
}

// StorageURI returns the primary model URI discovered for the template.
func (s TemplateState) StorageURI() string {
	return s.ModelStorageURI
}

// ExtractEngineArgs converts the discovery profile's engine arguments into CLI flags.
func ExtractEngineArgs(profile *apiextensionsv1.JSON) []string {
	if profile == nil || len(profile.Raw) == 0 {
		return nil
	}

	var parsed Profile
	if err := json.Unmarshal(profile.Raw, &parsed); err != nil {
		return nil
	}

	if len(parsed.EngineArgs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(parsed.EngineArgs))
	for key := range parsed.EngineArgs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var args []string
	for _, key := range keys {
		args = append(args, renderEngineArg(key, parsed.EngineArgs[key])...)
	}
	return args
}

func renderEngineArg(key string, value any) []string {
	switch v := value.(type) {
	case []any:
		var tokens []string
		for _, elem := range v {
			if token, ok := renderEngineArgSingle(key, elem); ok {
				tokens = append(tokens, token)
			}
		}
		return tokens
	default:
		if token, ok := renderEngineArgSingle(key, v); ok {
			return []string{token}
		}
		return nil
	}
}

func renderEngineArgSingle(key string, value any) (string, bool) {
	if rendered, ok := formatEngineArgValue(value); ok {
		return fmt.Sprintf("--%s=%s", key, rendered), true
	}
	return "", false
}

func formatEngineArgValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	case map[string]any, []any:
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v), true
		}
		if len(bytes) == 0 {
			return "", false
		}
		return string(bytes), true
	case nil:
		return "", false
	default:
		return fmt.Sprint(v), true
	}
}

// ExtractPrimaryModelURI returns the first non-empty model source URI from the template status.
func ExtractPrimaryModelURI(sources []aimv1alpha1.AIMModelSource) string {
	for _, source := range sources {
		if source.SourceURI != "" {
			return source.SourceURI
		}
	}
	return ""
}

func copyPullSecrets(in []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.LocalObjectReference, len(in))
	copy(out, in)
	return out
}
