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
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// ErrImageNotFound is returned when an image is not found in the catalog
var ErrImageNotFound = errors.New("image not found in catalog")

// ImageLookupResult captures the resolved image metadata from the catalog.
type ImageLookupResult struct {
	Image     string
	Resources corev1.ResourceRequirements
}

// DeepCopy returns a deep copy of the ImageLookupResult.
func (r *ImageLookupResult) DeepCopy() *ImageLookupResult {
	if r == nil {
		return nil
	}
	result := &ImageLookupResult{
		Image: r.Image,
	}
	result.Resources = *r.Resources.DeepCopy()
	return result
}

// LookupImageForClusterTemplate looks up the container image for a cluster-scoped template.
// It searches only in AIMClusterImage resources.
// Returns ErrImageNotFound if no image is found in the catalog.
func LookupImageForClusterTemplate(ctx context.Context, k8sClient client.Client, modelName string) (*ImageLookupResult, error) {
	clusterImage := &aimv1alpha1.AIMClusterImage{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName}, clusterImage); err == nil {
		return &ImageLookupResult{
			Image:     clusterImage.Spec.Image,
			Resources: *clusterImage.Spec.Resources.DeepCopy(),
		}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to lookup AIMClusterImage: %w", err)
	}

	return nil, ErrImageNotFound
}

// LookupImageForNamespaceTemplate looks up the container image for a namespace-scoped template.
// It searches AIMImage resources in the specified namespace first, then falls back to
// cluster-scoped AIMClusterImage resources.
// Returns ErrImageNotFound if no image is found in either location.
func LookupImageForNamespaceTemplate(ctx context.Context, k8sClient client.Client, namespace, modelName string) (*ImageLookupResult, error) {
	// Try namespace-scoped AIMImage first
	nsImage := &aimv1alpha1.AIMImage{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName, Namespace: namespace}, nsImage); err == nil {
		return &ImageLookupResult{
			Image:     nsImage.Spec.Image,
			Resources: *nsImage.Spec.Resources.DeepCopy(),
		}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to lookup AIMImage: %w", err)
	}

	// Fall back to cluster-scoped namespace
	return LookupImageForClusterTemplate(ctx, k8sClient, modelName)
}

// ImageObservation holds the observed state for an AIMImage or AIMClusterImage.
type ImageObservation struct {
	// MetadataAlreadyAttempted is true if we've already attempted metadata extraction.
	MetadataAlreadyAttempted bool

	// MetadataExtracted is true if metadata was successfully extracted.
	MetadataExtracted bool

	// ImageMetadata contains the extracted metadata (if extraction succeeded).
	ImageMetadata *aimv1alpha1.ImageMetadata

	// RuntimeConfigResolution contains the resolved runtime config (for image pull secrets).
	RuntimeConfigResolution *RuntimeConfigResolution

	// ExistingTemplates are the ServiceTemplates currently owned by this image.
	ExistingTemplates []client.Object
}

// ImageObservationOptions provides callbacks for observing image state.
type ImageObservationOptions struct {
	// GetRuntimeConfig returns the runtime config for this scope (namespace or cluster).
	GetRuntimeConfig func(ctx context.Context) (*RuntimeConfigResolution, error)

	// ListOwnedTemplates returns templates owned by this image.
	ListOwnedTemplates func(ctx context.Context) ([]client.Object, error)

	// GetCurrentStatus returns the current status to check for existing conditions.
	GetCurrentStatus func() *aimv1alpha1.AIMImageStatus

	// GetImageSpec returns the image spec.
	GetImageSpec func() aimv1alpha1.AIMImageSpec

	// GetLabels returns the image resource labels.
	GetLabels func() map[string]string
}

// ObserveImage gathers the current state for an image resource.
func ObserveImage(ctx context.Context, opts ImageObservationOptions) (*ImageObservation, error) {
	obs := &ImageObservation{}

	// Check if inspection should be skipped via label
	if opts.GetLabels != nil {
		labels := opts.GetLabels()
		if labels[LabelKeySkipInspection] == "true" {
			// Mark as already attempted to skip inspection
			obs.MetadataAlreadyAttempted = true
		}
	}

	// Resolve runtime config for image pull secrets
	if opts.GetRuntimeConfig != nil {
		runtimeConfig, err := opts.GetRuntimeConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve runtime config: %w", err)
		}
		obs.RuntimeConfigResolution = runtimeConfig
	}

	// Check if we've already attempted metadata extraction
	if opts.GetCurrentStatus != nil {
		status := opts.GetCurrentStatus()
		obs.MetadataExtracted = status.ImageMetadata != nil

		// Check conditions to see if we already attempted and failed
		for _, cond := range status.Conditions {
			if cond.Type == aimv1alpha1.AIMImageConditionMetadataExtracted {
				obs.MetadataAlreadyAttempted = true
				if cond.Status == "True" {
					obs.MetadataExtracted = true
				}
				break
			}
		}

		if status.ImageMetadata != nil {
			obs.ImageMetadata = status.ImageMetadata.DeepCopy()
		}
	}

	// List existing owned templates
	if opts.ListOwnedTemplates != nil {
		templates, err := opts.ListOwnedTemplates(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list owned templates: %w", err)
		}
		obs.ExistingTemplates = templates
	}

	return obs, nil
}

// ImagePlanInput provides the input for planning image resources.
type ImagePlanInput struct {
	// ImageName is the name of the image resource.
	ImageName string

	// Namespace is the namespace (empty for cluster-scoped).
	Namespace string

	// ImageSpec is the image specification.
	ImageSpec aimv1alpha1.AIMImageSpec

	// Observation is the observed state.
	Observation *ImageObservation

	// OwnerReference for created templates.
	OwnerReference []metav1.OwnerReference

	// Clientset for image inspection.
	Clientset kubernetes.Interface

	// IsClusterScoped indicates if this is a cluster-scoped image.
	IsClusterScoped bool
}

// PlanImageResources plans the desired state for an image resource.
// It performs metadata extraction if needed and creates ServiceTemplates based on recommendedDeployments.
func PlanImageResources(ctx context.Context, input ImagePlanInput) ([]client.Object, *aimv1alpha1.ImageMetadata, error) {
	logger := ctrl.LoggerFrom(ctx)
	var desired []client.Object
	var extractedMetadata *aimv1alpha1.ImageMetadata

	// Determine if we should attempt metadata extraction
	shouldExtract := !input.Observation.MetadataAlreadyAttempted

	if shouldExtract {
		baseutils.Debug(logger, "Attempting metadata extraction for image", "image", input.ImageSpec.Image)
		// Attempt to extract metadata
		var imagePullSecrets []corev1.LocalObjectReference
		var namespace string

		if input.Observation.RuntimeConfigResolution != nil {
			imagePullSecrets = input.Observation.RuntimeConfigResolution.EffectiveSpec.ImagePullSecrets
		}

		// For namespace-scoped images, use the image's namespace for secrets
		// For cluster-scoped images, use the operator namespace
		if input.IsClusterScoped {
			namespace = GetOperatorNamespace()
		} else {
			namespace = input.Namespace
		}

		metadata, err := InspectImage(
			ctx,
			input.ImageSpec.Image,
			imagePullSecrets,
			input.Clientset,
			namespace,
		)
		if err != nil {
			// Extraction failed - we'll track this but not block the reconciliation
			baseutils.Debug(logger, "Metadata extraction failed", "error", err)
			return desired, nil, fmt.Errorf("metadata extraction failed: %w", err)
		}

		extractedMetadata = metadata
		if metadata.Model != nil {
			baseutils.Debug(logger, "Metadata extraction succeeded",
				"hasModel", true,
				"recommendedDeployments", len(metadata.Model.RecommendedDeployments))
		} else {
			baseutils.Debug(logger, "Metadata extraction succeeded but no model metadata found")
		}
	} else if input.Observation.ImageMetadata != nil {
		// Use existing metadata
		baseutils.Debug(logger, "Using existing metadata from observation")
		extractedMetadata = input.Observation.ImageMetadata
	} else {
		baseutils.Debug(logger, "Skipping metadata extraction (already attempted or skipped)")
	}

	// If we have metadata with recommended deployments, create templates
	if extractedMetadata != nil && extractedMetadata.Model != nil && len(extractedMetadata.Model.RecommendedDeployments) > 0 {
		for _, deployment := range extractedMetadata.Model.RecommendedDeployments {
			template := buildServiceTemplateFromDeployment(
				input.ImageName,
				input.Namespace,
				deployment,
				input.OwnerReference,
				input.IsClusterScoped,
			)
			desired = append(desired, template)
		}
	}

	return desired, extractedMetadata, nil
}

// buildServiceTemplateFromDeployment creates a ServiceTemplate from a recommended deployment configuration.
func buildServiceTemplateFromDeployment(
	imageName string,
	namespace string,
	deployment aimv1alpha1.RecommendedDeployment,
	ownerRefs []metav1.OwnerReference,
	isClusterScoped bool,
) client.Object {
	// Generate template name using the specified format
	templateName := generateTemplateName(imageName, deployment)

	// Build common spec
	commonSpec := aimv1alpha1.AIMServiceTemplateSpecCommon{
		AIMImageName: imageName,
	}

	// Set runtime parameters from deployment
	if deployment.Metric != "" {
		metric := aimv1alpha1.AIMMetric(deployment.Metric)
		commonSpec.Metric = &metric
	}
	if deployment.Precision != "" {
		precision := aimv1alpha1.AIMPrecision(deployment.Precision)
		commonSpec.Precision = &precision
	}
	if deployment.GPUModel != "" && deployment.GPUCount > 0 {
		commonSpec.GpuSelector = &aimv1alpha1.AimGpuSelector{
			Model: deployment.GPUModel,
			Count: deployment.GPUCount,
		}
	}

	// Common labels
	labels := map[string]string{
		LabelKeyAutoGenerated: LabelValueAutoGenerated,
		LabelKeyImageName:     imageName,
	}

	if isClusterScoped {
		// Create AIMClusterServiceTemplate
		template := &aimv1alpha1.AIMClusterServiceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:            templateName,
				Labels:          labels,
				OwnerReferences: ownerRefs,
			},
			Spec: aimv1alpha1.AIMClusterServiceTemplateSpec{
				AIMServiceTemplateSpecCommon: commonSpec,
			},
		}
		return template
	}

	// Create namespace-scoped AIMServiceTemplate
	template := &aimv1alpha1.AIMServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            templateName,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: commonSpec,
		},
	}
	return template
}

// generateTemplateName creates an RFC1123-compliant name for a service template.
// Format: {image-name}-{gpuModel}-x{gpuCount}-{metric}-{precision}
func generateTemplateName(imageName string, deployment aimv1alpha1.RecommendedDeployment) string {
	parts := []string{imageName}

	if deployment.GPUModel != "" {
		parts = append(parts, deployment.GPUModel)
	}
	if deployment.GPUCount > 0 {
		parts = append(parts, fmt.Sprintf("x%d", deployment.GPUCount))
	}
	if deployment.Metric != "" {
		parts = append(parts, deployment.Metric)
	}
	if deployment.Precision != "" {
		parts = append(parts, deployment.Precision)
	}

	// Join with hyphens and make RFC1123 compliant
	name := strings.Join(parts, "-")
	return baseutils.MakeRFC1123Compliant(name)
}

// ProjectImageStatus updates the status of an image resource based on observation and errors.
func ProjectImageStatus(
	status *aimv1alpha1.AIMImageStatus,
	observation *ImageObservation,
	extractedMetadata *aimv1alpha1.ImageMetadata,
	extractionErr error,
	observedGeneration int64,
) {
	status.ObservedGeneration = observedGeneration

	// Update image metadata if we have extracted metadata
	if extractedMetadata != nil {
		status.ImageMetadata = extractedMetadata
		// Extraction succeeded - set condition
		setCondition(&status.Conditions, metav1.Condition{
			Type:               aimv1alpha1.AIMImageConditionMetadataExtracted,
			Status:             metav1.ConditionTrue,
			Reason:             aimv1alpha1.AIMImageReasonMetadataExtracted,
			Message:            "Image metadata successfully extracted",
			ObservedGeneration: observedGeneration,
		})
	} else if extractionErr != nil {
		// Extraction failed
		setCondition(&status.Conditions, metav1.Condition{
			Type:               aimv1alpha1.AIMImageConditionMetadataExtracted,
			Status:             metav1.ConditionFalse,
			Reason:             aimv1alpha1.AIMImageReasonMetadataExtractionFailed,
			Message:            fmt.Sprintf("Failed to extract metadata: %v", extractionErr),
			ObservedGeneration: observedGeneration,
		})
	}
	// If both are nil, don't set any condition (extraction not attempted or skipped)
}

// setCondition adds or updates a condition in the conditions list.
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	if conditions == nil {
		return
	}

	// Set LastTransitionTime if not already set
	now := metav1.Now()
	if newCondition.LastTransitionTime.IsZero() {
		newCondition.LastTransitionTime = now
	}

	// Find existing condition
	for i := range *conditions {
		if (*conditions)[i].Type == newCondition.Type {
			// Only update LastTransitionTime if status changed
			if (*conditions)[i].Status != newCondition.Status {
				newCondition.LastTransitionTime = now
			} else {
				// Keep the original transition time if status didn't change
				newCondition.LastTransitionTime = (*conditions)[i].LastTransitionTime
			}
			// Update existing condition
			(*conditions)[i] = newCondition
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, newCondition)
}
