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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
)

// BuildDerivedTemplate constructs an AIMServiceTemplate for a service with overrides.
// The template inherits from the base spec and applies service-specific customizations.
func BuildDerivedTemplate(
	service *aimv1alpha1.AIMService,
	templateName string,
	ownerRef metav1.OwnerReference,
	baseSpec *aimv1alpha1.AIMServiceTemplateSpec,
) *aimv1alpha1.AIMServiceTemplate {
	spec := aimv1alpha1.AIMServiceTemplateSpec{}
	if baseSpec != nil {
		spec = *baseSpec.DeepCopy()
	}

	specCommon := spec.AIMServiceTemplateSpecCommon

	if specCommon.AIMImageName == "" {
		specCommon.AIMImageName = service.Spec.AIMImageName
	}

	if rc := strings.TrimSpace(service.Spec.RuntimeConfigName); rc != "" {
		specCommon.RuntimeConfigName = NormalizeRuntimeConfigName(rc)
	} else {
		specCommon.RuntimeConfigName = NormalizeRuntimeConfigName(specCommon.RuntimeConfigName)
	}

	if service.Spec.Overrides != nil {
		if service.Spec.Overrides.Metric != nil {
			metric := *service.Spec.Overrides.Metric
			specCommon.Metric = &metric
		}
		if service.Spec.Overrides.Precision != nil {
			precision := *service.Spec.Overrides.Precision
			specCommon.Precision = &precision
		}
		if service.Spec.Overrides.GpuSelector != nil {
			selector := *service.Spec.Overrides.GpuSelector
			specCommon.GpuSelector = &selector
		}
	}

	spec.AIMServiceTemplateSpecCommon = specCommon

	if len(service.Spec.Env) > 0 {
		spec.Env = helpers.CopyEnvVars(service.Spec.Env)
	} else {
		spec.Env = helpers.CopyEnvVars(spec.Env)
	}

	if len(service.Spec.ImagePullSecrets) > 0 {
		spec.ImagePullSecrets = helpers.CopyPullSecrets(service.Spec.ImagePullSecrets)
	} else {
		spec.ImagePullSecrets = helpers.CopyPullSecrets(spec.ImagePullSecrets)
	}

	if service.Spec.Resources != nil {
		spec.Resources = service.Spec.Resources.DeepCopy()
	} else if spec.Resources != nil {
		spec.Resources = spec.Resources.DeepCopy()
	}

	if service.Spec.CacheModel {
		spec.Caching = &aimv1alpha1.AIMTemplateCachingConfig{
			Enabled: service.Spec.CacheModel,
			Env:     helpers.CopyEnvVars(service.Spec.Env),
		}
	} else if spec.Caching != nil {
		spec.Caching = spec.Caching.DeepCopy()
	}

	template := &aimv1alpha1.AIMServiceTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aimv1alpha1.GroupVersion.String(),
			Kind:       "AIMServiceTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            templateName,
			Namespace:       service.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: spec,
	}

	return template
}
