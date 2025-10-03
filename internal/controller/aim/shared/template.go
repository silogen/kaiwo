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
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/framework"
)

// TemplateObservation holds the common observed state for both template types
type TemplateObservation struct {
	Job              *batchv1.Job
	Image            string
	ImagePullSecrets []corev1.LocalObjectReference
}

// TemplateSpec provides the common template specification
type TemplateSpec interface {
	GetModelID() string
}

// ProjectTemplateStatus computes status from observation and errors.
// This is shared between cluster and namespace-scoped template controllers.
func ProjectTemplateStatus(
	ctx context.Context,
	k8sClient client.Client,
	templateSpec TemplateSpec,
	obs *TemplateObservation,
	errs framework.ReconcileErrors,
	imageNotFoundMessage string,
) (framework.StatusUpdate, error) {
	var conditions []metav1.Condition
	var status aimv1alpha1.AIMTemplateStatusEnum

	// Handle errors first
	if errs.HasError() {
		status = aimv1alpha1.AIMTemplateStatusFailed

		if errs.ObserveErr != nil {
			conditions = append(conditions, framework.NewCondition(
				framework.ConditionTypeFailure,
				metav1.ConditionTrue,
				framework.ReasonFailed,
				fmt.Sprintf("Observation failed: %v", errs.ObserveErr),
			))
		}

		if errs.ApplyErr != nil {
			conditions = append(conditions, framework.NewCondition(
				framework.ConditionTypeFailure,
				metav1.ConditionTrue,
				framework.ReasonFailed,
				fmt.Sprintf("Apply failed: %v", errs.ApplyErr),
			))
		}

		conditions = append(conditions, framework.NewCondition(
			framework.ConditionTypeReady,
			metav1.ConditionFalse,
			framework.ReasonFailed,
			"Template is not ready due to errors",
		))

		return framework.StatusUpdate{
			Conditions:  conditions,
			StatusField: "Status",
			StatusValue: status,
		}, nil
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

		return framework.StatusUpdate{
			Conditions:  conditions,
			StatusField: "Status",
			StatusValue: status,
		}, nil
	}

	// Progressing condition: True while job is running
	if obs.Job != nil && !IsJobComplete(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusProgressing

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

		return framework.StatusUpdate{
			Conditions:  conditions,
			StatusField: "Status",
			StatusValue: status,
		}, nil
	}

	// Discovery failed
	if obs.Job != nil && IsJobFailed(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusFailed

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

		return framework.StatusUpdate{
			Conditions:  conditions,
			StatusField: "Status",
			StatusValue: status,
		}, nil
	}

	// Discovery succeeded
	var modelSources []aimv1alpha1.AIMModelSource
	var profile any
	if obs.Job != nil && IsJobSucceeded(obs.Job) {
		status = aimv1alpha1.AIMTemplateStatusAvailable

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
		discovery, err := ParseDiscoveryLogs(ctx, k8sClient, obs.Job)
		if err == nil {
			modelSources = discovery.ModelSources
			profile = discovery.Profile
		}
	} else {
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

	update := framework.StatusUpdate{
		Conditions:  conditions,
		StatusField: "Status",
		StatusValue: status,
	}

	if len(modelSources) > 0 || profile != nil {
		update.AdditionalFields = map[string]any{}
		if len(modelSources) > 0 {
			update.AdditionalFields["ModelSources"] = modelSources
		}
		if profile != nil {
			update.AdditionalFields["Profile"] = profile
		}
	}

	return update, nil
}
