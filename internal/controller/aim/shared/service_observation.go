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
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

const templateNameMaxLength = 63

// TemplateScope indicates whether a template is namespace-scoped, cluster-scoped, or unresolved.
type TemplateScope string

const (
	TemplateScopeNone      TemplateScope = ""
	TemplateScopeNamespace TemplateScope = "namespace"
	TemplateScopeCluster   TemplateScope = "cluster"
)

// TemplateResolution captures the result of resolving a template name for a service.
type TemplateResolution struct {
	BaseName  string
	FinalName string
	Derived   bool
}

// ServiceObservation holds observed state for an AIMService reconciliation.
type ServiceObservation struct {
	TemplateName           string
	BaseTemplateName       string
	Scope                  TemplateScope
	TemplateAvailable      bool
	TemplateOwnedByService bool
	ShouldCreateTemplate   bool
	RuntimeConfigSpec      aimv1alpha1.AIMRuntimeConfigSpec
	EffectiveRuntimeConfig *aimv1alpha1.AIMEffectiveRuntimeConfig
	RoutePath              string
	RouteTemplateErr       error
	RuntimeConfigErr       error
	TemplateStatus         *aimv1alpha1.AIMServiceTemplateStatus
	TemplateSpecCommon     aimv1alpha1.AIMServiceTemplateSpecCommon
	TemplateSpec           *aimv1alpha1.AIMServiceTemplateSpec
	TemplateNamespace      string
	ImageResources         *corev1.ResourceRequirements
}

// TemplateFound returns true if a template was resolved (namespace or cluster scope).
func (o *ServiceObservation) TemplateFound() bool {
	return o != nil && o.Scope != TemplateScopeNone
}

// RuntimeName returns the effective runtime name for the service.
func (o *ServiceObservation) RuntimeName() string {
	if o == nil {
		return ""
	}
	return o.TemplateName
}

// ResolveTemplateNameForService determines the template name to use for a service.
// It handles default template lookup, base template resolution, and derived template naming.
func ResolveTemplateNameForService(ctx context.Context, k8sClient client.Client, service *aimv1alpha1.AIMService) (TemplateResolution, error) {
	var res TemplateResolution

	baseName := strings.TrimSpace(service.Spec.TemplateRef)
	if baseName == "" {
		defaultTemplate, err := LookupDefaultServiceTemplate(ctx, k8sClient, service)
		if err != nil {
			return res, err
		}
		if defaultTemplate != "" {
			baseName = defaultTemplate
		} else {
			baseName = service.Name
		}
	}

	res.BaseName = baseName
	res.Derived = service.Spec.Overrides != nil

	if res.Derived {
		suffix := OverridesSuffix(service.Spec.Overrides)
		if suffix != "" {
			res.FinalName = DerivedTemplateName(baseName, suffix)
		} else {
			res.FinalName = baseName
		}
	} else {
		res.FinalName = baseName
	}

	return res, nil
}

// OverridesSuffix computes a hash suffix for service overrides.
func OverridesSuffix(overrides *aimv1alpha1.AIMServiceOverrides) string {
	if overrides == nil {
		return ""
	}

	bytes, err := json.Marshal(overrides)
	if err != nil {
		return ""
	}

	sum := sha1.Sum(bytes)
	return fmt.Sprintf("%x", sum[:])[:8]
}

// DerivedTemplateName constructs a template name from a base name and suffix.
// Ensures the final name does not exceed Kubernetes name length limits.
func DerivedTemplateName(baseName, suffix string) string {
	if suffix == "" {
		return baseName
	}

	extra := "-ovr-" + suffix
	maxBaseLen := templateNameMaxLength - len(extra)
	if maxBaseLen <= 0 {
		maxBaseLen = 1
	}

	trimmed := baseName
	if len(trimmed) > maxBaseLen {
		trimmed = strings.TrimRight(trimmed[:maxBaseLen], "-")
		if trimmed == "" {
			trimmed = baseName[:maxBaseLen]
		}
	}

	return fmt.Sprintf("%s%s", trimmed, extra)
}

// LookupDefaultServiceTemplate searches for a default template name from the service's AIMImage or AIMClusterImage.
func LookupDefaultServiceTemplate(ctx context.Context, k8sClient client.Client, service *aimv1alpha1.AIMService) (string, error) {
	imageName := strings.TrimSpace(service.Spec.AIMImageName)
	if imageName == "" {
		return "", nil
	}

	if service.Namespace != "" {
		var nsImage aimv1alpha1.AIMImage
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: imageName, Namespace: service.Namespace}, &nsImage); err == nil {
			if tpl := strings.TrimSpace(nsImage.Spec.DefaultServiceTemplate); tpl != "" {
				return tpl, nil
			}
		} else if !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to get AIMImage %s/%s: %w", service.Namespace, imageName, err)
		}
	}

	var clusterImage aimv1alpha1.AIMClusterImage
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: imageName}, &clusterImage); err == nil {
		if tpl := strings.TrimSpace(clusterImage.Spec.DefaultServiceTemplate); tpl != "" {
			return tpl, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get AIMClusterImage %s: %w", imageName, err)
	}

	return "", nil
}

// LoadBaseTemplateSpec fetches the base template spec for a derived template.
// Searches namespace-scoped templates first, then falls back to cluster-scoped templates.
func LoadBaseTemplateSpec(ctx context.Context, k8sClient client.Client, service *aimv1alpha1.AIMService, baseName string) (*aimv1alpha1.AIMServiceTemplateSpec, TemplateScope, error) {
	if baseName == "" {
		return nil, TemplateScopeNone, fmt.Errorf("base template name is empty")
	}

	if service.Namespace != "" {
		var namespaceTemplate aimv1alpha1.AIMServiceTemplate
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: service.Namespace, Name: baseName}, &namespaceTemplate); err == nil {
			return namespaceTemplate.Spec.DeepCopy(), TemplateScopeNamespace, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, TemplateScopeNone, err
		}
	}

	var clusterTemplate aimv1alpha1.AIMClusterServiceTemplate
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: baseName}, &clusterTemplate); err == nil {
		spec := &aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: clusterTemplate.Spec.AIMServiceTemplateSpecCommon,
		}
		return spec, TemplateScopeCluster, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, TemplateScopeNone, err
	}

	return nil, TemplateScopeNone, fmt.Errorf("base template %q not found", baseName)
}

// PopulateObservationFromNamespaceTemplate extracts data from a namespace-scoped template into the observation.
func PopulateObservationFromNamespaceTemplate(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	template *aimv1alpha1.AIMServiceTemplate,
	obs *ServiceObservation,
) error {
	obs.Scope = TemplateScopeNamespace
	obs.TemplateAvailable = template.Status.Status == aimv1alpha1.AIMTemplateStatusAvailable
	obs.TemplateOwnedByService = HasOwnerReference(template.GetOwnerReferences(), service.UID)
	if template.Status.EffectiveRuntimeConfig != nil {
		obs.EffectiveRuntimeConfig = template.Status.EffectiveRuntimeConfig.DeepCopy()
	}
	obs.TemplateStatus = template.Status.DeepCopy()
	obs.TemplateSpecCommon = template.Spec.AIMServiceTemplateSpecCommon
	obs.TemplateSpec = template.Spec.DeepCopy()
	runtimeConfigName := RuntimeConfigNameForService(service, obs.TemplateSpecCommon)
	obs.TemplateSpecCommon.RuntimeConfigName = runtimeConfigName
	if resolution, resolveErr := ResolveRuntimeConfig(ctx, k8sClient, service.Namespace, runtimeConfigName); resolveErr != nil {
		if errors.Is(resolveErr, ErrRuntimeConfigNotFound) {
			obs.RuntimeConfigErr = fmt.Errorf("AIMRuntimeConfig %q not found in namespace %q", runtimeConfigName, service.Namespace)
		} else {
			return fmt.Errorf("failed to resolve AIMRuntimeConfig %q in namespace %q: %w", runtimeConfigName, service.Namespace, resolveErr)
		}
	} else {
		obs.RuntimeConfigSpec = resolution.EffectiveSpec
		if resolution.EffectiveStatus != nil {
			obs.EffectiveRuntimeConfig = resolution.EffectiveStatus
		}
	}
	obs.TemplateNamespace = template.Namespace
	if image, imageErr := LookupImageForNamespaceTemplate(ctx, k8sClient, template.Namespace, template.Spec.AIMImageName); imageErr == nil {
		obs.ImageResources = image.Resources.DeepCopy()
	} else if imageErr != nil && !errors.Is(imageErr, ErrImageNotFound) {
		return fmt.Errorf("failed to lookup AIMImage %q in namespace %q: %w", template.Spec.AIMImageName, template.Namespace, imageErr)
	}
	return nil
}

// PopulateObservationFromClusterTemplate extracts data from a cluster-scoped template into the observation.
func PopulateObservationFromClusterTemplate(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	template *aimv1alpha1.AIMClusterServiceTemplate,
	obs *ServiceObservation,
) error {
	obs.Scope = TemplateScopeCluster
	obs.TemplateAvailable = template.Status.Status == aimv1alpha1.AIMTemplateStatusAvailable
	if template.Status.EffectiveRuntimeConfig != nil {
		obs.EffectiveRuntimeConfig = template.Status.EffectiveRuntimeConfig.DeepCopy()
	}
	obs.TemplateStatus = template.Status.DeepCopy()
	obs.TemplateSpecCommon = template.Spec.AIMServiceTemplateSpecCommon
	obs.TemplateSpec = &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: template.Spec.AIMServiceTemplateSpecCommon,
	}
	runtimeConfigName := RuntimeConfigNameForService(service, obs.TemplateSpecCommon)
	obs.TemplateSpecCommon.RuntimeConfigName = runtimeConfigName
	if resolution, resolveErr := ResolveRuntimeConfig(ctx, k8sClient, service.Namespace, runtimeConfigName); resolveErr == nil {
		obs.RuntimeConfigSpec = resolution.EffectiveSpec
		if resolution.EffectiveStatus != nil {
			obs.EffectiveRuntimeConfig = resolution.EffectiveStatus
		}
	} else if errors.Is(resolveErr, ErrRuntimeConfigNotFound) {
		obs.RuntimeConfigErr = fmt.Errorf("AIMRuntimeConfig %q not found in namespace %q", runtimeConfigName, service.Namespace)
	} else {
		return fmt.Errorf("failed to resolve AIMRuntimeConfig %q in namespace %q: %w", runtimeConfigName, service.Namespace, resolveErr)
	}
	if image, imageErr := LookupImageForClusterTemplate(ctx, k8sClient, template.Spec.AIMImageName); imageErr == nil {
		obs.ImageResources = image.Resources.DeepCopy()
	} else if !errors.Is(imageErr, ErrImageNotFound) {
		return fmt.Errorf("failed to lookup AIMClusterImage %q: %w", template.Spec.AIMImageName, imageErr)
	}
	return nil
}

// RuntimeConfigNameForService determines the effective runtime config name for a service.
func RuntimeConfigNameForService(service *aimv1alpha1.AIMService, templateSpec aimv1alpha1.AIMServiceTemplateSpecCommon) string {
	name := service.Spec.RuntimeConfigName
	if name == "" {
		name = templateSpec.RuntimeConfigName
	}
	return NormalizeRuntimeConfigName(name)
}

// ObserveDerivedTemplate handles observation for services with derived templates.
// It fetches the derived template if it exists, or loads the base template spec for creation.
func ObserveDerivedTemplate(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	resolution TemplateResolution,
	obs *ServiceObservation,
) error {
	var namespaceTemplate aimv1alpha1.AIMServiceTemplate
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: service.Namespace,
		Name:      resolution.FinalName,
	}, &namespaceTemplate)

	switch {
	case err == nil:
		// Derived template exists, populate observation from it
		return PopulateObservationFromNamespaceTemplate(ctx, k8sClient, service, &namespaceTemplate, obs)

	case apierrors.IsNotFound(err):
		// Derived template doesn't exist yet, load base template spec for creation
		return loadBaseTemplateForDerivedCreation(ctx, k8sClient, service, resolution.BaseName, obs)

	default:
		return fmt.Errorf("failed to get AIMServiceTemplate %s/%s: %w", service.Namespace, resolution.FinalName, err)
	}
}

// loadBaseTemplateForDerivedCreation loads the base template spec and prepares observation for derived template creation.
func loadBaseTemplateForDerivedCreation(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	baseName string,
	obs *ServiceObservation,
) error {
	baseSpec, baseScope, err := LoadBaseTemplateSpec(ctx, k8sClient, service, baseName)
	if err != nil {
		return err
	}

	if baseSpec == nil {
		obs.ShouldCreateTemplate = true
		return nil
	}

	obs.TemplateSpec = baseSpec
	obs.TemplateSpecCommon = baseSpec.AIMServiceTemplateSpecCommon

	// Resolve runtime config
	runtimeConfigName := RuntimeConfigNameForService(service, obs.TemplateSpecCommon)
	obs.TemplateSpecCommon.RuntimeConfigName = runtimeConfigName

	if err := resolveRuntimeConfigForObservation(ctx, k8sClient, service.Namespace, runtimeConfigName, obs); err != nil {
		return err
	}

	// Lookup image resources based on base scope
	if err := lookupImageResourcesForScope(ctx, k8sClient, service.Namespace, baseSpec.AIMImageName, baseScope, obs); err != nil {
		return err
	}

	obs.ShouldCreateTemplate = true
	return nil
}

// ObserveNonDerivedTemplate handles observation for services with non-derived templates.
// It searches for namespace-scoped templates first, then falls back to cluster-scoped templates.
func ObserveNonDerivedTemplate(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	templateName string,
	obs *ServiceObservation,
) error {
	// Try namespace-scoped template first
	var namespaceTemplate aimv1alpha1.AIMServiceTemplate
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: service.Namespace,
		Name:      templateName,
	}, &namespaceTemplate)

	switch {
	case err == nil:
		return PopulateObservationFromNamespaceTemplate(ctx, k8sClient, service, &namespaceTemplate, obs)

	case apierrors.IsNotFound(err):
		// Fall back to cluster-scoped template
		return observeClusterTemplate(ctx, k8sClient, service, templateName, obs)

	default:
		return fmt.Errorf("failed to get AIMServiceTemplate %s/%s: %w", service.Namespace, templateName, err)
	}
}

// observeClusterTemplate attempts to fetch and populate observation from a cluster-scoped template.
func observeClusterTemplate(
	ctx context.Context,
	k8sClient client.Client,
	service *aimv1alpha1.AIMService,
	templateName string,
	obs *ServiceObservation,
) error {
	var clusterTemplate aimv1alpha1.AIMClusterServiceTemplate
	err := k8sClient.Get(ctx, client.ObjectKey{Name: templateName}, &clusterTemplate)

	switch {
	case err == nil:
		return PopulateObservationFromClusterTemplate(ctx, k8sClient, service, &clusterTemplate, obs)

	case apierrors.IsNotFound(err):
		obs.ShouldCreateTemplate = true
		return nil

	default:
		return fmt.Errorf("failed to get AIMClusterServiceTemplate %s: %w", templateName, err)
	}
}

// resolveRuntimeConfigForObservation resolves the runtime config and updates the observation.
func resolveRuntimeConfigForObservation(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	runtimeConfigName string,
	obs *ServiceObservation,
) error {
	resolution, err := ResolveRuntimeConfig(ctx, k8sClient, namespace, runtimeConfigName)
	if err != nil {
		if errors.Is(err, ErrRuntimeConfigNotFound) {
			obs.RuntimeConfigErr = fmt.Errorf("AIMRuntimeConfig %q not found in namespace %q", runtimeConfigName, namespace)
			return nil
		}
		return fmt.Errorf("failed to resolve AIMRuntimeConfig %q in namespace %q: %w", runtimeConfigName, namespace, err)
	}

	obs.RuntimeConfigSpec = resolution.EffectiveSpec
	if resolution.EffectiveStatus != nil {
		obs.EffectiveRuntimeConfig = resolution.EffectiveStatus
	}
	return nil
}

// lookupImageResourcesForScope looks up image resources based on template scope.
func lookupImageResourcesForScope(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	imageName string,
	scope TemplateScope,
	obs *ServiceObservation,
) error {
	var image *ImageLookupResult
	var err error

	switch scope {
	case TemplateScopeNamespace:
		image, err = LookupImageForNamespaceTemplate(ctx, k8sClient, namespace, imageName)
		if err != nil && !errors.Is(err, ErrImageNotFound) {
			return fmt.Errorf("failed to lookup AIMImage %q in namespace %q: %w", imageName, namespace, err)
		}

	case TemplateScopeCluster:
		image, err = LookupImageForClusterTemplate(ctx, k8sClient, imageName)
		if err != nil && !errors.Is(err, ErrImageNotFound) {
			return fmt.Errorf("failed to lookup AIMClusterImage %q: %w", imageName, err)
		}
	}

	if image != nil {
		obs.ImageResources = image.Resources.DeepCopy()
	}
	return nil
}
