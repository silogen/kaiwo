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
	corev1 "k8s.io/api/core/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
)

// TemplateState captures the resolved data required to materialize runtimes and services from a template.
type TemplateState struct {
	Name              string
	Namespace         string
	SpecCommon        aimv1alpha1.AIMServiceTemplateSpecCommon
	Image             string
	ImageResources    *corev1.ResourceRequirements
	ImagePullSecrets  []corev1.LocalObjectReference
	RuntimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec
	Status            *aimv1alpha1.AIMServiceTemplateStatus
	ModelSource       *aimv1alpha1.AIMModelSource
}

// NewTemplateState constructs a TemplateState from the provided base values.
// Callers populate the struct with template-derived data before invoking this helper.
func NewTemplateState(base TemplateState) TemplateState {
	if base.SpecCommon.Resources != nil {
		base.SpecCommon.Resources = base.SpecCommon.Resources.DeepCopy()
	}

	if base.ImageResources != nil {
		base.ImageResources = base.ImageResources.DeepCopy()
	}

	base.ImagePullSecrets = helpers.CopyPullSecrets(base.ImagePullSecrets)

	if base.Status != nil {
		base.Status = base.Status.DeepCopy()
		base.ModelSource = ExtractPrimaryModelSource(base.Status.ModelSources)
	}

	return base
}

// ServiceAccountName returns the resolved service account for resources derived from this template.
func (s TemplateState) ServiceAccountName() string {
	return s.RuntimeConfigSpec.ServiceAccountName
}

// ExtractPrimaryModelSource returns the first non-empty model source from the template status.
func ExtractPrimaryModelSource(sources []aimv1alpha1.AIMModelSource) *aimv1alpha1.AIMModelSource {
	for _, source := range sources {
		if source.SourceURI != "" {
			return &source
		}
	}
	return nil
}

// StatusMetric returns the metric discovered during template resolution, if available.
func (s TemplateState) StatusMetric() *aimv1alpha1.AIMMetric {
	if s.Status == nil {
		return nil
	}
	if metric := s.Status.Profile.Metadata.Metric; metric != "" {
		m := metric
		return &m
	}
	return nil
}

// StatusPrecision returns the precision discovered during template resolution, if available.
func (s TemplateState) StatusPrecision() *aimv1alpha1.AIMPrecision {
	if s.Status == nil {
		return nil
	}
	if precision := s.Status.Profile.Metadata.Precision; precision != "" {
		p := precision
		return &p
	}
	return nil
}
