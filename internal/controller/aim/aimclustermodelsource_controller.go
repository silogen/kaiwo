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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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

// FilterResult captures the result of processing a single filter.
type FilterResult struct {
	Filter aimv1alpha1.ModelSourceFilter
	Images []aimclustermodelsource.RegistryImage
	Error  error // Error encountered while processing this filter
}

// ClusterModelSourceObservation contains interpreted state from fetched data.
type ClusterModelSourceObservation struct {
	filteredImages   []aimclustermodelsource.RegistryImage   // After all filters applied
	newImages        []aimclustermodelsource.RegistryImage   // Need model creation
	existingByURI    map[string]*aimv1alpha1.AIMClusterModel // Lookup map
	filterResults    []FilterResult                          // Per-filter results
	failedFilters    int                                     // Count of filters that failed
	succeededFilters int                                     // Count of filters that succeeded
	totalScanned     int
	totalFiltered    int
	runtimeConfig    *aimv1alpha1.AIMRuntimeConfigCommon // For label propagation
}

// ClusterModelSourceFetchResult contains data gathered during the Fetch phase.
type ClusterModelSourceFetchResult struct {
	existingModels []aimv1alpha1.AIMClusterModel
	filterResults  []FilterResult // Per-filter results with errors
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources/finalizers,verbs=update
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

	result, err := controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMClusterModelSource, aimv1alpha1.AIMClusterModelSourceStatus]{
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

			// Log any filter errors
			for _, result := range observation.filterResults {
				if result.Error != nil {
					logger.Error(result.Error, "Filter failed", "filter", result.Filter.Image)
				}
			}

			return nil
		},

		FinalizeFn: nil,
	})

	// If reconciliation succeeded, schedule periodic requeue based on syncInterval
	if err == nil && result.RequeueAfter == 0 {
		syncInterval := source.Spec.SyncInterval.Duration
		if syncInterval == 0 {
			syncInterval = aimv1alpha1.DefaultSyncInterval
		}
		result.RequeueAfter = syncInterval
		baseutils.Debug(logger, "Scheduled next sync", "interval", syncInterval)
	}

	return result, err
}

// observe gathers current cluster state (read-only)
func (r *ClusterModelSourceReconciler) observe(
	source *aimv1alpha1.AIMClusterModelSource,
	fetched ClusterModelSourceFetchResult,
) ClusterModelSourceObservation {
	obs := ClusterModelSourceObservation{
		existingByURI: make(map[string]*aimv1alpha1.AIMClusterModel),
		filterResults: fetched.filterResults,
	}

	// Build lookup map of existing models by image URI
	for i := range fetched.existingModels {
		model := &fetched.existingModels[i]
		obs.existingByURI[model.Spec.Image] = model
	}

	// Get the max models limit
	maxModels := source.GetMaxModels()
	existingCount := len(fetched.existingModels)

	// Process filter results
	for _, result := range fetched.filterResults {
		if result.Error != nil {
			obs.failedFilters++
		} else {
			obs.succeededFilters++
			obs.totalScanned += len(result.Images)

			// Collect all successful images
			for _, img := range result.Images {
				obs.filteredImages = append(obs.filteredImages, img)

				// Check if model already exists
				imageURI := img.ToImageURI()
				if _, exists := obs.existingByURI[imageURI]; !exists {
					// Only add to newImages if we haven't hit the limit
					if existingCount+len(obs.newImages) < maxModels {
						obs.newImages = append(obs.newImages, img)
					}
				}
			}
		}
	}
	obs.totalFiltered = len(obs.filteredImages)

	// Resolve runtime config for label propagation (cluster-scoped, so use empty namespace)
	// We silently ignore errors since label propagation is optional
	if resolution, err := shared.ResolveRuntimeConfig(context.Background(), r.Client, "", shared.DefaultRuntimeConfigName); err == nil && resolution != nil {
		obs.runtimeConfig = &resolution.EffectiveSpec.AIMRuntimeConfigCommon
	}

	return obs
}

// Fetch gathers all data needed for reconciliation.
// This includes existing AIMClusterModel resources and images from the registry per filter.
// Each filter is processed independently - failures in one filter don't affect others.
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

	// 2. Process each filter independently
	registryClient := aimclustermodelsource.NewRegistryClient(r.Clientset, shared.GetOperatorNamespace())

	for _, filter := range source.Spec.Filters {
		filterResult := r.fetchFilter(ctx, registryClient, source.Spec, filter)
		result.filterResults = append(result.filterResults, filterResult)
	}

	return result, nil
}

// fetchFilter processes a single filter and returns its result.
// This tries multiple strategies in order and captures errors appropriately.
func (r *ClusterModelSourceReconciler) fetchFilter(
	ctx context.Context,
	registryClient *aimclustermodelsource.RegistryClient,
	spec aimv1alpha1.AIMClusterModelSourceSpec,
	filter aimv1alpha1.ModelSourceFilter,
) FilterResult {
	result := FilterResult{Filter: filter}

	// Create a temporary spec with just this one filter for processing
	singleFilterSpec := spec
	singleFilterSpec.Filters = []aimv1alpha1.ModelSourceFilter{filter}

	// Strategy 1: Try static images (exact versions, no registry query needed)
	staticImages := aimclustermodelsource.ExtractStaticImages(singleFilterSpec)
	if len(staticImages) > 0 {
		result.Images = staticImages
		return result
	}

	// Strategy 2: Try tags list API (works for exact repos with version ranges)
	tagsListImages := aimclustermodelsource.FetchImagesUsingTagsList(ctx, registryClient, singleFilterSpec)
	if len(tagsListImages) > 0 {
		result.Images = tagsListImages
		return result
	}

	// Strategy 3: Try catalog API (works for wildcards on Docker Hub/Harbor/etc)
	catalogImages, err := registryClient.ListImages(ctx, singleFilterSpec)
	if err != nil {
		// Check if this filter has wildcards - if so, catalog API failure is a real error
		hasWildcard := aimclustermodelsource.FilterHasWildcard(filter)
		if hasWildcard {
			result.Error = fmt.Errorf("catalog API failed for wildcard filter %q: %w", filter.Image, err)
		} else {
			// No wildcards - catalog API is not required, this is a fallback failure
			// Treat as empty result, not an error
			result.Images = []aimclustermodelsource.RegistryImage{}
		}
		return result
	}

	result.Images = catalogImages
	return result
}

// Plan derives desired state changes based on observations.
// This is a pure function - returns only what should be created/deleted.
func (r *ClusterModelSourceReconciler) plan(
	source *aimv1alpha1.AIMClusterModelSource,
	obs ClusterModelSourceObservation,
) ([]client.Object, error) {
	var result []client.Object

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

		// Propagate labels from model source to cluster model based on runtime config
		shared.PropagateLabels(source, model, obs.runtimeConfig)

		result = append(result, model)
	}

	// Never delete - append-only lifecycle
	return result, nil
}

func (r *ClusterModelSourceReconciler) Project(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	obs ClusterModelSourceObservation,
) {
	totalFilters := obs.failedFilters + obs.succeededFilters

	// Determine overall status based on filter success/failure
	if obs.failedFilters == totalFilters && obs.failedFilters > 0 {
		// All filters failed
		status.Status = string(aimv1alpha1.AIMStatusFailed)
		r.setFailedConditions(status, obs)
	} else if obs.failedFilters > 0 {
		// Some filters failed, some succeeded
		status.Status = string(aimv1alpha1.AIMStatusDegraded)
		r.setDegradedConditions(status, obs)
	} else if obs.totalFiltered == 0 {
		// No filters, or no images matched
		status.Status = string(aimv1alpha1.AIMStatusPending)
		r.setPendingConditions(status, obs)
	} else {
		// All filters succeeded
		status.Status = string(aimv1alpha1.AIMStatusReady)
		r.setReadyConditions(status, obs)
	}

	// Update metrics
	status.DiscoveredModels = len(obs.existingByURI) + len(obs.newImages)
	status.AvailableModels = obs.totalFiltered
	status.ModelsLimitReached = status.AvailableModels > status.DiscoveredModels

	// Update LastSyncTime on every sync attempt (successful or not)
	now := metav1.NewTime(time.Now())
	status.LastSyncTime = &now

	// Set MaxModelsLimitReached condition (applies to all statuses)
	r.setMaxModelsCondition(status)
}

func (r *ClusterModelSourceReconciler) setReadyConditions(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	obs ClusterModelSourceObservation,
) {
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "AllFiltersSucceeded",
		Message: fmt.Sprintf("Successfully discovered %d model(s) from %d filter(s)", obs.totalFiltered, obs.succeededFilters),
	})
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Degraded",
		Status:  metav1.ConditionFalse,
		Reason:  "AllFiltersSucceeded",
		Message: "All filters processed successfully",
	})
}

func (r *ClusterModelSourceReconciler) setMaxModelsCondition(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
) {
	// Set MaxModelsLimitReached condition
	if status.ModelsLimitReached {
		shared.SetCondition(&status.Conditions, metav1.Condition{
			Type:    "MaxModelsLimitReached",
			Status:  metav1.ConditionTrue,
			Reason:  "LimitReached",
			Message: fmt.Sprintf("Model creation limit reached (%d models created). %d available images not created as models.", status.DiscoveredModels, status.AvailableModels-status.DiscoveredModels),
		})
	} else {
		shared.SetCondition(&status.Conditions, metav1.Condition{
			Type:    "MaxModelsLimitReached",
			Status:  metav1.ConditionFalse,
			Reason:  "LimitNotReached",
			Message: fmt.Sprintf("Created %d models, within limit", status.DiscoveredModels),
		})
	}
}

func (r *ClusterModelSourceReconciler) setDegradedConditions(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	obs ClusterModelSourceObservation,
) {
	// Collect error messages from failed filters
	var errorMsgs []string
	for _, result := range obs.filterResults {
		if result.Error != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("filter %q: %v", result.Filter.Image, result.Error))
		}
	}

	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "SomeFiltersFailed",
		Message: fmt.Sprintf("%d of %d filter(s) failed", obs.failedFilters, obs.failedFilters+obs.succeededFilters),
	})
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Degraded",
		Status:  metav1.ConditionTrue,
		Reason:  "SomeFiltersFailed",
		Message: fmt.Sprintf("%d filter(s) failed: %s", obs.failedFilters, strings.Join(errorMsgs, "; ")),
	})
}

func (r *ClusterModelSourceReconciler) setFailedConditions(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	obs ClusterModelSourceObservation,
) {
	// Collect error messages from failed filters
	var errorMsgs []string
	for _, result := range obs.filterResults {
		if result.Error != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("filter %q: %v", result.Filter.Image, result.Error))
		}
	}

	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "AllFiltersFailed",
		Message: fmt.Sprintf("All %d filter(s) failed", obs.failedFilters),
	})
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Degraded",
		Status:  metav1.ConditionTrue,
		Reason:  "AllFiltersFailed",
		Message: fmt.Sprintf("All filters failed: %s", strings.Join(errorMsgs, "; ")),
	})
}

func (r *ClusterModelSourceReconciler) setPendingConditions(
	status *aimv1alpha1.AIMClusterModelSourceStatus,
	_ ClusterModelSourceObservation,
) {
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "NoImagesDiscovered",
		Message: "No images matched the configured filters",
	})
	shared.SetCondition(&status.Conditions, metav1.Condition{
		Type:    "Degraded",
		Status:  metav1.ConditionFalse,
		Reason:  "NoImagesDiscovered",
		Message: "No filter failures",
	})
}

func (r *ClusterModelSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aim-cluster-model-source-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterModelSource{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&aimv1alpha1.AIMClusterModel{}).
		Named("aim-cluster-model-source").
		Complete(r)
}
