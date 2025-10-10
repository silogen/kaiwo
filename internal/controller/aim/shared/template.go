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

	"github.com/silogen/kaiwo/internal/controller/framework"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// TemplateObservation holds the common observed state for both template types
type TemplateObservation struct {
	Job              *batchv1.Job
	Image            string
	ImagePullSecrets []corev1.LocalObjectReference
	RuntimeConfig    *RuntimeConfigResolution
	RuntimeFound     bool
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
	LookupImage             func(ctx context.Context) (string, error)
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
			obs.RuntimeFound = true
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
			return nil, err
		}
		obs.Image = image
	}

	if opts.ResolveRuntimeConfig != nil {
		resolution, err := opts.ResolveRuntimeConfig(ctx)
		if err != nil {
			return nil, err
		}
		if resolution != nil {
			obs.RuntimeConfig = resolution
			obs.ImagePullSecrets = CopyPullSecrets(resolution.EffectiveSpec.ImagePullSecrets)
			if opts.OnRuntimeConfigResolved != nil {
				opts.OnRuntimeConfigResolved(resolution)
			}
		}
	}

	return obs, nil
}

// TemplatePlanContext provides metadata needed during plan generation.
type TemplatePlanContext struct {
	Template    TemplateWithStatus
	APIVersion  string
	Kind        string
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

	if ctx.Template == nil {
		return nil
	}

	templateStatus := ctx.Template.GetStatus()

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

	if builders.BuildRuntime != nil {
		includeRuntime := false
		if ctx.Observation.RuntimeFound {
			includeRuntime = true
		} else if templateStatus != nil {
			discovered := meta.IsStatusConditionTrue(templateStatus.Conditions, framework.ConditionTypeDiscovered)
			profileReady := isProfilePopulated(templateStatus.Profile)
			if discovered && profileReady {
				includeRuntime = true
			}
		}

		if includeRuntime {
			if runtime := builders.BuildRuntime(input); runtime != nil {
				desired = append(desired, runtime)
			}
		}
	}

	currentStatus := aimv1alpha1.AIMTemplateStatusPending
	if templateStatus != nil && templateStatus.Status != "" {
		currentStatus = templateStatus.Status
	}

	if currentStatus != aimv1alpha1.AIMTemplateStatusAvailable &&
		builders.BuildDiscoveryJob != nil &&
		(ctx.Observation.Job == nil || !IsJobComplete(ctx.Observation.Job)) {
		if job := builders.BuildDiscoveryJob(input); job != nil {
			desired = append(desired, job)
		}
	}

	return desired
}

func isProfilePopulated(profile aimv1alpha1.AIMProfile) bool {
	if profile.EngineArgs != nil {
		return true
	}

	if len(profile.EnvVars) > 0 {
		return true
	}

	metadata := profile.Metadata
	if metadata.Engine != "" || metadata.GPU != "" || metadata.GPUCount != 0 {
		return true
	}

	if string(metadata.Metric) != "" || string(metadata.Precision) != "" {
		return true
	}

	return false
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

// templateStatusResult encapsulates the status update result
type templateStatusResult struct {
	status       aimv1alpha1.AIMTemplateStatusEnum
	conditions   []metav1.Condition
	modelSources []aimv1alpha1.AIMModelSource
	profile      *aimv1alpha1.AIMProfile
	shouldReturn bool // true if we should return early
}

// handleTemplateErrors processes reconciliation errors and returns appropriate status
func handleTemplateErrors(errs framework.ReconcileErrors, imageNotFoundMessage string) *templateStatusResult {
	if !errs.HasError() {
		return nil
	}

	result := &templateStatusResult{
		status:       aimv1alpha1.AIMTemplateStatusFailed,
		shouldReturn: true,
	}

	if errs.ObserveErr != nil {
		if errors.Is(errs.ObserveErr, ErrImageNotFound) {
			result.conditions = append(result.conditions,
				framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, "ImageNotFound", imageNotFoundMessage),
				framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, "ImageNotFound", "Cannot proceed without image"),
			)
		} else {
			result.conditions = append(result.conditions,
				framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, framework.ReasonFailed,
					fmt.Sprintf("Observation failed: %v", errs.ObserveErr)),
				framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonFailed,
					"Template is not ready due to errors"),
			)
		}
	}

	if errs.ApplyErr != nil {
		result.conditions = append(result.conditions,
			framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, framework.ReasonFailed,
				fmt.Sprintf("Apply failed: %v", errs.ApplyErr)),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonFailed,
				"Template is not ready due to errors"),
		)
	}

	return result
}

// handleMissingImage processes missing image scenario
func handleMissingImage(imageNotFoundMessage string) *templateStatusResult {
	return &templateStatusResult{
		status: aimv1alpha1.AIMTemplateStatusFailed,
		conditions: []metav1.Condition{
			framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, "ImageNotFound", imageNotFoundMessage),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, "ImageNotFound", "Cannot proceed without image"),
		},
		shouldReturn: true,
	}
}

// handleRunningJob processes running discovery job state
func handleRunningJob(currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if currentStatus == aimv1alpha1.AIMTemplateStatusPending {
		framework.EmitNormalEvent(recorder, template, "DiscoveryStarted", "Discovery job is running")
	}

	return &templateStatusResult{
		status: aimv1alpha1.AIMTemplateStatusProgressing,
		conditions: []metav1.Condition{
			framework.NewCondition(framework.ConditionTypeProgressing, metav1.ConditionTrue, framework.ReasonDiscoveryRunning, "Discovery job is running"),
			framework.NewCondition(framework.ConditionTypeDiscovered, metav1.ConditionFalse, framework.ReasonJobPending, "Discovery job has not completed"),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonReconciling, "Template is not ready yet"),
		},
		shouldReturn: true,
	}
}

// handleFailedJob processes failed discovery job state
func handleFailedJob(currentStatus aimv1alpha1.AIMTemplateStatusEnum, recorder record.EventRecorder, template TemplateWithStatus) *templateStatusResult {
	if currentStatus == aimv1alpha1.AIMTemplateStatusProgressing {
		framework.EmitWarningEvent(recorder, template, "DiscoveryFailed", "Discovery job failed")
	}

	return &templateStatusResult{
		status: aimv1alpha1.AIMTemplateStatusFailed,
		conditions: []metav1.Condition{
			framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, framework.ReasonJobFailed, "Discovery job failed"),
			framework.NewCondition(framework.ConditionTypeDiscovered, metav1.ConditionFalse, framework.ReasonDiscoveryFailed, "Discovery failed"),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonFailed, "Template is not ready"),
		},
		shouldReturn: true,
	}
}

// handleDiscoverySuccess processes successful discovery and runtime creation
func handleDiscoverySuccess(
	ctx context.Context,
	k8sClient client.Client,
	clientset kubernetes.Interface,
	recorder record.EventRecorder,
	template TemplateWithStatus,
	job *batchv1.Job,
	runtimeObserved bool,
	currentStatus aimv1alpha1.AIMTemplateStatusEnum,
	templateStatus *aimv1alpha1.AIMServiceTemplateStatus,
) *templateStatusResult {
	jobSucceeded := job != nil && IsJobSucceeded(job)

	if jobSucceeded && currentStatus == aimv1alpha1.AIMTemplateStatusProgressing &&
		!meta.IsStatusConditionTrue(templateStatus.Conditions, framework.ConditionTypeDiscovered) {
		framework.EmitNormalEvent(recorder, template, "DiscoverySucceeded", "Model sources discovered successfully")
	}

	result := &templateStatusResult{
		status: aimv1alpha1.AIMTemplateStatusProgressing,
		conditions: []metav1.Condition{
			framework.NewCondition(framework.ConditionTypeDiscovered, metav1.ConditionTrue, framework.ReasonDiscovered, "Model sources discovered"),
		},
	}

	if runtimeObserved {
		if currentStatus != aimv1alpha1.AIMTemplateStatusAvailable {
			framework.EmitNormalEvent(recorder, template, "RuntimeReady", "Serving runtime created successfully")
		}
		result.status = aimv1alpha1.AIMTemplateStatusAvailable
		result.conditions = append(result.conditions,
			framework.NewCondition(framework.ConditionTypeProgressing, metav1.ConditionFalse, framework.ReasonAvailable, "Discovery complete"),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionTrue, framework.ReasonAvailable, "Template is ready"),
		)
	} else {
		result.conditions = append(result.conditions,
			framework.NewCondition(framework.ConditionTypeProgressing, metav1.ConditionTrue, framework.ReasonReconciling, "Waiting for ServingRuntime creation"),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonReconciling, "Waiting for ServingRuntime creation"),
		)
	}

	if jobSucceeded {
		discovery, err := ParseDiscoveryLogs(ctx, k8sClient, clientset, job)
		if err != nil {
			framework.EmitWarningEvent(recorder, template, "DiscoveryParseFailed",
				fmt.Sprintf("Failed to parse discovery output: %v", err))
			result.status = aimv1alpha1.AIMTemplateStatusFailed
			result.conditions = []metav1.Condition{
				framework.NewCondition(framework.ConditionTypeFailure, metav1.ConditionTrue, "DiscoveryParseFailed",
					fmt.Sprintf("Failed to parse discovery output: %v", err)),
				framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, "DiscoveryParseFailed", "Template is not ready"),
			}
		} else {
			result.modelSources = discovery.ModelSources
			result.profile = discovery.Profile
		}
	}

	return result
}

// handleInitialState processes initial pending state
func handleInitialState() *templateStatusResult {
	return &templateStatusResult{
		status: aimv1alpha1.AIMTemplateStatusPending,
		conditions: []metav1.Condition{
			framework.NewCondition(framework.ConditionTypeProgressing, metav1.ConditionTrue, framework.ReasonReconciling, "Initiating discovery"),
			framework.NewCondition(framework.ConditionTypeReady, metav1.ConditionFalse, framework.ReasonReconciling, "Template is not ready"),
		},
	}
}

// isDiscoveryKnown checks if discovery results are available
func isDiscoveryKnown(job *batchv1.Job, templateStatus *aimv1alpha1.AIMServiceTemplateStatus) bool {
	if job != nil && IsJobSucceeded(job) {
		return true
	}
	if templateStatus != nil {
		return meta.IsStatusConditionTrue(templateStatus.Conditions, framework.ConditionTypeDiscovered) ||
			isProfilePopulated(templateStatus.Profile) ||
			len(templateStatus.ModelSources) > 0
	}
	return false
}

// applyStatusResult applies the status result to the template status
func applyStatusResult(templateStatus *aimv1alpha1.AIMServiceTemplateStatus, result *templateStatusResult) {
	templateStatus.Status = result.status
	for _, cond := range result.conditions {
		meta.SetStatusCondition(&templateStatus.Conditions, cond)
	}
	if len(result.modelSources) > 0 {
		templateStatus.ModelSources = result.modelSources
	}
	if result.profile != nil {
		templateStatus.Profile = *result.profile
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
	errs framework.ReconcileErrors,
	imageNotFoundMessage string,
) error {
	templateStatus := template.GetStatus()
	currentStatus := templateStatus.Status
	templateStatus.EffectiveRuntimeConfig = nil

	if obs != nil && obs.RuntimeConfig != nil {
		templateStatus.EffectiveRuntimeConfig = obs.RuntimeConfig.EffectiveStatus
	}

	// Handle errors first
	if result := handleTemplateErrors(errs, imageNotFoundMessage); result != nil {
		applyStatusResult(templateStatus, result)
		return nil
	}

	// Check if image is missing
	if obs.Image == "" {
		applyStatusResult(templateStatus, handleMissingImage(imageNotFoundMessage))
		return nil
	}

	var job *batchv1.Job
	if obs != nil {
		job = obs.Job
	}
	runtimeObserved := obs != nil && obs.RuntimeFound

	// Handle running job
	if job != nil && !IsJobComplete(job) {
		applyStatusResult(templateStatus, handleRunningJob(currentStatus, recorder, template))
		return nil
	}

	// Handle failed job
	if job != nil && IsJobFailed(job) {
		applyStatusResult(templateStatus, handleFailedJob(currentStatus, recorder, template))
		return nil
	}

	// Handle discovery success or initial state
	if isDiscoveryKnown(job, templateStatus) {
		result := handleDiscoverySuccess(ctx, k8sClient, clientset, recorder, template, job, runtimeObserved, currentStatus, templateStatus)
		applyStatusResult(templateStatus, result)
		return nil
	}

	// Check if template is already Available (job lookup was skipped to prevent re-running discovery)
	if currentStatus == aimv1alpha1.AIMTemplateStatusAvailable {
		return nil
	}

	// Initial state - no job yet
	applyStatusResult(templateStatus, handleInitialState())
	return nil
}
