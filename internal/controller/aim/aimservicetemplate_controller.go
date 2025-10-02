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

package aim

import (
	"context"
	"fmt"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/framework"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
)

const (
	namespaceTemplateFinalizerName = "aim.silogen.ai/namespace-template-finalizer"
	namespaceTemplateFieldOwner    = "aim-namespace-template-controller"
)

// AIMServiceTemplateReconciler reconciles a AIMServiceTemplate object
type AIMServiceTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *AIMServiceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the template
	var template aimv1alpha1.AIMServiceTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMServiceTemplate", "name", template.Name, "namespace", template.Namespace)

	// Use framework orchestrator with closures
	return framework.Reconcile(ctx, framework.ReconcileSpec{
		Client:        r.Client,
		Scheme:        r.Scheme,
		Object:        &template,
		FinalizerName: namespaceTemplateFinalizerName,
		FieldOwner:    namespaceTemplateFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &template)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			return r.plan(ctx, &template, obs.(*namespaceTemplateObservation))
		},

		ProjectFn: func(ctx context.Context, obs any, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
			return r.projectStatus(ctx, &template, obs.(*namespaceTemplateObservation), errs)
		},

		FinalizeFn: nil, // No external cleanup needed
	})
}

// namespaceTemplateObservation holds observed state
type namespaceTemplateObservation struct {
	Runtime *servingv1alpha1.ServingRuntime
	Job     *batchv1.Job
	Image   string // Container image from AIMImage or AIMClusterImage
}

// observe gathers current cluster state (read-only)
func (r *AIMServiceTemplateReconciler) observe(ctx context.Context, template *aimv1alpha1.AIMServiceTemplate) (*namespaceTemplateObservation, error) {
	obs := &namespaceTemplateObservation{}

	// Check if ServingRuntime exists
	runtime, err := shared.GetServingRuntime(ctx, r.Client, template.Namespace, template.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ServingRuntime: %w", err)
	}
	if err == nil {
		obs.Runtime = runtime
	}

	// Check if discovery job exists (in same namespace)
	job, err := shared.GetDiscoveryJob(ctx, r.Client, template.Namespace, template.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovery job: %w", err)
	}
	obs.Job = job

	// Lookup image from AIMImage (namespace-scoped) first, then AIMClusterImage (cluster-scoped)
	image, err := shared.LookupImageForNamespaceTemplate(ctx, r.Client, template.Namespace, template.Spec.ModelID)
	if err != nil {
		return nil, err
	}
	obs.Image = image

	return obs, nil
}

// plan computes desired state (pure function)
func (r *AIMServiceTemplateReconciler) plan(_ context.Context, template *aimv1alpha1.AIMServiceTemplate, obs *namespaceTemplateObservation) ([]client.Object, error) {
	var desired []client.Object

	// If no image found, return empty desired state (will be handled in status projection)
	if obs.Image == "" {
		return desired, nil
	}

	// Owner reference for all created objects
	ownerRef := metav1.OwnerReference{
		APIVersion:         template.APIVersion,
		Kind:               template.Kind,
		Name:               template.Name,
		UID:                template.UID,
		Controller:         ptr(true),
		BlockOwnerDeletion: ptr(true),
	}

	// Always include ServingRuntime in desired state
	runtime := shared.BuildServingRuntime(shared.ServingRuntimeSpec{
		Name:      template.Name,
		Namespace: template.Namespace,
		ModelID:   template.Spec.ModelID,
		Image:     obs.Image,
		OwnerRef:  ownerRef,
	})
	desired = append(desired, runtime)

	// Include discovery job if not yet complete
	if obs.Job == nil || !shared.IsJobComplete(obs.Job) {
		job := shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
			TemplateName: template.Name,
			Namespace:    template.Namespace,
			ModelID:      template.Spec.ModelID,
			Image:        obs.Image,
			Env:          template.Spec.Env,
			OwnerRef:     ownerRef,
		})
		desired = append(desired, job)
	}

	// TODO: If caching.enabled, create AIMTemplateCache object
	// This will be handled in a future iteration

	return desired, nil
}

// projectStatus computes status from observation + errors (read-only)
func (r *AIMServiceTemplateReconciler) projectStatus(
	ctx context.Context,
	template *aimv1alpha1.AIMServiceTemplate,
	obs *namespaceTemplateObservation,
	errs framework.ReconcileErrors,
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
			fmt.Sprintf("No AIMImage or AIMClusterImage found for modelId %q", template.Spec.ModelID),
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

	// Compute conditions based on observation

	// Progressing condition: True while job is running
	if obs.Job != nil && !shared.IsJobComplete(obs.Job) {
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
	if obs.Job != nil && shared.IsJobFailed(obs.Job) {
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
	if obs.Job != nil && shared.IsJobSucceeded(obs.Job) {
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
		sources, err := shared.ParseDiscoveryLogs(ctx, r.Client, obs.Job)
		if err == nil {
			modelSources = sources
		}

		// TODO: Add CacheWarm condition when caching is enabled
		// if template.Spec.Caching != nil && template.Spec.Caching.Enabled {
		//     conditions = append(conditions, framework.NewCondition(
		//         framework.ConditionTypeCacheWarm,
		//         metav1.ConditionFalse,
		//         "CachePending",
		//         "Cache warming not yet implemented",
		//     ))
		// }
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

	if len(modelSources) > 0 {
		update.AdditionalFields = map[string]any{
			"ModelSources": modelSources,
		}
	}

	return update, nil
}

func (r *AIMServiceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Named("aim-namespace-template").
		Complete(r)
}
