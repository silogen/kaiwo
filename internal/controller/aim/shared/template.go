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

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// TemplateObservation holds the common observed state for both template types
type TemplateObservation struct {
	Job                *batchv1.Job
	Image              string
	ImageResources     *corev1.ResourceRequirements
	ImagePullSecrets   []corev1.LocalObjectReference
	ServiceAccountName string
	RuntimeConfig      *RuntimeConfigResolution
	TemplateCaches     *aimv1alpha1.AIMTemplateCacheList
	GPUModel           string
	GPUAvailable       bool
	GPUChecked         bool
	JobPodImageError   *ImagePullError // Categorized image pull error if job pod is stuck
}

// TemplateSpec provides the common template specification
type TemplateSpec interface {
	GetModelName() string
	GetSpecModelSources() []aimv1alpha1.AIMModelSource
}

// TemplateWithStatus extends TemplateSpec with status access
type TemplateWithStatus interface {
	TemplateSpec
	client.Object
	GetStatus() *aimv1alpha1.AIMServiceTemplateStatus
}

// RuntimeObservation combines TemplateObservation with a controller-specific runtime object.
type RuntimeObservation[R client.Object] struct {
	Runtime R
	TemplateObservation
}

// TemplateObservationOptions configures ObserveTemplate behaviour.
type TemplateObservationOptions[R client.Object] struct {
	K8sClient               client.Client // Required for pod status checking
	GetRuntime              func(ctx context.Context) (R, error)
	ShouldCheckDiscoveryJob bool
	GetDiscoveryJob         func(ctx context.Context) (*batchv1.Job, error)
	GetJobNamespace         func() string // Namespace where the job runs (for pod lookup)
	LookupImage             func(ctx context.Context) (*ImageLookupResult, error)
	ResolveRuntimeConfig    func(ctx context.Context) (*RuntimeConfigResolution, error)
	OnRuntimeConfigResolved func(resolution *RuntimeConfigResolution)
	GetImagePullSecrets     func() []corev1.LocalObjectReference // Template's imagePullSecrets
	GetServiceAccountName   func() string                        // Template's serviceAccountName
	GetTemplateCaches       func(ctx context.Context) (*aimv1alpha1.AIMTemplateCacheList, error)
}

// ObserveTemplate gathers runtime, discovery job, image, and runtime config information with common error handling.
func ObserveTemplate[R client.Object](ctx context.Context, opts TemplateObservationOptions[R]) (*RuntimeObservation[R], error) {
	obs := &RuntimeObservation[R]{}

	if opts.GetRuntime != nil {
		runtime, err := opts.GetRuntime(ctx)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			obs.Runtime = runtime
		}
	}

	if opts.ShouldCheckDiscoveryJob && opts.GetDiscoveryJob != nil {
		job, err := opts.GetDiscoveryJob(ctx)
		if err != nil {
			return nil, err
		}
		obs.Job = job

		// If we have a job and it's not complete, check if its pod is stuck in ImagePullBackOff
		if job != nil && !IsJobComplete(job) && opts.GetJobNamespace != nil && opts.K8sClient != nil {
			obs.JobPodImageError = checkJobPodImagePullStatus(ctx, opts.K8sClient, job, opts.GetJobNamespace)
		}
	}

	if opts.LookupImage != nil {
		image, err := opts.LookupImage(ctx)
		if err != nil {
			// For image not found errors, continue with empty image
			// This allows status projection to handle it gracefully
			if !errors.Is(err, ErrImageNotFound) {
				return nil, err
			}
			// obs.Image remains empty string
		} else if image != nil {
			obs.Image = image.Image
			obs.ImageResources = image.Resources.DeepCopy()
		}
	}

	if opts.ResolveRuntimeConfig != nil {
		resolution, err := opts.ResolveRuntimeConfig(ctx)
		if err != nil {
			return nil, err
		}
		if resolution != nil {
			obs.RuntimeConfig = resolution
			if opts.OnRuntimeConfigResolved != nil {
				opts.OnRuntimeConfigResolved(resolution)
			}
		}
	}

	// Get template's imagePullSecrets
	if opts.GetImagePullSecrets != nil {
		obs.ImagePullSecrets = opts.GetImagePullSecrets()
	}

	// Get template's serviceAccountName
	if opts.GetServiceAccountName != nil {
		obs.ServiceAccountName = opts.GetServiceAccountName()
	}

	if opts.GetTemplateCaches != nil {
		cache, err := opts.GetTemplateCaches(ctx)
		if err != nil {
			return nil, err
		}
		obs.TemplateCaches = cache

	}

	return obs, nil
}

// TemplatePlanContext provides metadata needed during plan generation.
type TemplatePlanContext struct {
	Ctx         context.Context
	Client      client.Client
	Template    metav1.Object
	APIVersion  string
	Kind        string
	Status      aimv1alpha1.AIMTemplateStatusEnum
	Observation *TemplateObservation
}

// TemplatePlanInput supplies builders with convenient access to observation data.
type TemplatePlanInput struct {
	Observation       *TemplateObservation
	RuntimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec
	OwnerReference    metav1.OwnerReference
}

// TemplatePlanBuilders specifies how to render runtime and discovery job objects.
type TemplatePlanBuilders struct {
	BuildRuntime      func(input TemplatePlanInput) client.Object
	BuildDiscoveryJob func(input TemplatePlanInput) client.Object
}

// CountActiveDiscoveryJobs counts the number of active (non-complete) discovery jobs across all namespaces.
// A job is considered active if it exists and is not in a complete state (succeeded or failed).
func CountActiveDiscoveryJobs(ctx context.Context, k8sClient client.Client) (int, error) {
	logger := ctrl.LoggerFrom(ctx)

	// List all jobs with discovery job labels across all namespaces
	var jobs batchv1.JobList
	if err := k8sClient.List(ctx, &jobs, client.MatchingLabels{
		"app.kubernetes.io/component":  LabelValueDiscoveryComponent,
		"app.kubernetes.io/managed-by": LabelValueManagedBy,
	}); err != nil {
		return 0, fmt.Errorf("failed to list discovery jobs: %w", err)
	}

	// Count jobs that are not complete (not succeeded and not failed)
	activeCount := 0
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if !IsJobComplete(job) {
			activeCount++
			baseutils.Debug(logger, "Found active discovery job",
				"namespace", job.Namespace,
				"name", job.Name,
				"active", job.Status.Active,
				"succeeded", job.Status.Succeeded,
				"failed", job.Status.Failed)
		}
	}

	baseutils.Debug(logger, "Discovery job count", "active", activeCount, "total", len(jobs.Items))
	return activeCount, nil
}

// ImagePullErrorType categorizes image pull errors
type ImagePullErrorType string

const (
	ImagePullErrorAuth     ImagePullErrorType = "auth"
	ImagePullErrorNotFound ImagePullErrorType = "not-found"
	ImagePullErrorGeneric  ImagePullErrorType = "generic"
)

// Kubernetes container status reasons
const (
	containerStatusReasonImagePullBackOff = "ImagePullBackOff"
	containerStatusReasonErrImagePull     = "ErrImagePull"
	containerStatusReasonImageNotFound    = "ImageNotFound"
)

// ImagePullError contains categorized information about an image pull failure
type ImagePullError struct {
	Type            ImagePullErrorType
	Container       string
	Reason          string // e.g., "ImagePullBackOff", "ErrImagePull"
	Message         string // Full error message from Kubernetes
	IsInitContainer bool
}

// categorizeImagePullError analyzes an error message to determine if it's auth-related or not-found
func categorizeImagePullError(message string) ImagePullErrorType {
	lowerMsg := strings.ToLower(message)

	// Check for authentication/authorization errors
	authIndicators := []string{
		"unauthorized",
		"authentication required",
		"authentication failed",
		"401",
		"403",
		"forbidden",
		"denied",
		"permission denied",
		"access denied",
		"credentials",
	}
	for _, indicator := range authIndicators {
		if strings.Contains(lowerMsg, indicator) {
			return ImagePullErrorAuth
		}
	}

	// Check for not-found errors
	notFoundIndicators := []string{
		"not found",
		"404",
		"manifest unknown",
		"name unknown",
		"image not found",
	}
	for _, indicator := range notFoundIndicators {
		if strings.Contains(lowerMsg, indicator) {
			return ImagePullErrorNotFound
		}
	}

	return ImagePullErrorGeneric
}

// checkJobPodImagePullStatus checks if a job's pod is stuck in ImagePullBackOff or ErrImagePull state.
// Returns the image pull error details if found, or nil otherwise.
func checkJobPodImagePullStatus(ctx context.Context, k8sClient client.Client, job *batchv1.Job, getNamespace func() string) *ImagePullError {
	logger := ctrl.LoggerFrom(ctx)
	namespace := getNamespace()

	// List pods owned by this job
	var pods corev1.PodList
	if err := k8sClient.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"job-name": job.Name,
		}); err != nil {
		baseutils.Debug(logger, "Failed to list pods for job",
			"job", job.Name,
			"namespace", namespace,
			"error", err)
		return nil
	}

	// Check each pod for ImagePullBackOff or ErrImagePull status
	for i := range pods.Items {
		pod := &pods.Items[i]

		// Check init container statuses
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == containerStatusReasonImagePullBackOff || reason == containerStatusReasonErrImagePull {
					message := containerStatus.State.Waiting.Message
					pullError := &ImagePullError{
						Type:            categorizeImagePullError(message),
						Container:       containerStatus.Name,
						Reason:          reason,
						Message:         message,
						IsInitContainer: true,
					}
					baseutils.Debug(logger, "Found image pull failure",
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", reason,
						"errorType", pullError.Type)
					return pullError
				}
			}
		}

		// Check main container statuses
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == containerStatusReasonImagePullBackOff || reason == containerStatusReasonErrImagePull {
					message := containerStatus.State.Waiting.Message
					pullError := &ImagePullError{
						Type:            categorizeImagePullError(message),
						Container:       containerStatus.Name,
						Reason:          reason,
						Message:         message,
						IsInitContainer: false,
					}
					baseutils.Debug(logger, "Found image pull failure",
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", reason,
						"errorType", pullError.Type)
					return pullError
				}
			}
		}
	}

	return nil
}

// CheckInferenceServicePodImagePullStatus checks if an InferenceService's pods are stuck in ImagePullBackOff or ErrImagePull state.
// It looks for pods with the isvc.serving.kserve.io/inferenceservice label matching the InferenceService name.
// Returns the image pull error details if found, or nil otherwise.
func CheckInferenceServicePodImagePullStatus(ctx context.Context, k8sClient client.Client, inferenceServiceName, namespace string) *ImagePullError {
	logger := ctrl.LoggerFrom(ctx)

	// List pods owned by this InferenceService
	var pods corev1.PodList
	if err := k8sClient.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"serving.kserve.io/inferenceservice": inferenceServiceName,
		}); err != nil {
		baseutils.Debug(logger, "Failed to list pods for InferenceService",
			"inferenceService", inferenceServiceName,
			"namespace", namespace,
			"error", err)
		return nil
	}

	// Check each pod for ImagePullBackOff or ErrImagePull status
	for i := range pods.Items {
		pod := &pods.Items[i]

		// Check init container statuses
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					message := containerStatus.State.Waiting.Message
					pullError := &ImagePullError{
						Type:            categorizeImagePullError(message),
						Container:       containerStatus.Name,
						Reason:          reason,
						Message:         message,
						IsInitContainer: true,
					}
					baseutils.Debug(logger, "Found image pull failure in InferenceService pod",
						"inferenceService", inferenceServiceName,
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", reason,
						"errorType", pullError.Type)
					return pullError
				}
			}
		}

		// Check main container statuses
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					message := containerStatus.State.Waiting.Message
					pullError := &ImagePullError{
						Type:            categorizeImagePullError(message),
						Container:       containerStatus.Name,
						Reason:          reason,
						Message:         message,
						IsInitContainer: false,
					}
					baseutils.Debug(logger, "Found image pull failure in InferenceService pod",
						"inferenceService", inferenceServiceName,
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", reason,
						"errorType", pullError.Type)
					return pullError
				}
			}
		}
	}

	return nil
}

// PlanTemplateResources produces desired objects based on the observation and controller-provided builders.
// It respects the global limit on concurrent discovery jobs (MaxConcurrentDiscoveryJobs).
// Returns the desired objects and a boolean indicating if a requeue is needed (when job limit is reached).
func PlanTemplateResources(ctx TemplatePlanContext, builders TemplatePlanBuilders) ([]client.Object, bool) {
	if ctx.Observation != nil && ctx.Observation.GPUChecked && !ctx.Observation.GPUAvailable {
		return nil, false
	}

	if ctx.Observation == nil || ctx.Observation.Image == "" {
		return nil, false
	}

	logger := ctrl.LoggerFrom(ctx.Ctx)

	runtimeConfigSpec := aimv1alpha1.AIMRuntimeConfigSpec{}
	if ctx.Observation.RuntimeConfig != nil {
		runtimeConfigSpec = ctx.Observation.RuntimeConfig.EffectiveSpec
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         ctx.APIVersion,
		Kind:               ctx.Kind,
		Name:               ctx.Template.GetName(),
		UID:                ctx.Template.GetUID(),
		Controller:         baseutils.Pointer(true),
		BlockOwnerDeletion: baseutils.Pointer(true),
	}

	input := TemplatePlanInput{
		Observation:       ctx.Observation,
		RuntimeConfigSpec: runtimeConfigSpec,
		OwnerReference:    ownerRef,
	}

	var desired []client.Object

	// Only create the ServingRuntime after discovery has completed successfully
	if ctx.Status == aimv1alpha1.AIMTemplateStatusReady && builders.BuildRuntime != nil {
		if runtime := builders.BuildRuntime(input); runtime != nil {
			desired = append(desired, runtime)
		}
	}

	// Create discovery job if template is not yet Available and job hasn't completed
	if ctx.Status != aimv1alpha1.AIMTemplateStatusReady &&
		builders.BuildDiscoveryJob != nil &&
		(ctx.Observation.Job == nil || !IsJobComplete(ctx.Observation.Job)) {

		// Check if we've reached the global limit for concurrent discovery jobs
		if ctx.Client != nil {
			activeJobCount, err := CountActiveDiscoveryJobs(ctx.Ctx, ctx.Client)
			if err != nil {
				baseutils.Debug(logger, "Failed to count active discovery jobs, proceeding anyway",
					"error", err,
					"template", ctx.Template.GetName())
			} else if activeJobCount >= MaxConcurrentDiscoveryJobs {
				baseutils.Debug(logger, "Discovery job limit reached, deferring job creation",
					"template", ctx.Template.GetName(),
					"activeJobs", activeJobCount,
					"limit", MaxConcurrentDiscoveryJobs)
				// Don't create the job - signal that we need to requeue
				return desired, true
			} else {
				baseutils.Debug(logger, "Discovery job limit not reached, proceeding with job creation",
					"template", ctx.Template.GetName(),
					"activeJobs", activeJobCount,
					"limit", MaxConcurrentDiscoveryJobs)
			}
		}

		if job := builders.BuildDiscoveryJob(input); job != nil {
			// Propagate labels from template to discovery job based on runtime config
			if parentObj, ok := ctx.Template.(client.Object); ok {
				PropagateLabels(parentObj, job, &input.RuntimeConfigSpec.AIMRuntimeConfigCommon)
			}
			desired = append(desired, job)
		}
	}

	return desired, false
}

// FormatRuntimeConfigSources renders a human-readable list of runtime config sources for logging/events.
func FormatRuntimeConfigSources(resolution *RuntimeConfigResolution, namespaceLabel string) []string {
	if resolution == nil {
		return nil
	}

	var sources []string
	if resolution.NamespaceConfig != nil {
		ns := namespaceLabel
		if ns == "" {
			ns = resolution.Namespace
		}
		sources = append(sources, fmt.Sprintf("namespace/%s", ns))
	}
	if resolution.ClusterConfig != nil {
		sources = append(sources, "cluster")
	}
	return sources
}

// JoinRuntimeConfigSources joins runtime config sources for concise logging.
func JoinRuntimeConfigSources(resolution *RuntimeConfigResolution, namespaceLabel string) string {
	sources := FormatRuntimeConfigSources(resolution, namespaceLabel)
	return strings.Join(sources, ", ")
}

// templateStatusResult encapsulates the status, conditions, and additional fields to set.
type templateStatusResult struct {
	Status       aimv1alpha1.AIMTemplateStatusEnum
	Conditions   []metav1.Condition
	ModelSources []aimv1alpha1.AIMModelSource
	Profile      *aimv1alpha1.AIMProfile
}

// applyTemplateStatusResult applies the computed status result to the template.
func applyTemplateStatusResult(templateStatus *aimv1alpha1.AIMServiceTemplateStatus, result templateStatusResult) {
	templateStatus.Status = result.Status
	for _, cond := range result.Conditions {
		meta.SetStatusCondition(&templateStatus.Conditions, cond)
	}
	if len(result.ModelSources) > 0 {
		templateStatus.ModelSources = result.ModelSources
	}
	if result.Profile != nil {
		templateStatus.Profile = *result.Profile
	}
}

// handleTemplateReconcileErrors processes reconciliation errors and returns appropriate status.
func handleTemplateReconcileErrors(errs controllerutils.ReconcileErrors, imageNotFoundMessage string) *templateStatusResult {
	if !errs.HasError() {
		return nil
	}

	status := aimv1alpha1.AIMTemplateStatusFailed
	var conditions []metav1.Condition

	if errs.ObserveErr != nil {
		if errors.Is(errs.ObserveErr, ErrImageNotFound) {
			status = aimv1alpha1.AIMTemplateStatusDegraded
			conditions = append(conditions,
				controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, "ImageNotFound", imageNotFoundMessage),
				controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, "ImageNotFound", "Cannot proceed without image"),
			)
		} else {
			conditions = append(conditions,
				controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, controllerutils.ReasonFailed, fmt.Sprintf("Observation failed: %v", errs.ObserveErr)),
				controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, controllerutils.ReasonFailed, "Template is not ready due to errors"),
			)
		}
	}

	if errs.ApplyErr != nil {
		conditions = append(conditions,
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, controllerutils.ReasonFailed, fmt.Sprintf("Apply failed: %v", errs.ApplyErr)),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, controllerutils.ReasonFailed, "Template is not ready due to errors"),
		)
	}

	return &templateStatusResult{Status: status, Conditions: conditions}
}

// handleTemplateMissingImage handles the case where the image is missing.
func handleTemplateMissingImage(obs *TemplateObservation, imageNotFoundMessage string) *templateStatusResult {
	if obs.Image != "" {
		return nil
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusDegraded,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, containerStatusReasonImageNotFound, imageNotFoundMessage),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, containerStatusReasonImageNotFound, "Cannot proceed without image"),
		},
	}
}

// handleTemplateImagePullFailed handles the case where the discovery job pod is stuck in ImagePullBackOff.
func handleTemplateImagePullFailed(obs *TemplateObservation, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if obs == nil || obs.JobPodImageError == nil {
		return nil
	}

	pullErr := obs.JobPodImageError

	// Determine the condition reason based on error type
	var conditionReason string
	var eventReason string
	switch pullErr.Type {
	case ImagePullErrorAuth:
		conditionReason = "ImagePullAuthFailure"
		eventReason = "ImagePullAuthFailed"
	case ImagePullErrorNotFound:
		conditionReason = containerStatusReasonImageNotFound
		eventReason = containerStatusReasonImageNotFound
	default:
		conditionReason = containerStatusReasonImagePullBackOff
		eventReason = "ImagePullFailed"
	}

	// Format detailed message
	containerType := "Container"
	if pullErr.IsInitContainer {
		containerType = "Init container"
	}
	detailedMessage := fmt.Sprintf("%s %q is stuck in %s: %s", containerType, pullErr.Container, pullErr.Reason, pullErr.Message)

	// Emit warning event about image pull failure
	controllerutils.EmitWarningEvent(recorder, template, eventReason,
		fmt.Sprintf("Discovery job pod cannot pull image: %s", detailedMessage))

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusDegraded,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, conditionReason,
				fmt.Sprintf("Discovery job pod stuck pulling image: %s", detailedMessage)),
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionFalse, conditionReason,
				"Discovery cannot proceed due to image pull failure"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, conditionReason,
				"Template is not ready due to image pull failure"),
		},
	}
}

// handleTemplateGPUUnavailable handles the case where the required GPU is not available in the cluster.
func handleTemplateGPUUnavailable(
	obs *TemplateObservation,
	currentStatus aimv1alpha1.AIMTemplateStatusEnum,
	recorder record.EventRecorder,
	template TemplateWithStatus,
) *templateStatusResult {
	if obs == nil || !obs.GPUChecked || obs.GPUAvailable {
		return nil
	}

	message := "Required GPU model is not available in the cluster"
	if obs.GPUModel != "" {
		message = fmt.Sprintf("Required GPU model %q is not available in the cluster", obs.GPUModel)
	}

	if recorder != nil && currentStatus != aimv1alpha1.AIMTemplateStatusNotAvailable {
		controllerutils.EmitWarningEvent(recorder, template, "GPUUnavailable", message)
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusNotAvailable,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionFalse, "GPUUnavailable", message),
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionFalse, "GPUUnavailable", "Waiting for required GPU resources"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, "GPUUnavailable", message),
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionFalse, "GPUUnavailable", "Cannot run discovery without required GPU resources"),
		},
	}
}

// handleTemplateDiscoveryRunning handles the case where discovery job is running.
func handleTemplateDiscoveryRunning(obs *TemplateObservation, currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if obs.Job == nil || IsJobComplete(obs.Job) {
		return nil
	}

	// Emit event if transitioning from Pending to Progressing
	if currentStatus == aimv1alpha1.AIMTemplateStatusPending {
		controllerutils.EmitNormalEvent(recorder, template, "DiscoveryStarted", "Discovery job is running")
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusProgressing,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionTrue, controllerutils.ReasonDiscoveryRunning, "Discovery job is running"),
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionFalse, controllerutils.ReasonJobPending, "Discovery job has not completed"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, controllerutils.ReasonReconciling, "Template is not ready yet"),
		},
	}
}

// handleTemplateDiscoveryFailed handles the case where discovery job failed.
func handleTemplateDiscoveryFailed(obs *TemplateObservation, currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if obs.Job == nil || !IsJobFailed(obs.Job) {
		return nil
	}

	// Emit event if transitioning from Progressing to Failed
	if currentStatus == aimv1alpha1.AIMTemplateStatusProgressing {
		controllerutils.EmitWarningEvent(recorder, template, "DiscoveryFailed", "Discovery job failed")
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusFailed,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, controllerutils.ReasonJobFailed, "Discovery job failed"),
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionFalse, controllerutils.ReasonDiscoveryFailed, "Discovery failed"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, controllerutils.ReasonFailed, "Template is not ready"),
		},
	}
}

// handleTemplateDiscoverySucceeded handles the case where discovery job succeeded.
func handleTemplateDiscoverySucceeded(ctx context.Context, k8sClient client.Client, clientset kubernetes.Interface, obs *TemplateObservation, currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if obs.Job == nil || !IsJobSucceeded(obs.Job) {
		return nil
	}

	// Emit event if transitioning from Progressing to Available
	if currentStatus == aimv1alpha1.AIMTemplateStatusProgressing {
		controllerutils.EmitNormalEvent(recorder, template, "DiscoverySucceeded", "Model sources discovered successfully")
	}

	// Parse discovery results
	discovery, err := ParseDiscoveryLogs(ctx, k8sClient, clientset, obs.Job)
	if err != nil {
		controllerutils.EmitWarningEvent(recorder, template, "DiscoveryParseFailed", fmt.Sprintf("Failed to parse discovery output: %v", err))
		return &templateStatusResult{
			Status: aimv1alpha1.AIMTemplateStatusFailed,
			Conditions: []metav1.Condition{
				controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, "DiscoveryParseFailed", fmt.Sprintf("Failed to parse discovery output: %v", err)),
				controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, "DiscoveryParseFailed", "Template is not ready"),
			},
		}
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusReady,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionTrue, controllerutils.ReasonDiscovered, "Model sources discovered"),
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionFalse, controllerutils.ReasonAvailable, "Discovery complete"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionTrue, controllerutils.ReasonAvailable, "Template is ready"),
		},
		ModelSources: discovery.ModelSources,
		Profile:      discovery.Profile,
	}
}

// handleTemplateWithInlineModelSources handles the case where modelSources are provided in-line in the spec.
// When modelSources are provided, discovery is skipped and the template is marked as Ready immediately.
func handleTemplateWithInlineModelSources(specModelSources []aimv1alpha1.AIMModelSource, currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if len(specModelSources) == 0 {
		return nil
	}

	// Emit event if transitioning to Ready for the first time
	if currentStatus != aimv1alpha1.AIMTemplateStatusReady {
		controllerutils.EmitNormalEvent(recorder, template, "ModelSourcesProvided", "Using in-line model sources, skipping discovery")
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusReady,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionTrue, "InlineModelSources", "Model sources provided in-line in spec"),
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionFalse, controllerutils.ReasonAvailable, "No discovery needed"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionTrue, controllerutils.ReasonAvailable, "Template is ready"),
		},
		ModelSources: specModelSources,
	}
}

// handleTemplateInitialState handles the case where no discovery job exists yet.
func handleTemplateInitialState(currentStatus aimv1alpha1.AIMTemplateStatusEnum) *templateStatusResult {
	// Check if template is already Available (job lookup was skipped to prevent re-running discovery)
	if currentStatus == aimv1alpha1.AIMTemplateStatusReady {
		// Template is already Available, return nil to indicate no status changes needed
		return nil
	}

	return &templateStatusResult{
		Status: aimv1alpha1.AIMTemplateStatusPending,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionTrue, controllerutils.ReasonReconciling, "Initiating discovery"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, controllerutils.ReasonReconciling, "Template is not ready"),
		},
	}
}

// ProjectTemplateStatus computes status from observation and errors.
// This is shared between cluster and namespace-scoped template controllers.
// Modifies templateStatus directly and emits events for discovery phase changes.
func ProjectTemplateStatus(
	ctx context.Context,
	k8sClient client.Client,
	clientset kubernetes.Interface,
	recorder record.EventRecorder,
	template TemplateWithStatus,
	obs *TemplateObservation,
	errs controllerutils.ReconcileErrors,
	imageNotFoundMessage string,
) error {
	templateStatus := template.GetStatus()
	currentStatus := templateStatus.Status

	// Set resolved runtime config and image
	templateStatus.ResolvedRuntimeConfig = nil
	templateStatus.ResolvedModel = nil
	if obs != nil && obs.RuntimeConfig != nil {
		templateStatus.ResolvedRuntimeConfig = obs.RuntimeConfig.ResolvedRef
	}

	// Try each handler in order, applying the first one that returns a result
	handlers := []func() *templateStatusResult{
		func() *templateStatusResult { return handleTemplateReconcileErrors(errs, imageNotFoundMessage) },
		func() *templateStatusResult {
			return handleTemplateWithInlineModelSources(template.GetSpecModelSources(), currentStatus, recorder, template)
		},
		func() *templateStatusResult {
			return handleTemplateGPUUnavailable(obs, currentStatus, recorder, template)
		},
		func() *templateStatusResult { return handleTemplateMissingImage(obs, imageNotFoundMessage) },
		func() *templateStatusResult { return handleTemplateImagePullFailed(obs, recorder, template) },
		func() *templateStatusResult {
			return handleTemplateDiscoveryRunning(obs, currentStatus, recorder, template)
		},
		func() *templateStatusResult {
			return handleTemplateDiscoveryFailed(obs, currentStatus, recorder, template)
		},
		func() *templateStatusResult {
			return handleTemplateDiscoverySucceeded(ctx, k8sClient, clientset, obs, currentStatus, recorder, template)
		},
		func() *templateStatusResult { return handleTemplateInitialState(currentStatus) },
	}

	for _, handler := range handlers {
		if result := handler(); result != nil {
			applyTemplateStatusResult(templateStatus, *result)
			return nil
		}
	}

	// No handler applied, no changes needed (template already in correct state)
	return nil
}
