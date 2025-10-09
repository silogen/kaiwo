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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

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

// NewTemplateState constructs a TemplateState from the template specification, observation, and status.
func NewTemplateState(name, namespace string, specCommon aimv1alpha1.AIMServiceTemplateSpecCommon, observation *TemplateObservation, runtimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec, status *aimv1alpha1.AIMServiceTemplateStatus) TemplateState {
	state := TemplateState{
		Name:              name,
		Namespace:         namespace,
		SpecCommon:        specCommon,
		RuntimeConfigSpec: runtimeConfigSpec,
	}

	if observation != nil {
		state.Image = observation.Image
		state.ImagePullSecrets = CopyPullSecrets(observation.ImagePullSecrets)
	}

	if status != nil {
		state.Status = status.DeepCopy()
		state.EngineArgs = ExtractEngineArgs(state.Status.Profile)
		state.ModelStorageURI = ExtractPrimaryModelURI(state.Status.ModelSources)
	}

	return state
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
		tokens := make([]string, 0, len(v))
		for _, elem := range v {
			tokens = append(tokens, renderEngineArgSingle(key, elem))
		}
		return tokens
	default:
		return []string{renderEngineArgSingle(key, v)}
	}
}

func renderEngineArgSingle(key string, value any) string {
	return fmt.Sprintf("--%s=%s", key, formatEngineArgValue(value))
}

func formatEngineArgValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case map[string]any, []any:
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(bytes)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
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
