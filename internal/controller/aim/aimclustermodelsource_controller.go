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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/aimclustermodelsource"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// / ClusterModelSourceReconciler implements domain reconciliation for AIMClusterModelSource.
type ClusterModelSourceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// ClusterModelSourceObservation contains interpreted state from fetched data.
type ClusterModelSourceObservation struct {
	filteredImages    []aimclustermodelsource.RegistryImage   // After all filters applied
	newImages         []aimclustermodelsource.RegistryImage   // Need model creation
	existingByURI     map[string]*aimv1alpha1.AIMClusterModel // Lookup map
	registryReachable bool
	registryError     error
	totalScanned      int
	totalFiltered     int
}

// ClusterModelSourceFetchResult contains data gathered during the Fetch phase.
type ClusterModelSourceFetchResult struct {
	existingModels []aimv1alpha1.AIMClusterModel
	registryImages []aimclustermodelsource.RegistryImage
	registryError  error // Non-fatal, captured for status
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ClusterModelSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the cluster image source
	var source aimv1alpha1.AIMClusterModelSource
	if err := r.Get(ctx, req.NamespacedName, &source); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling AIMClusterModelSource", "name", source.Name)

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMClusterModelSource, aimv1alpha1.AIMClusterModelSourceStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &source,
		FieldOwner: "aim-cluster-model-source-controller",

		ObserveFn: func(ctx context.Context) (any, error) {
			fetched, err := r.Fetch(ctx, r.Client, &source)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch source data: %w", err)
			}
			return r.observe(&source, fetched), nil
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			observation, ok := obs.(ClusterModelSourceObservation)
			if !ok {
				return nil, baseutils.LogErrorf(logger, "unexpected observation type: %T", nil)
			}
			return r.plan(&source, observation)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			observation, ok := obs.(ClusterModelSourceObservation)
			if !ok {
				return baseutils.LogErrorf(logger, "unexpected observation type: %T", nil)
			}

			r.Project(&source.Status, observation)

			if observation.registryError != nil {
				logger.Error(observation.registryError, "Registry interaction failed")
			}

			return nil
		},

		FinalizeFn: nil,
	})
}

// observe gathers current cluster state (read-only)
func (r *ClusterModelSourceReconciler) observe(
	source *aimv1alpha1.AIMClusterModelSource,
	fetched ClusterModelSourceFetchResult,
) ClusterModelSourceObservation {
	obs := ClusterModelSourceObservation{
		existingByURI: make(map[string]*aimv1alpha1.AIMClusterModel),
	}

	// Build lookup map of existing models by image URI
	for i := range fetched.existingModels {
		model := &fetched.existingModels[i]
		obs.existingByURI[model.Spec.Image] = model
	}

	// Handle registry errors
	if fetched.registryError != nil {
		obs.registryReachable = false
		obs.registryError = fetched.registryError
		return obs
	}

	obs.registryReachable = true
	obs.totalScanned = len(fetched.registryImages)

	// Apply filters to registry images
	for _, img := range fetched.registryImages {
		if aimclustermodelsource.MatchesFilters(img, source.Spec.Filters, source.Spec.Versions) {
			obs.filteredImages = append(obs.filteredImages, img)

			// Check if model already exists
			imageURI := img.ToImageURI()
			if _, exists := obs.existingByURI[imageURI]; !exists {
				obs.newImages = append(obs.newImages, img)
			}
		}
	}
	obs.totalFiltered = len(obs.filteredImages)

	return obs
}

// Fetch gathers all data needed for reconciliation.
// This includes existing AIMClusterModel resources and available images from the registry.
func (r *ClusterModelSourceReconciler) Fetch(
	ctx context.Context,
	c client.Client,
	source *aimv1alpha1.AIMClusterModelSource,
) (ClusterModelSourceFetchResult, error) {
	result := ClusterModelSourceFetchResult{}

	// 1. List existing models owned by this source
	modelList := &aimv1alpha1.AIMClusterModelList{}
	if err := c.List(ctx, modelList, client.MatchingLabels{
		shared.LabelKeyModelSource: source.Name,
	}); err != nil {
		return result, fmt.Errorf("failed to list models: %w", err)
	}
	result.existingModels = modelList.Items

	// 2. Check if we can skip registry queries (all filters are exact image references)
	staticImages := aimclustermodelsource.ExtractStaticImages(source.Spec.Filters)
	if len(staticImages) > 0 && len(staticImages) == len(source.Spec.Filters) {
		// All filters are exact references - no registry query needed
		result.registryImages = staticImages
		return result, nil
	}

	// 3. Query registry for available images
	registryClient := aimclustermodelsource.NewRegistryClient(r.Clientset, shared.GetOperatorNamespace())
	images, err := registryClient.ListImages(ctx, source.Spec)
	if err != nil {
		// Capture error but don't fail - will be handled in Observe
		result.registryError = err
	} else {
		result.registryImages = images
	}

	return result, nil
}

// Plan derives desired state changes based on observations.
// This is a pure function - returns only what should be created/deleted.
func (r *ClusterModelSourceReconciler) plan(
	source *aimv1alpha1.AIMClusterModelSource,
	obs ClusterModelSourceObservation,
) ([]client.Object, error) {
	result := []client.Object{}

	// Only create models for new images (append-only lifecycle)
	for _, img := range obs.newImages {
		model := aimclustermodelsource.BuildClusterModel(source, img)

		// Set owner reference (non-blocking deletion)
		if err := controllerutil.SetOwnerReference(
			source, model, r.Scheme,
			controllerutil.WithBlockOwnerDeletion(false),
		); err != nil {
			return result, fmt.Errorf("failed to set owner reference: %w", err)
		}

		result = append(result, model)
	}

	// Never delete - append-only lifecycle
	return result, nil
}

func (r *ClusterModelSourceReconciler) Project(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	obs ClusterModelSourceObservation,
) {
	// Ready condition and overall status
	if !obs.registryReachable {
		if len(obs.existingByURI) > 0 {
			status.Status = string(aimv1alpha1.AIMStatusDegraded)
		} else {
			status.Status = string(aimv1alpha1.AIMStatusFailed)
		}
	} else if obs.totalFiltered == 0 {
		status.Status = string(aimv1alpha1.AIMStatusPending)
	} else {
		status.Status = string(aimv1alpha1.AIMStatusReady)
	}

	// Update metrics
	status.DiscoveredModels = len(obs.existingByURI) + len(obs.newImages)

	// Update LastSyncTime on every successful sync
	// Status updates don't trigger reconciliations due to GenerationChangedPredicate
	if obs.registryReachable {
		now := metav1.NewTime(time.Now())
		status.LastSyncTime = &now
	}

	// Update discovered images summary (limited to most recent 50)
	status.DiscoveredImages = aimclustermodelsource.BuildDiscoveredImagesSummary(obs.filteredImages, obs.existingByURI)
}

func (r *ClusterModelSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aim-cluster-image-source-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterModelSource{}).
		Owns(&aimv1alpha1.AIMClusterModel{}).
		Named("aim-cluster-image-source").
		Complete(r)
}
