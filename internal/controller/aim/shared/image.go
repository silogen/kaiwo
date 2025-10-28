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
	"crypto/sha256"
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
// It searches only in AIMClusterModel resources.
// Returns ErrImageNotFound if no image is found in the catalog.
func LookupImageForClusterTemplate(ctx context.Context, k8sClient client.Client, modelName string) (*ImageLookupResult, error) {
	clusterImage := &aimv1alpha1.AIMClusterModel{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName}, clusterImage); err == nil {
		return &ImageLookupResult{
			Image:     clusterImage.Spec.Image,
			Resources: *clusterImage.Spec.Resources.DeepCopy(),
		}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to lookup AIMClusterModel: %w", err)
	}

	return nil, ErrImageNotFound
}

// LookupImageForNamespaceTemplate looks up the container image for a namespace-scoped template.
// It searches AIMModel resources in the specified namespace first, then falls back to
// cluster-scoped AIMClusterModel resources.
// Returns ErrImageNotFound if no image is found in either location.
func LookupImageForNamespaceTemplate(ctx context.Context, k8sClient client.Client, namespace, modelName string) (*ImageLookupResult, error) {
	// Try namespace-scoped AIMModel first
	nsImage := &aimv1alpha1.AIMModel{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName, Namespace: namespace}, nsImage); err == nil {
		return &ImageLookupResult{
			Image:     nsImage.Spec.Image,
			Resources: *nsImage.Spec.Resources.DeepCopy(),
		}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to lookup AIMModel: %w", err)
	}

	// Fall back to cluster-scoped namespace
	return LookupImageForClusterTemplate(ctx, k8sClient, modelName)
}

// ImageObservation holds the observed state for an AIMModel or AIMClusterModel.
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

	// DiscoveryEnabled reflects whether discovery is enabled from runtime config.
	// Discovery is now always attempted unless disabled by runtime config.
	DiscoveryEnabled bool

	// MetadataError captures the latest metadata format issue encountered during extraction.
	MetadataError *MetadataFormatError

	// RegistryError captures categorized registry access errors (auth, not-found, etc.).
	RegistryError *ImageRegistryError

	// MetadataExtractionErr captures non-format extraction failures (e.g., registry or auth errors).
	MetadataExtractionErr error

	// TemplatesAutoGenerated tracks whether auto-generated templates were requested this cycle.
	TemplatesAutoGenerated bool
}

// ImageObservationOptions provides callbacks for observing image state.
type ImageObservationOptions struct {
	// GetRuntimeConfig returns the runtime config for this scope (namespace or cluster).
	GetRuntimeConfig func(ctx context.Context) (*RuntimeConfigResolution, error)

	// ListOwnedTemplates returns templates owned by this image.
	ListOwnedTemplates func(ctx context.Context) ([]client.Object, error)

	// GetCurrentStatus returns the current status to check for existing conditions.
	GetCurrentStatus func() *aimv1alpha1.AIMModelStatus

	// GetImageSpec returns the image spec.
	GetImageSpec func() aimv1alpha1.AIMModelSpec
}

// ObserveImage gathers the current state for an image resource.
func ObserveImage(ctx context.Context, opts ImageObservationOptions) (*ImageObservation, error) {
	obs := &ImageObservation{}

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
		// Also reconstruct error state from conditions
		for _, cond := range status.Conditions {
			if cond.Type == aimv1alpha1.AIMModelConditionMetadataExtracted {
				obs.MetadataAlreadyAttempted = true
				if cond.Status == "True" {
					obs.MetadataExtracted = true
				} else {
					// Metadata extraction failed - reconstruct the error from the condition
					// Check if it's a registry error (ImageNotFound, ImagePullAuthFailure)
					switch cond.Reason {
					case aimv1alpha1.AIMModelReasonImageNotFound:
						obs.RegistryError = &ImageRegistryError{
							Type:    ImagePullErrorNotFound,
							Message: cond.Message,
						}
						obs.MetadataExtractionErr = obs.RegistryError
					case aimv1alpha1.AIMModelReasonImagePullAuthFailure:
						obs.RegistryError = &ImageRegistryError{
							Type:    ImagePullErrorAuth,
							Message: cond.Message,
						}
						obs.MetadataExtractionErr = obs.RegistryError
					case aimv1alpha1.AIMModelReasonMetadataExtractionFailed:
						obs.RegistryError = &ImageRegistryError{
							Type:    ImagePullErrorGeneric,
							Message: cond.Message,
						}
						obs.MetadataExtractionErr = obs.RegistryError
					default:
						// It's a metadata format error
						obs.MetadataError = &MetadataFormatError{
							Reason:  cond.Reason,
							Message: cond.Message,
						}
					}
				}
				break
			}
		}

		if status.ImageMetadata != nil {
			obs.ImageMetadata = status.ImageMetadata.DeepCopy()
		}
	}

	// Apply discovery configuration from model spec and runtime config
	// Priority: model.spec.discovery.enabled → runtime config AutoDiscovery → true (default)
	spec := opts.GetImageSpec()
	obs.DiscoveryEnabled = true // default to true

	// Check model-level discovery config first
	if spec.Discovery != nil && spec.Discovery.Enabled != nil {
		// Model-level setting overrides runtime config
		obs.DiscoveryEnabled = *spec.Discovery.Enabled
	} else {
		// No model-level override, use runtime config
		if obs.RuntimeConfigResolution != nil && obs.RuntimeConfigResolution.EffectiveSpec.Model != nil {
			if obs.RuntimeConfigResolution.EffectiveSpec.Model.AutoDiscovery != nil {
				obs.DiscoveryEnabled = *obs.RuntimeConfigResolution.EffectiveSpec.Model.AutoDiscovery
			}
		}
	}

	if !obs.DiscoveryEnabled {
		obs.MetadataAlreadyAttempted = true
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
	ImageSpec aimv1alpha1.AIMModelSpec

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

	observation := input.Observation
	if observation == nil {
		observation = &ImageObservation{}
	}
	observation.TemplatesAutoGenerated = false
	observation.MetadataExtractionErr = nil

	spec := input.ImageSpec
	if input.Observation == nil {
		// Default discovery to enabled (controlled by runtime config)
		observation.DiscoveryEnabled = true
		if observation.RuntimeConfigResolution != nil && observation.RuntimeConfigResolution.EffectiveSpec.Model != nil {
			if observation.RuntimeConfigResolution.EffectiveSpec.Model.AutoDiscovery != nil {
				observation.DiscoveryEnabled = *observation.RuntimeConfigResolution.EffectiveSpec.Model.AutoDiscovery
			}
		}
		if !observation.DiscoveryEnabled {
			observation.MetadataAlreadyAttempted = true
		}
	}

	discoveryEnabled := observation.DiscoveryEnabled
	if input.Observation == nil {
		// When observation was synthesized locally, use the value we just set
		discoveryEnabled = observation.DiscoveryEnabled
	}

	// Determine if we should attempt metadata extraction
	shouldExtract := discoveryEnabled && !observation.MetadataAlreadyAttempted

	if shouldExtract {
		baseutils.Debug(logger, "Attempting metadata extraction for image", "image", spec.Image)
		// Attempt to extract metadata
		var namespace string

		// Use the model's imagePullSecrets for metadata extraction
		imagePullSecrets := input.ImageSpec.ImagePullSecrets

		// For namespace-scoped images, use the image's namespace for secrets
		// For cluster-scoped images, use the operator namespace
		if input.IsClusterScoped {
			namespace = GetOperatorNamespace()
		} else {
			namespace = input.Namespace
		}

		metadata, err := InspectImage(
			ctx,
			spec.Image,
			imagePullSecrets,
			input.Clientset,
			namespace,
		)
		if err != nil {
			var formatErr *MetadataFormatError
			var regErr *ImageRegistryError
			if errors.As(err, &formatErr) {
				baseutils.Debug(logger, "Metadata malformed", "error", formatErr)
				observation.MetadataAlreadyAttempted = true
				observation.MetadataExtracted = false
				observation.MetadataError = formatErr
				observation.RegistryError = nil
				observation.ImageMetadata = nil
				observation.MetadataExtractionErr = nil
				return desired, nil, nil
			} else if errors.As(err, &regErr) {
				baseutils.Debug(logger, "Registry access failed",
					"error", regErr,
					"errorType", regErr.Type)
				observation.MetadataAlreadyAttempted = true
				observation.MetadataExtracted = false
				observation.MetadataError = nil
				observation.RegistryError = regErr
				observation.ImageMetadata = nil
				observation.MetadataExtractionErr = fmt.Errorf("registry access failed: %w", err)
				return desired, nil, nil
			}
			// Extraction failed - we'll track this but not block the reconciliation
			baseutils.Debug(logger, "Metadata extraction failed", "error", err)
			observation.MetadataAlreadyAttempted = true
			observation.MetadataExtracted = false
			observation.MetadataError = nil
			observation.RegistryError = nil
			observation.MetadataExtractionErr = fmt.Errorf("metadata extraction failed: %w", err)
			observation.ImageMetadata = nil
			return desired, nil, nil
		}

		observation.MetadataError = nil
		observation.MetadataExtracted = true
		observation.ImageMetadata = metadata
		observation.MetadataExtractionErr = nil

		extractedMetadata = metadata
		if metadata.Model != nil {
			baseutils.Debug(logger, "Metadata extraction succeeded",
				"hasModel", true,
				"recommendedDeployments", len(metadata.Model.RecommendedDeployments))
		} else {
			baseutils.Debug(logger, "Metadata extraction succeeded but no model metadata found")
		}
	} else if observation.ImageMetadata != nil {
		// Use existing metadata
		baseutils.Debug(logger, "Using existing metadata from observation")
		extractedMetadata = observation.ImageMetadata
	} else {
		baseutils.Debug(logger, "Skipping metadata extraction (already attempted or skipped)")
	}

	// When discovery is enabled, templates are always auto-created if recommended deployments exist
	// Only check for missing deployments if there's no registry error (404, auth, etc.) or other metadata errors
	if discoveryEnabled && observation.RegistryError == nil && observation.MetadataError == nil && observation.MetadataExtractionErr == nil {
		if extractedMetadata == nil || extractedMetadata.Model == nil || len(extractedMetadata.Model.RecommendedDeployments) == 0 {
			formatErr := newMetadataFormatError(
				"MetadataMissingRecommendedDeployments",
				"auto template creation requires image label \"org.amd.silogen.model.recommendedDeployments\" with at least one entry",
			)
			observation.MetadataAlreadyAttempted = true
			observation.MetadataExtracted = false
			observation.MetadataError = formatErr
			observation.RegistryError = nil
			observation.ImageMetadata = nil
			observation.TemplatesAutoGenerated = false
			observation.MetadataExtractionErr = nil
			return desired, extractedMetadata, nil
		}
	}

	// If we have metadata with recommended deployments, create templates
	// Check both discoveryEnabled and autoCreateTemplates setting
	shouldCreateTemplates := discoveryEnabled
	if spec.Discovery != nil && spec.Discovery.AutoCreateTemplates != nil {
		// Explicit autoCreateTemplates setting overrides discoveryEnabled
		shouldCreateTemplates = *spec.Discovery.AutoCreateTemplates
	}

	// Only create templates if we have valid metadata
	if shouldCreateTemplates && extractedMetadata != nil && extractedMetadata.Model != nil {
		createdTemplates := false
		for _, deployment := range extractedMetadata.Model.RecommendedDeployments {
			template := buildServiceTemplateFromDeployment(
				input.ImageName,
				input.Namespace,
				deployment,
				input.OwnerReference,
				input.IsClusterScoped,
				input.ImageSpec.ImagePullSecrets,
				input.ImageSpec.ServiceAccountName,
			)
			desired = append(desired, template)
			createdTemplates = true
		}
		if createdTemplates {
			observation.TemplatesAutoGenerated = true
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
	imagePullSecrets []corev1.LocalObjectReference,
	serviceAccountName string,
) client.Object {
	// Generate template name using the specified format
	templateName := generateTemplateName(imageName, deployment)

	// Build common spec
	commonSpec := aimv1alpha1.AIMServiceTemplateSpecCommon{
		ModelName:          imageName,
		ImagePullSecrets:   imagePullSecrets,
		ServiceAccountName: serviceAccountName,
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
		commonSpec.GpuSelector = &aimv1alpha1.AIMGpuSelector{
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

// metricShorthand maps metric values to their abbreviated forms for template naming
var metricShorthand = map[string]string{
	"latency":    "lat",
	"throughput": "thr",
}

// getMetricShorthand returns the abbreviated form of a metric, or the original if no mapping exists
func getMetricShorthand(metric string) string {
	if shorthand, ok := metricShorthand[metric]; ok {
		return shorthand
	}
	return metric
}

// generateTemplateName creates an RFC1123-compliant name for a service template.
// Format: {truncated-image}-{count}x-{gpu}-{metric-shorthand}-{precision}-{hash4}
//
// The name is constructed to ensure uniqueness while preserving readability:
// - Image name is truncated to fit within the 63-character Kubernetes limit
// - Profile parameters (GPU count, model, metric, precision) are always preserved
// - A 4-character hash suffix ensures uniqueness even when truncation occurs
//
// Example: llama-3-1-70b-instruct-1x-mi300x-lat-fp8-a7f3
func generateTemplateName(imageName string, deployment aimv1alpha1.RecommendedDeployment) string {
	const (
		maxLen         = 63
		hashLength     = 4
		separatorCount = 5 // hyphens between components
	)

	// Build the distinguishing suffix components
	var suffixParts []string

	// Format: {count}x-{gpu}
	if deployment.GPUCount > 0 && deployment.GPUModel != "" {
		suffixParts = append(suffixParts, fmt.Sprintf("%dx-%s", deployment.GPUCount, deployment.GPUModel))
	} else if deployment.GPUModel != "" {
		suffixParts = append(suffixParts, deployment.GPUModel)
	} else if deployment.GPUCount > 0 {
		suffixParts = append(suffixParts, fmt.Sprintf("x%d", deployment.GPUCount))
	}

	// Add metric with shorthand
	if deployment.Metric != "" {
		suffixParts = append(suffixParts, getMetricShorthand(deployment.Metric))
	}

	// Add precision
	if deployment.Precision != "" {
		suffixParts = append(suffixParts, deployment.Precision)
	}

	// Create deterministic hash from all components for uniqueness
	hashInput := imageName
	if deployment.GPUModel != "" {
		hashInput += "-" + deployment.GPUModel
	}
	if deployment.GPUCount > 0 {
		hashInput += fmt.Sprintf("-x%d", deployment.GPUCount)
	}
	if deployment.Metric != "" {
		hashInput += "-" + deployment.Metric
	}
	if deployment.Precision != "" {
		hashInput += "-" + deployment.Precision
	}

	hash := sha256.Sum256([]byte(hashInput))
	hashSuffix := fmt.Sprintf("%x", hash[:])[:hashLength]
	suffixParts = append(suffixParts, hashSuffix)

	// Build the complete suffix
	suffix := strings.Join(suffixParts, "-")

	// Calculate how much space is available for the image name
	// Account for the hyphen between image name and suffix
	reservedLen := len(suffix) + 1 // +1 for hyphen separator
	maxImageLen := maxLen - reservedLen

	if maxImageLen < 1 {
		maxImageLen = 1
	}

	// Truncate image name to fit
	truncatedImage := imageName
	if len(truncatedImage) > maxImageLen {
		truncatedImage = truncatedImage[:maxImageLen]
	}

	// Combine image name and suffix
	name := truncatedImage + "-" + suffix

	// Make RFC1123 compliant (lowercase, valid chars, trim invalid endings)
	return baseutils.MakeRFC1123Compliant(name)
}

// handleMetadataExtraction sets the MetadataExtracted condition based on extraction results.
// Returns true if metadata was successfully extracted.
func handleMetadataExtraction(
	status *aimv1alpha1.AIMModelStatus,
	extractedMetadata *aimv1alpha1.ImageMetadata,
	extractionErr error,
	observation *ImageObservation,
	observedGeneration int64,
) (metadataOK bool, registryErr *ImageRegistryError, metadataFormatErr *MetadataFormatError) {
	if extractedMetadata != nil {
		status.ImageMetadata = extractedMetadata
		setCondition(&status.Conditions, metav1.Condition{
			Type:               aimv1alpha1.AIMModelConditionMetadataExtracted,
			Status:             metav1.ConditionTrue,
			Reason:             aimv1alpha1.AIMModelReasonMetadataExtracted,
			Message:            "Image metadata successfully extracted",
			ObservedGeneration: observedGeneration,
		})
		return true, nil, nil
	}

	if extractionErr != nil {
		if errors.As(extractionErr, &metadataFormatErr) {
			setCondition(&status.Conditions, metav1.Condition{
				Type:               aimv1alpha1.AIMModelConditionMetadataExtracted,
				Status:             metav1.ConditionFalse,
				Reason:             metadataFormatErr.Reason,
				Message:            metadataFormatErr.Error(),
				ObservedGeneration: observedGeneration,
			})
			return false, nil, metadataFormatErr
		}
		if errors.As(extractionErr, &registryErr) {
			setMetadataExtractionCondition(status, registryErr, observedGeneration)
			return false, registryErr, nil
		}
		setCondition(&status.Conditions, metav1.Condition{
			Type:               aimv1alpha1.AIMModelConditionMetadataExtracted,
			Status:             metav1.ConditionFalse,
			Reason:             aimv1alpha1.AIMModelReasonMetadataExtractionFailed,
			Message:            fmt.Sprintf("Failed to extract metadata: %v", extractionErr),
			ObservedGeneration: observedGeneration,
		})
		return false, nil, nil
	}

	// Check observation for errors
	if observation != nil {
		if observation.MetadataError != nil {
			setCondition(&status.Conditions, metav1.Condition{
				Type:               aimv1alpha1.AIMModelConditionMetadataExtracted,
				Status:             metav1.ConditionFalse,
				Reason:             observation.MetadataError.Reason,
				Message:            observation.MetadataError.Error(),
				ObservedGeneration: observedGeneration,
			})
			return false, nil, observation.MetadataError
		}
		if observation.RegistryError != nil {
			setMetadataExtractionCondition(status, observation.RegistryError, observedGeneration)
			return false, observation.RegistryError, nil
		}
	}

	return false, nil, nil
}

// setMetadataExtractionCondition sets the MetadataExtracted condition based on registry error type.
func setMetadataExtractionCondition(status *aimv1alpha1.AIMModelStatus, registryErr *ImageRegistryError, observedGeneration int64) {
	var reason string
	switch registryErr.Type {
	case ImagePullErrorAuth:
		reason = aimv1alpha1.AIMModelReasonImagePullAuthFailure
	case ImagePullErrorNotFound:
		reason = aimv1alpha1.AIMModelReasonImageNotFound
	default:
		reason = aimv1alpha1.AIMModelReasonMetadataExtractionFailed
	}
	setCondition(&status.Conditions, metav1.Condition{
		Type:               aimv1alpha1.AIMModelConditionMetadataExtracted,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            registryErr.Error(),
		ObservedGeneration: observedGeneration,
	})
}

// handleRegistryError marks the model as degraded or failed when registry access fails.
// Not found errors (404) are terminal and result in Failed status.
// Auth errors and other registry errors are potentially recoverable and result in Degraded status.
func handleRegistryError(
	status *aimv1alpha1.AIMModelStatus,
	registryErr *ImageRegistryError,
	observedGeneration int64,
	setAutoCondition func(metav1.ConditionStatus, string, string),
) {
	var reason string
	switch registryErr.Type {
	case ImagePullErrorAuth:
		reason = aimv1alpha1.AIMModelReasonImagePullAuthFailure
	case ImagePullErrorNotFound:
		reason = aimv1alpha1.AIMModelReasonImageNotFound
	default:
		reason = aimv1alpha1.AIMModelReasonMetadataExtractionFailed
	}

	setAutoCondition(metav1.ConditionFalse, reason, registryErr.Error())
	status.ImageMetadata = nil

	// Not found (404) is a terminal error - the image will never exist without changing the spec
	// Set to Failed instead of Degraded
	if registryErr.Type == ImagePullErrorNotFound {
		status.Status = aimv1alpha1.AIMModelStatusFailed
	} else {
		// Auth errors and other registry errors are potentially recoverable
		status.Status = aimv1alpha1.AIMModelStatusDegraded
	}

	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            registryErr.Error(),
		ObservedGeneration: observedGeneration,
	})
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Degraded",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            registryErr.Error(),
		ObservedGeneration: observedGeneration,
	})
}

// handleAutoTemplateStatus determines the auto-template generation status and sets appropriate conditions.
func handleAutoTemplateStatus(
	status *aimv1alpha1.AIMModelStatus,
	observation *ImageObservation,
	extractedMetadata *aimv1alpha1.ImageMetadata,
	metadataCompleted bool,
	discoveryEnabled bool,
	observedGeneration int64,
	setAutoCondition func(metav1.ConditionStatus, string, string),
) {
	existingAutoTemplates := observation != nil && len(observation.ExistingTemplates) > 0
	generatedAutoTemplates := observation != nil && observation.TemplatesAutoGenerated

	if existingAutoTemplates {
		setAutoCondition(metav1.ConditionTrue, "AutoTemplatesPresent", "Auto-generated templates present for this image")
		updateImageStatusFromTemplates(status, observation.ExistingTemplates, observedGeneration)
		return
	}

	if !discoveryEnabled {
		if generatedAutoTemplates {
			setAutoCondition(metav1.ConditionTrue, "AutoTemplatesPresent", "Auto-generated templates present for this image")
		} else {
			setAutoCondition(metav1.ConditionFalse, "DiscoveryDisabled", "Discovery disabled; auto-generated templates not created")
		}
		markImageReady(status, "DiscoveryDisabled", "Discovery disabled; no templates to manage", observedGeneration)
		return
	}

	if !metadataCompleted {
		setAutoCondition(metav1.ConditionFalse, "AutoTemplateCreationPending", "Auto template creation awaiting metadata extraction")
		markImageProgressing(status, "DiscoveryInProgress", "Discovery enabled; awaiting metadata extraction", observedGeneration)
		return
	}

	recommended := 0
	if extractedMetadata != nil && extractedMetadata.Model != nil {
		recommended = len(extractedMetadata.Model.RecommendedDeployments)
	} else if observation != nil && observation.ImageMetadata != nil && observation.ImageMetadata.Model != nil {
		recommended = len(observation.ImageMetadata.Model.RecommendedDeployments)
	}

	if recommended == 0 {
		if generatedAutoTemplates {
			setAutoCondition(metav1.ConditionTrue, "AutoTemplatesPresent", "Auto-generated templates present for this image")
		} else {
			setAutoCondition(metav1.ConditionFalse, "NoRecommendedTemplates", "Discovery completed with no recommended templates")
		}
		markImageReady(status, "NoRecommendedTemplates", "Discovery completed with no recommended templates", observedGeneration)
	} else {
		if generatedAutoTemplates {
			setAutoCondition(metav1.ConditionTrue, "RecommendedDeploymentsApplied", "Auto-generated templates from image recommended deployments")
		} else {
			setAutoCondition(metav1.ConditionFalse, "AutoTemplateCreationPending", fmt.Sprintf("%d template(s) pending creation", recommended))
		}
		markImageProgressing(status, "TemplateCreationPending", fmt.Sprintf("%d template(s) pending creation", recommended), observedGeneration)
	}
}

// ProjectImageStatus updates the status of an image resource based on observation and errors.
func ProjectImageStatus(
	status *aimv1alpha1.AIMModelStatus,
	spec aimv1alpha1.AIMModelSpec,
	observation *ImageObservation,
	extractedMetadata *aimv1alpha1.ImageMetadata,
	extractionErr error,
	observedGeneration int64,
) {
	logger := ctrl.Log.WithName("ProjectImageStatus")

	// Debug logging to understand what we're receiving
	logger.Info("ProjectImageStatus called",
		"image", spec.Image,
		"hasExtractedMetadata", extractedMetadata != nil,
		"hasExtractionErr", extractionErr != nil,
		"hasObservation", observation != nil)

	if observation != nil {
		logger.Info("Observation state",
			"hasRegistryError", observation.RegistryError != nil,
			"hasMetadataError", observation.MetadataError != nil,
			"hasMetadataExtractionErr", observation.MetadataExtractionErr != nil,
			"metadataExtracted", observation.MetadataExtracted,
			"metadataAlreadyAttempted", observation.MetadataAlreadyAttempted)

		if observation.RegistryError != nil {
			logger.Info("Registry error present in observation",
				"errorType", observation.RegistryError.Type,
				"message", observation.RegistryError.Message)
		}
		if observation.MetadataError != nil {
			logger.Info("Metadata error present in observation",
				"reason", observation.MetadataError.Reason,
				"message", observation.MetadataError.Message)
		}
	}

	status.ObservedGeneration = observedGeneration

	setAutoCondition := func(condStatus metav1.ConditionStatus, reason, message string) {
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "TemplatesAutoGenerated",
			Status:             condStatus,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: observedGeneration,
		})
	}

	// Handle metadata extraction and get any errors
	_, registryErr, metadataFormatErr := handleMetadataExtraction(status, extractedMetadata, extractionErr, observation, observedGeneration)

	logger.Info("After handleMetadataExtraction",
		"hasRegistryErr", registryErr != nil,
		"hasMetadataFormatErr", metadataFormatErr != nil)

	if registryErr != nil {
		logger.Info("Registry error detected", "type", registryErr.Type, "message", registryErr.Message)
	}
	if metadataFormatErr != nil {
		logger.Info("Metadata format error detected", "reason", metadataFormatErr.Reason, "message", metadataFormatErr.Message)
	}

	// Handle metadata format errors - mark as Failed since we can't use the image
	if metadataFormatErr != nil {
		setAutoCondition(metav1.ConditionFalse, metadataFormatErr.Reason, metadataFormatErr.Error())
		status.ImageMetadata = nil
		status.Status = aimv1alpha1.AIMModelStatusFailed
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             metadataFormatErr.Reason,
			Message:            metadataFormatErr.Error(),
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Degraded",
			Status:             metav1.ConditionTrue,
			Reason:             metadataFormatErr.Reason,
			Message:            metadataFormatErr.Error(),
			ObservedGeneration: observedGeneration,
		})
		return
	}

	// Handle registry errors (auth, not found, etc.)
	if registryErr != nil {
		handleRegistryError(status, registryErr, observedGeneration, setAutoCondition)
		return
	}

	// Determine if discovery is enabled and metadata is complete
	discoveryEnabled := true
	if observation != nil {
		discoveryEnabled = observation.DiscoveryEnabled
	}

	metadataCompleted := extractedMetadata != nil
	if !metadataCompleted && observation != nil {
		metadataCompleted = observation.MetadataExtracted
	}

	// Handle auto-template generation status
	handleAutoTemplateStatus(status, observation, extractedMetadata, metadataCompleted, discoveryEnabled, observedGeneration, setAutoCondition)
}

// updateImageStatusFromTemplates aggregates template statuses and sets conditions on the image.
// Logic:
// - Ready: All templates are Available or NotAvailable (terminal non-problematic states)
// - Progressing: At least one template is Progressing (and none are Failed/Degraded)
// - Degraded: One or more templates are Degraded or Failed (but not all)
// - Failed: All templates are Degraded or Failed
func updateImageStatusFromTemplates(status *aimv1alpha1.AIMModelStatus, templates []client.Object, observedGeneration int64) {
	if status == nil {
		return
	}

	// Default to Pending when no templates exist or none provide a definitive state.
	status.Status = aimv1alpha1.AIMModelStatusPending

	if len(templates) == 0 {
		return
	}

	var availableCount, notAvailableCount, progressingCount, degradedCount, failedCount int
	var messages []string

	// Count templates by status
	for _, tmpl := range templates {
		var templateStatus aimv1alpha1.AIMTemplateStatusEnum
		var templateName string

		switch t := tmpl.(type) {
		case *aimv1alpha1.AIMServiceTemplate:
			templateStatus = t.Status.Status
			templateName = t.Name
		case *aimv1alpha1.AIMClusterServiceTemplate:
			templateStatus = t.Status.Status
			templateName = t.Name
		default:
			continue
		}

		switch templateStatus {
		case aimv1alpha1.AIMTemplateStatusReady:
			availableCount++
		case aimv1alpha1.AIMTemplateStatusNotAvailable:
			notAvailableCount++
		case aimv1alpha1.AIMTemplateStatusProgressing, aimv1alpha1.AIMTemplateStatusPending:
			progressingCount++
		case aimv1alpha1.AIMTemplateStatusDegraded:
			degradedCount++
			messages = append(messages, fmt.Sprintf("template %q is degraded", templateName))
		case aimv1alpha1.AIMTemplateStatusFailed:
			failedCount++
			messages = append(messages, fmt.Sprintf("template %q has failed", templateName))
		}
	}

	totalTemplates := len(templates)
	problemCount := degradedCount + failedCount
	readyCount := availableCount + notAvailableCount

	// Determine overall status based on template states
	if problemCount == totalTemplates {
		// All templates are degraded or failed
		status.Status = aimv1alpha1.AIMModelStatusFailed

		msg := fmt.Sprintf("All %d template(s) are degraded or failed", totalTemplates)
		if len(messages) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(messages, "; "))
		}
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "AllTemplatesFailed",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Degraded",
			Status:             metav1.ConditionTrue,
			Reason:             "AllTemplatesFailed",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
	} else if problemCount > 0 {
		// Some templates are degraded or failed
		status.Status = aimv1alpha1.AIMModelStatusDegraded

		msg := fmt.Sprintf("%d of %d template(s) are degraded or failed", problemCount, totalTemplates)
		if len(messages) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(messages, "; "))
		}
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SomeTemplatesDegraded",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Degraded",
			Status:             metav1.ConditionTrue,
			Reason:             "SomeTemplatesDegraded",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
	} else if progressingCount > 0 {
		// At least one template is progressing
		status.Status = aimv1alpha1.AIMModelStatusProgressing

		msg := fmt.Sprintf("%d of %d template(s) are progressing", progressingCount, totalTemplates)
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "TemplatesProgressing",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionTrue,
			Reason:             "TemplatesProgressing",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Degraded",
			Status:             metav1.ConditionFalse,
			Reason:             "TemplatesProgressing",
			Message:            "No templates are degraded",
			ObservedGeneration: observedGeneration,
		})
	} else if readyCount == totalTemplates {
		// All templates are in a ready state (available or not available due to missing GPU)
		status.Status = aimv1alpha1.AIMModelStatusReady

		var msg string
		if notAvailableCount > 0 {
			msg = fmt.Sprintf("%d template(s) available, %d not available (GPU not in cluster)", availableCount, notAvailableCount)
		} else {
			msg = fmt.Sprintf("All %d template(s) are available", totalTemplates)
		}
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "AllTemplatesReady",
			Message:            msg,
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionFalse,
			Reason:             "AllTemplatesReady",
			Message:            "All templates have completed discovery",
			ObservedGeneration: observedGeneration,
		})
		setCondition(&status.Conditions, metav1.Condition{
			Type:               "Degraded",
			Status:             metav1.ConditionFalse,
			Reason:             "AllTemplatesReady",
			Message:            "No templates are degraded",
			ObservedGeneration: observedGeneration,
		})
	}
}

func markImageReady(status *aimv1alpha1.AIMModelStatus, reason, message string, observedGeneration int64) {
	if status == nil {
		return
	}
	status.Status = aimv1alpha1.AIMModelStatusReady
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Degraded",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            "No templates are degraded",
		ObservedGeneration: observedGeneration,
	})
}

func markImageProgressing(status *aimv1alpha1.AIMModelStatus, reason, message string, observedGeneration int64) {
	if status == nil {
		return
	}
	status.Status = aimv1alpha1.AIMModelStatusProgressing
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
	setCondition(&status.Conditions, metav1.Condition{
		Type:               "Degraded",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            "No templates are degraded",
		ObservedGeneration: observedGeneration,
	})
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
