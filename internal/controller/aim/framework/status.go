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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchStatus patches the status subresource if it changed from originalStatus.
// Sets ObservedGeneration to match metadata.generation.
// Uses retry logic to handle conflicts when the object is modified between read and update.
//
// This function uses generics to provide type-safe status access. The only reflection
// used is for setting ObservedGeneration, which is unavoidable without adding methods
// to every status type.
func PatchStatus[T ObjectWithStatus[S], S any](
	ctx context.Context,
	k8sClient client.Client,
	obj T,
	originalStatus S,
) error {
	// Retry on conflicts with exponential backoff
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the latest version of the object to avoid conflicts
		key := client.ObjectKeyFromObject(obj)
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			return err
		}

		status := obj.GetStatus()

		// Set ObservedGeneration using reflection (unavoidable without adding methods to every status type)
		statusValue := reflect.ValueOf(status).Elem()
		observedGenField := statusValue.FieldByName("ObservedGeneration")
		if observedGenField.IsValid() && observedGenField.CanSet() {
			observedGenField.SetInt(obj.GetGeneration())
		}

		// Compare with original status to detect changes
		if reflect.DeepEqual(originalStatus, *status) {
			return nil
		}

		// Patch status subresource
		if err := k8sClient.Status().Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		return nil
	})
}

// cloneStatus creates a deep copy of the status field using the typed accessor
func cloneStatus[T ObjectWithStatus[S], S any](obj T) S {
	status := obj.GetStatus()
	// Deep copy using reflection (necessary for complex status objects with slices/maps)
	clone := reflect.New(reflect.TypeOf(*status)).Elem()
	clone.Set(reflect.ValueOf(*status))
	return clone.Interface().(S)
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
