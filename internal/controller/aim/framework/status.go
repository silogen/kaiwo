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

package framework

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchStatus patches the status subresource with conditions and fields from StatusUpdate.
// Sets ObservedGeneration to match metadata.generation.
func PatchStatus(ctx context.Context, k8sClient client.Client, obj client.Object, update StatusUpdate) error {
	// Get status via reflection
	statusValue := reflect.ValueOf(obj).Elem().FieldByName("Status")
	if !statusValue.IsValid() {
		return fmt.Errorf("object %T has no Status field", obj)
	}

	// Set conditions (merge with existing)
	conditionsField := statusValue.FieldByName("Conditions")
	if conditionsField.IsValid() && conditionsField.CanSet() {
		existingConditions, ok := conditionsField.Interface().([]metav1.Condition)
		if !ok {
			return fmt.Errorf("conditions field is not []metav1.Condition")
		}

		for _, condition := range update.Conditions {
			meta.SetStatusCondition(&existingConditions, condition)
		}

		conditionsField.Set(reflect.ValueOf(existingConditions))
	}

	// Set ObservedGeneration
	observedGenField := statusValue.FieldByName("ObservedGeneration")
	if observedGenField.IsValid() && observedGenField.CanSet() {
		observedGenField.SetInt(obj.GetGeneration())
	}

	// Set status field (e.g., "Status" with value "Available")
	if update.StatusField != "" {
		statusEnumField := statusValue.FieldByName(update.StatusField)
		if statusEnumField.IsValid() && statusEnumField.CanSet() {
			statusEnumField.Set(reflect.ValueOf(update.StatusValue))
		}
	}

	// Set additional fields
	for fieldName, fieldValue := range update.AdditionalFields {
		field := statusValue.FieldByName(fieldName)
		if field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(fieldValue))
		}
	}

	// Patch status subresource
	if err := k8sClient.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// NewCondition creates a new condition with the given parameters
func NewCondition(
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: 0, // Will be set by meta.SetStatusCondition
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// Standard condition types for AIM resources
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeFailure     = "Failure"
	ConditionTypeDiscovered  = "Discovered"
	ConditionTypeCacheWarm   = "CacheWarm"
)

// Standard condition reasons
const (
	ReasonReconciling      = "Reconciling"
	ReasonAvailable        = "Available"
	ReasonFailed           = "Failed"
	ReasonDiscoveryRunning = "DiscoveryRunning"
	ReasonDiscoveryFailed  = "DiscoveryFailed"
	ReasonDiscovered       = "Discovered"
	ReasonJobPending       = "JobPending"
	ReasonJobFailed        = "JobFailed"
	ReasonJobSucceeded     = "JobSucceeded"
)
