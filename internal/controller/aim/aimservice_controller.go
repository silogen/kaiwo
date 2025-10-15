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
	"strings"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	aimServiceFieldOwner       = "aim-service-controller"
	aimServiceTemplateIndexKey = ".spec.templateRef"
)

// AIMServiceReconciler reconciles AIMService resources into KServe InferenceServices.
type AIMServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/status,verbs=get
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var service aimv1alpha1.AIMService
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMService", "name", service.Name, "namespace", service.Namespace)

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMService, aimv1alpha1.AIMServiceStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &service,
		Recorder:   r.Recorder,
		FieldOwner: aimServiceFieldOwner,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &service)
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var observation *shared.ServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ServiceObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &service, observation)
		},
		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var observation *shared.ServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ServiceObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &service, observation, errs)
		},
	})
}

func (r *AIMServiceReconciler) observe(ctx context.Context, service *aimv1alpha1.AIMService) (*shared.ServiceObservation, error) {
	resolution, err := shared.ResolveTemplateNameForService(ctx, r.Client, service)
	if err != nil {
		return nil, err
	}

	obs := &shared.ServiceObservation{
		TemplateName:     resolution.FinalName,
		BaseTemplateName: resolution.BaseName,
		Scope:            shared.TemplateScopeNone,
	}

	// Observe template based on whether it's derived or not
	if resolution.Derived {
		obs.TemplateNamespace = service.Namespace
		if err := shared.ObserveDerivedTemplate(ctx, r.Client, service, resolution, obs); err != nil {
			return nil, err
		}
	} else if resolution.FinalName != "" {
		if err := shared.ObserveNonDerivedTemplate(ctx, r.Client, service, resolution.FinalName, obs); err != nil {
			return nil, err
		}
	}

	// Set template namespace if creating a new template
	if obs.ShouldCreateTemplate && obs.TemplateNamespace == "" {
		obs.TemplateNamespace = service.Namespace
	}

	// Mark for template creation if no template was found
	if !obs.TemplateFound() {
		obs.ShouldCreateTemplate = true
	}

	// Resolve route path if routing is enabled
	if service.Spec.Routing != nil && service.Spec.Routing.Enabled && obs.TemplateFound() {
		if routePath, err := shared.ResolveServiceRoutePath(service, obs.RuntimeConfigSpec); err != nil {
			obs.RouteTemplateErr = err
		} else {
			obs.RoutePath = routePath
		}
	}

	return obs, nil
}

func (r *AIMServiceReconciler) plan(_ context.Context, service *aimv1alpha1.AIMService, obs *shared.ServiceObservation) ([]client.Object, error) {
	var desired []client.Object

	if obs == nil {
		return desired, nil
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         service.APIVersion,
		Kind:               service.Kind,
		Name:               service.Name,
		UID:                service.UID,
		Controller:         baseutils.Pointer(true),
		BlockOwnerDeletion: baseutils.Pointer(true),
	}

	// Manage namespace-scoped template if we created it or need to create it.
	if obs.ShouldCreateTemplate || (obs.Scope == shared.TemplateScopeNamespace && obs.TemplateOwnedByService) {
		var baseSpec *aimv1alpha1.AIMServiceTemplateSpec
		if obs != nil && obs.TemplateSpec != nil {
			baseSpec = obs.TemplateSpec.DeepCopy()
		}
		template := shared.BuildDerivedTemplate(service, obs.TemplateName, baseSpec)
		desired = append(desired, template)
	}

	// Only create/update the InferenceService once the template is available.
	if obs.TemplateAvailable && obs.RuntimeConfigErr == nil {
		routePath := shared.DefaultRoutePath(service)
		if obs.RouteTemplateErr == nil && obs.RoutePath != "" {
			routePath = obs.RoutePath
		}
		templateState := aimstate.NewTemplateState(aimstate.TemplateState{
			Name:              obs.TemplateName,
			Namespace:         obs.TemplateNamespace,
			SpecCommon:        obs.TemplateSpecCommon,
			ImageResources:    obs.ImageResources,
			RuntimeConfigSpec: obs.RuntimeConfigSpec,
			Status:            obs.TemplateStatus,
		})

		// Only proceed if we have a model source (discovery must have succeeded and populated ModelSources)
		if templateState.ModelSource != nil {
			serviceState := aimstate.NewServiceState(service, templateState, aimstate.ServiceStateOptions{
				RuntimeName: obs.RuntimeName(),
				RoutePath:   routePath,
			})
			inferenceService := shared.BuildInferenceService(serviceState, ownerRef)
			desired = append(desired, inferenceService)

			if serviceState.Routing.Enabled && serviceState.Routing.GatewayRef != nil && obs.RouteTemplateErr == nil {
				route := shared.BuildInferenceServiceHTTPRoute(serviceState, ownerRef)
				desired = append(desired, route)
			}
		}
		// If ModelSource is nil, the status projection will handle showing that discovery
		// succeeded but produced no usable model sources.
	}

	return desired, nil
}

func (r *AIMServiceReconciler) projectStatus(
	ctx context.Context,
	service *aimv1alpha1.AIMService,
	obs *shared.ServiceObservation,
	errs controllerutils.ReconcileErrors,
) error {
	// Fetch InferenceService and HTTPRoute for status evaluation
	var inferenceService *servingv1beta1.InferenceService
	{
		var is servingv1beta1.InferenceService
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		}, &is); err == nil {
			inferenceService = &is
		}
	}

	var httpRoute *gatewayapiv1.HTTPRoute
	if service.Spec.Routing != nil && service.Spec.Routing.Enabled {
		routeName := shared.InferenceServiceRouteName(service.Name)
		var route gatewayapiv1.HTTPRoute
		if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: service.Namespace}, &route); err == nil {
			httpRoute = &route
		}
	}

	// Delegate status projection to shared function
	shared.ProjectServiceStatus(service, obs, inferenceService, httpRoute, errs)
	return nil
}

func (r *AIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMService{}, aimServiceTemplateIndexKey, func(obj client.Object) []string {
		service, ok := obj.(*aimv1alpha1.AIMService)
		if !ok {
			return nil
		}
		resolved := strings.TrimSpace(service.Spec.TemplateRef)
		if resolved == "" {
			if service.Status.ResolvedTemplate != nil {
				resolved = strings.TrimSpace(service.Status.ResolvedTemplate.Name)
			}
		}
		if resolved == "" {
			return nil
		}
		return []string{resolved}
	}); err != nil {
		return err
	}

	templateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		template, ok := obj.(*aimv1alpha1.AIMServiceTemplate)
		if !ok {
			return nil
		}

		var services aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &services,
			client.InNamespace(template.Namespace),
			client.MatchingFields{aimServiceTemplateIndexKey: template.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMServiceTemplate", "template", template.Name)
			return nil
		}

		return shared.RequestsForServices(services.Items)
	})

	clusterTemplateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterTemplate, ok := obj.(*aimv1alpha1.AIMClusterServiceTemplate)
		if !ok {
			return nil
		}

		var services aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &services,
			client.MatchingFields{aimServiceTemplateIndexKey: clusterTemplate.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMClusterServiceTemplate", "template", clusterTemplate.Name)
			return nil
		}

		return shared.RequestsForServices(services.Items)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMService{}).
		Owns(&servingv1beta1.InferenceService{}).
		Owns(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&gatewayapiv1.HTTPRoute{}).
		Watches(&aimv1alpha1.AIMServiceTemplate{}, templateHandler).
		Watches(&aimv1alpha1.AIMClusterServiceTemplate{}, clusterTemplateHandler).
		Named("aim-service").
		Complete(r)
}
