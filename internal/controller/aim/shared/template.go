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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
	baseutils "github.com/silogen/kaiwo/pkg/utils"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// TemplateObservation holds the common observed state for both template types
type TemplateObservation struct {
	Job              *batchv1.Job
	Image            string
	ImageResources   *corev1.ResourceRequirements
	ImagePullSecrets []corev1.LocalObjectReference
	RuntimeConfig    *RuntimeConfigResolution
}

// TemplateSpec provides the common template specification
type TemplateSpec interface {
	GetModelName() string
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
	GetRuntime              func(ctx context.Context) (R, error)
	ShouldCheckDiscoveryJob bool
	GetDiscoveryJob         func(ctx context.Context) (*batchv1.Job, error)
	LookupImage             func(ctx context.Context) (*ImageLookupResult, error)
	ResolveRuntimeConfig    func(ctx context.Context) (*RuntimeConfigResolution, error)
	OnRuntimeConfigResolved func(resolution *RuntimeConfigResolution)
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
			obs.ImagePullSecrets = helpers.CopyPullSecrets(resolution.EffectiveSpec.ImagePullSecrets)
			if opts.OnRuntimeConfigResolved != nil {
				opts.OnRuntimeConfigResolved(resolution)
			}
		}
	}

	return obs, nil
}

// TemplatePlanContext provides metadata needed during plan generation.
type TemplatePlanContext struct {
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

// PlanTemplateResources produces desired objects based on the observation and controller-provided builders.
func PlanTemplateResources(ctx TemplatePlanContext, builders TemplatePlanBuilders) []client.Object {
	if ctx.Observation == nil || ctx.Observation.Image == "" {
		return nil
	}

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
	if ctx.Status == aimv1alpha1.AIMTemplateStatusAvailable && builders.BuildRuntime != nil {
		if runtime := builders.BuildRuntime(input); runtime != nil {
			desired = append(desired, runtime)
		}
	}

	// Create discovery job if template is not yet Available and job hasn't completed
	if ctx.Status != aimv1alpha1.AIMTemplateStatusAvailable &&
		builders.BuildDiscoveryJob != nil &&
		(ctx.Observation.Job == nil || !IsJobComplete(ctx.Observation.Job)) {
		if job := builders.BuildDiscoveryJob(input); job != nil {
			desired = append(desired, job)
		}
	}

	return desired
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
			controllerutils.NewCondition(controllerutils.ConditionTypeFailure, metav1.ConditionTrue, "ImageNotFound", imageNotFoundMessage),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionFalse, "ImageNotFound", "Cannot proceed without image"),
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
		Status: aimv1alpha1.AIMTemplateStatusAvailable,
		Conditions: []metav1.Condition{
			controllerutils.NewCondition(controllerutils.ConditionTypeDiscovered, metav1.ConditionTrue, controllerutils.ReasonDiscovered, "Model sources discovered"),
			controllerutils.NewCondition(controllerutils.ConditionTypeProgressing, metav1.ConditionFalse, controllerutils.ReasonAvailable, "Discovery complete"),
			controllerutils.NewCondition(controllerutils.ConditionTypeReady, metav1.ConditionTrue, controllerutils.ReasonAvailable, "Template is ready"),
		},
		ModelSources: discovery.ModelSources,
		Profile:      discovery.Profile,
	}
}

// handleTemplateInitialState handles the case where no discovery job exists yet.
func handleTemplateInitialState(currentStatus aimv1alpha1.AIMTemplateStatusEnum) *templateStatusResult {
	// Check if template is already Available (job lookup was skipped to prevent re-running discovery)
	if currentStatus == aimv1alpha1.AIMTemplateStatusAvailable {
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
	templateStatus.ResolvedImage = nil
	if obs != nil && obs.RuntimeConfig != nil {
		templateStatus.ResolvedRuntimeConfig = obs.RuntimeConfig.ResolvedRef
	}

	// Try each handler in order, applying the first one that returns a result
	handlers := []func() *templateStatusResult{
		func() *templateStatusResult { return handleTemplateReconcileErrors(errs, imageNotFoundMessage) },
		func() *templateStatusResult { return handleTemplateMissingImage(obs, imageNotFoundMessage) },
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
