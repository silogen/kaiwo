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

	framework "github.com/silogen/kaiwo/internal/controller/framework"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// TemplateObservation holds the common observed state for both template types
type TemplateObservation struct {
	Job              *batchv1.Job
	Image            string
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
	var conditions []metav1.Condition
	var status aimv1alpha1.AIMTemplateStatusEnum
	templateStatus.EffectiveRuntimeConfig = nil

	if obs != nil && obs.RuntimeConfig != nil {
		templateStatus.EffectiveRuntimeConfig = obs.RuntimeConfig.EffectiveStatus
	}

	// Handle errors first
	if errs.HasError() {
		status = aimv1alpha1.AIMTemplateStatusFailed

		if errs.ObserveErr != nil {
			// Check if the error is specifically ErrImageNotFound
			if errors.Is(errs.ObserveErr, ErrImageNotFound) {
				conditions = append(conditions, framework.NewCondition(
					framework.ConditionTypeFailure,
					metav1.ConditionTrue,
					"ImageNotFound",
					imageNotFoundMessage,
				))
				conditions = append(conditions, framework.NewCondition(
					framework.ConditionTypeReady,
					metav1.ConditionFalse,
					"ImageNotFound",
					"Cannot proceed without image",
				))
			} else {
				conditions = append(conditions, framework.NewCondition(
					framework.ConditionTypeFailure,
					metav1.ConditionTrue,
					framework.ReasonFailed,
					fmt.Sprintf("Observation failed: %v", errs.ObserveErr),
				))
				conditions = append(conditions, framework.NewCondition(
					framework.ConditionTypeReady,
					metav1.ConditionFalse,
					framework.ReasonFailed,
					"Template is not ready due to errors",
				))
			}
		}

		if errs.ApplyErr != nil {
			conditions = append(conditions, framework.NewCondition(
				framework.ConditionTypeFailure,
				metav1.ConditionTrue,
				framework.ReasonFailed,
				fmt.Sprintf("Apply failed: %v", errs.ApplyErr),
			))
			conditions = append(conditions, framework.NewCondition(
				framework.ConditionTypeReady,
				metav1.ConditionFalse,
				framework.ReasonFailed,
				"Template is not ready due to errors",
			))
		}

		// Set status and conditions
		templateStatus.Status = status
		for _, cond := range conditions {
			meta.SetStatusCondition(&templateStatus.Conditions, cond)
		}
		return nil
	}

	// Check if image is missing
	if obs.Image == "" {
		status = aimv1alpha1.AIMTemplateStatusFailed

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeFailure,
			metav1.ConditionTrue,
			"ImageNotFound",
			imageNotFoundMessage,
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionFalse,
			"ImageNotFound",
			"Cannot proceed without image",
		))

		// Set status and conditions
		templateStatus.Status = status
		for _, cond := range conditions {
			meta.SetStatusCondition(&templateStatus.Conditions, cond)
		}
		return nil
	}

	// Progressing condition: True while job is running
	if obs.Job != nil && !IsJobComplete(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusProgressing

		// Emit event if transitioning from Pending to Progressing
		if currentStatus == aimv1alpha1.AIMTemplateStatusPending {
			framework.EmitNormalEvent(recorder, template, "DiscoveryStarted", "Discovery job is running")
		}

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeProgressing,
			metav1.ConditionTrue,
			framework.ReasonDiscoveryRunning,
			"Discovery job is running",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeDiscovered,
			metav1.ConditionFalse,
			framework.ReasonJobPending,
			"Discovery job has not completed",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionFalse,
			framework.ReasonReconciling,
			"Template is not ready yet",
		))

		// Set status and conditions
		templateStatus.Status = status
		for _, cond := range conditions {
			meta.SetStatusCondition(&templateStatus.Conditions, cond)
		}
		return nil
	}

	// Discovery failed
	if obs.Job != nil && IsJobFailed(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusFailed

		// Emit event if transitioning from Progressing to Failed
		if currentStatus == aimv1alpha1.AIMTemplateStatusProgressing {
			framework.EmitWarningEvent(recorder, template, "DiscoveryFailed", "Discovery job failed")
		}

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeFailure,
			metav1.ConditionTrue,
			framework.ReasonJobFailed,
			"Discovery job failed",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeDiscovered,
			metav1.ConditionFalse,
			framework.ReasonDiscoveryFailed,
			"Discovery failed",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionFalse,
			framework.ReasonFailed,
			"Template is not ready",
		))

		// Set status and conditions
		templateStatus.Status = status
		for _, cond := range conditions {
			meta.SetStatusCondition(&templateStatus.Conditions, cond)
		}
		return nil
	}

	// Discovery succeeded
	var modelSources []aimv1alpha1.AIMModelSource
	var profile *apiextensionsv1.JSON
	if obs.Job != nil && IsJobSucceeded(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusAvailable

		// Emit event if transitioning from Progressing to Available
		if currentStatus == aimv1alpha1.AIMTemplateStatusProgressing {
			framework.EmitNormalEvent(recorder, template, "DiscoverySucceeded", "Model sources discovered successfully")
		}

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeDiscovered,
			metav1.ConditionTrue,
			framework.ReasonDiscovered,
			"Model sources discovered",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeProgressing,
			metav1.ConditionFalse,
			framework.ReasonAvailable,
			"Discovery complete",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionTrue,
			framework.ReasonAvailable,
			"Template is ready",
		))

		// Parse discovery results
		discovery, err := ParseDiscoveryLogs(ctx, k8sClient, clientset, obs.Job)
		if err != nil {
			// Discovery job succeeded but parsing failed
			status = aimv1alpha1.AIMTemplateStatusFailed

			framework.EmitWarningEvent(recorder, template, "DiscoveryParseFailed",
				fmt.Sprintf("Failed to parse discovery output: %v", err))

			conditions = []metav1.Condition{
				framework.NewCondition(
					framework.ConditionTypeFailure,
					metav1.ConditionTrue,
					"DiscoveryParseFailed",
					fmt.Sprintf("Failed to parse discovery output: %v", err),
				),
				framework.NewCondition(
					framework.ConditionTypeReady,
					metav1.ConditionFalse,
					"DiscoveryParseFailed",
					"Template is not ready",
				),
			}
		} else {
			modelSources = discovery.ModelSources
			profile = discovery.Profile
		}
	} else {
		// Check if template is already Available (job lookup was skipped to prevent re-running discovery)
		if currentStatus == aimv1alpha1.AIMTemplateStatusAvailable {
			// Template is already Available, return without status changes
			// This prevents resetting to Pending when job lookup is skipped for Available templates
			return nil
		}

		// No job yet (initial state)
		status = aimv1alpha1.AIMTemplateStatusPending

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeProgressing,
			metav1.ConditionTrue,
			framework.ReasonReconciling,
			"Initiating discovery",
		))

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionFalse,
			framework.ReasonReconciling,
			"Template is not ready",
		))
	}

	// Update status fields directly
	templateStatus.Status = status
	for _, cond := range conditions {
		meta.SetStatusCondition(&templateStatus.Conditions, cond)
	}

	// Set additional fields
	if len(modelSources) > 0 {
		templateStatus.ModelSources = modelSources
	}
	if profile != nil {
		templateStatus.Profile = profile
	}

	return nil
}
