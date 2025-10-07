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

	framework2 "github.com/silogen/kaiwo/internal/controller/framework"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
)

const (
	// clusterTemplateFinalizerName = "aim.silogen.ai/cluster-template-finalizer"
	clusterTemplateFieldOwner = "aim-cluster-template-controller"
)

// AIMClusterServiceTemplateReconciler reconciles a AIMClusterServiceTemplate object
type AIMClusterServiceTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
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
	return framework2.Reconcile(ctx, framework2.ReconcileSpec[*aimv1alpha1.AIMClusterServiceTemplate, aimv1alpha1.AIMServiceTemplateStatus]{
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
			var o *clusterTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*clusterTemplateObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &template, o)
		},

		ProjectFn: func(ctx context.Context, obs any, errs framework2.ReconcileErrors) error {
			var o *clusterTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*clusterTemplateObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &template, o, errs)
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
	clusterServingRuntime, err := shared.GetClusterServingRuntime(ctx, r.Client, template.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ClusterServingRuntime: %w", err)
	}
	if err == nil {
		obs.Runtime = clusterServingRuntime
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
	image, err := shared.LookupImageForClusterTemplate(ctx, r.Client, template.Spec.AIMImageName)
	if err != nil {
		return nil, err
	}
	obs.Image = image

	// Fetch AIMClusterConfig using configName from spec
	configName := template.Spec.ConfigName
	if configName == "" {
		configName = shared.DefaultConfigName
	}

	config, notFound, err := shared.GetClusterConfig(ctx, r.Client, configName)
	if err != nil {
		return nil, fmt.Errorf("failed to get AIMClusterConfig %q: %w", configName, err)
	}

	if notFound {
		// Distinguish between default (lenient) and explicit (strict)
		if configName != shared.DefaultConfigName {
			// User explicitly referenced a config that doesn't exist - FAIL
			return nil, fmt.Errorf("AIMClusterConfig %q not found", configName)
		}
		// Default config not found - WARN but continue
		logger.Info("Default AIMClusterConfig not found, proceeding without it")
		framework2.EmitWarningEvent(r.Recorder, template, "DefaultConfigNotFound",
			"Default AIMClusterConfig not found, proceeding with defaults. Discovery job may fail if images require authentication")
	} else {
		logger.Info("Using AIMClusterConfig", "configName", configName, "imagePullSecrets", len(config.Spec.ImagePullSecrets))
		framework2.EmitNormalEvent(r.Recorder, template, "ConfigFound",
			fmt.Sprintf("Using AIMClusterConfig %q with %d image pull secrets", configName, len(config.Spec.ImagePullSecrets)))
		obs.ImagePullSecrets = config.Spec.ImagePullSecrets
		obs.Config = config
	}

	return obs, nil
}

// plan computes desired state (pure function)
func (r *AIMClusterServiceTemplateReconciler) plan(_ context.Context, template *aimv1alpha1.AIMClusterServiceTemplate, obs *clusterTemplateObservation) ([]client.Object, error) {
	var desired []client.Object

	// If observation is nil or no image found, return empty desired state
	if obs == nil || obs.Image == "" {
		return desired, nil
	}

	// Owner reference for all created objects
	ownerRef := metav1.OwnerReference{
		APIVersion:         template.APIVersion,
		Kind:               template.Kind,
		Name:               template.Name,
		UID:                template.UID,
		Controller:         baseutils.Pointer(true),
		BlockOwnerDeletion: baseutils.Pointer(true),
	}

	// Always include ClusterServingRuntime in desired state
	clusterServingRuntime := shared.BuildClusterServingRuntime(shared.ClusterServingRuntimeSpec{
		Name:     template.Name,
		ModelID:  template.Spec.AIMImageName,
		Image:    obs.Image,
		Metric:   template.Spec.Metric,
		OwnerRef: ownerRef,
	})
	desired = append(desired, clusterServingRuntime)

	// Include discovery job only if:
	// 1. Template is not already Available (gate to prevent re-running after TTL expires)
	// 2. Job doesn't exist or is not yet complete
	if template.Status.Status != aimv1alpha1.AIMTemplateStatusAvailable {
		if obs.Job == nil || !shared.IsJobComplete(obs.Job) {
			job := shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
				TemplateName:     template.Name,
				Namespace:        shared.OperatorNamespace,
				ModelID:          template.Spec.AIMImageName,
				Image:            obs.Image,
				Env:              nil,
				TemplateSpec:     template.Spec.AIMServiceTemplateSpecCommon,
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
	errs framework2.ReconcileErrors,
) error {
	imageNotFoundMsg := fmt.Sprintf("No AIMClusterImage found for image name %q", template.Spec.AIMImageName)
	var templateObs *shared.TemplateObservation
	if obs != nil {
		templateObs = &obs.TemplateObservation
	}
	return shared.ProjectTemplateStatus(ctx, r.Client, r.Clientset, r.Recorder, template, templateObs, errs, imageNotFoundMsg)
}

func (r *AIMClusterServiceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Owns(&servingv1alpha1.ClusterServingRuntime{}).
		Named("aim-cluster-template").
		Complete(r)
}
