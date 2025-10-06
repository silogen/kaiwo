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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/framework"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
)

const (
	// clusterTemplateFinalizerName = "aim.silogen.ai/cluster-template-finalizer"
	clusterTemplateFieldOwner = "aim-cluster-template-controller"
)

// AIMClusterServiceTemplateReconciler reconciles a AIMClusterServiceTemplate object
type AIMClusterServiceTemplateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIMClusterServiceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the template
	var template aimv1alpha1.AIMClusterServiceTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMClusterServiceTemplate", "name", template.Name)

	// Use framework orchestrator with closures
	return framework.Reconcile(ctx, framework.ReconcileSpec[*aimv1alpha1.AIMClusterServiceTemplate, aimv1alpha1.AIMServiceTemplateStatus]{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Object:   &template,
		Recorder: r.Recorder,
		// FinalizerName: clusterTemplateFinalizerName,
		FieldOwner: clusterTemplateFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &template)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			return r.plan(ctx, &template, obs.(*clusterTemplateObservation))
		},

		ProjectFn: func(ctx context.Context, obs any, errs framework.ReconcileErrors) error {
			return r.projectStatus(ctx, &template, obs.(*clusterTemplateObservation), errs)
		},

		FinalizeFn: nil, // No external cleanup needed
	})
}

// clusterTemplateObservation holds observed state
type clusterTemplateObservation struct {
	Runtime *servingv1alpha1.ClusterServingRuntime
	shared.TemplateObservation
}

// observe gathers current cluster state (read-only)
func (r *AIMClusterServiceTemplateReconciler) observe(ctx context.Context, template *aimv1alpha1.AIMClusterServiceTemplate) (*clusterTemplateObservation, error) {
	logger := log.FromContext(ctx)
	obs := &clusterTemplateObservation{}

	// Check if ClusterServingRuntime exists
	runtime, err := shared.GetClusterServingRuntime(ctx, r.Client, template.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ClusterServingRuntime: %w", err)
	}
	if err == nil {
		obs.Runtime = runtime
	}

	// Only check for discovery job if template is not already Available
	// This prevents unnecessary job lookups and creation attempts after TTL cleanup
	if template.Status.Status != aimv1alpha1.AIMTemplateStatusAvailable {
		job, err := shared.GetDiscoveryJob(ctx, r.Client, shared.OperatorNamespace, template.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get discovery job: %w", err)
		}
		obs.Job = job
	}

	// Lookup image from AIMClusterImage catalog
	image, err := shared.LookupImageForClusterTemplate(ctx, r.Client, template.Spec.ModelID)
	if err != nil {
		return nil, err
	}
	obs.Image = image

	// Fetch default AIMClusterConfig for image pull secrets
	config, err := shared.GetDefaultClusterConfig(ctx, r.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to get default AIMClusterConfig: %w", err)
	}

	if config != nil {
		logger.Info("Using default AIMClusterConfig", "imagePullSecrets", len(config.Spec.ImagePullSecrets))
		framework.EmitNormalEvent(r.Recorder, template, "ConfigFound", fmt.Sprintf("Using default AIMClusterConfig with %d image pull secrets", len(config.Spec.ImagePullSecrets)))
		obs.ImagePullSecrets = config.Spec.ImagePullSecrets
	} else {
		logger.Info("Default AIMClusterConfig not found, proceeding without image pull secrets")
		framework.EmitWarningEvent(r.Recorder, template, "ConfigNotFound", "Default AIMClusterConfig not found, discovery job may fail if images require authentication")
	}

	return obs, nil
}

// plan computes desired state (pure function)
func (r *AIMClusterServiceTemplateReconciler) plan(_ context.Context, template *aimv1alpha1.AIMClusterServiceTemplate, obs *clusterTemplateObservation) ([]client.Object, error) {
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

	// Always include ClusterServingRuntime in desired state
	runtime := shared.BuildClusterServingRuntime(shared.ClusterServingRuntimeSpec{
		Name:     template.Name,
		ModelID:  template.Spec.ModelID,
		Image:    obs.Image,
		Metric:   template.Spec.Metric,
		OwnerRef: ownerRef,
	})
	desired = append(desired, runtime)

	// Include discovery job only if:
	// 1. Template is not already Available (gate to prevent re-running after TTL expires)
	// 2. Job doesn't exist or is not yet complete
	if template.Status.Status != aimv1alpha1.AIMTemplateStatusAvailable {
		if obs.Job == nil || !shared.IsJobComplete(obs.Job) {
			job := shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
				TemplateName:     template.Name,
				Namespace:        shared.OperatorNamespace,
				ModelID:          template.Spec.ModelID,
				Image:            obs.Image,
				Env:              nil,
				ImagePullSecrets: obs.ImagePullSecrets,
				OwnerRef:         ownerRef,
			})
			desired = append(desired, job)
		}
	}

	return desired, nil
}

// projectStatus computes status from observation + errors (modifies template.Status directly)
func (r *AIMClusterServiceTemplateReconciler) projectStatus(
	ctx context.Context,
	template *aimv1alpha1.AIMClusterServiceTemplate,
	obs *clusterTemplateObservation,
	errs framework.ReconcileErrors,
) error {
	imageNotFoundMsg := fmt.Sprintf("No AIMClusterImage found for modelId %q", template.Spec.ModelID)
	var templateObs *shared.TemplateObservation
	if obs != nil {
		templateObs = &obs.TemplateObservation
	}
	return shared.ProjectTemplateStatus(ctx, r.Client, template, templateObs, errs, imageNotFoundMsg)
}

func (r *AIMClusterServiceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Named("aim-cluster-template").
		Complete(r)
}

func ptr[T any](v T) *T {
	return &v
}
