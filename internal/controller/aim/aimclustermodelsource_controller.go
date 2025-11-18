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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	aimClusterModelSourceFieldOwner = "aim-cluster-model-source-controller"
)

// AIMClusterModelSourceReconciler reconciles an AIMClusterModelSource object
type AIMClusterModelSourceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodelsources/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclustermodels,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AIMClusterModelSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the model source
	var source aimv1alpha1.AIMClusterModelSource
	if err := r.Get(ctx, req.NamespacedName, &source); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	baseutils.Debug(logger, "Reconciling AIMClusterModelSource", "name", source.Name)

	// Use framework orchestrator
	result, err := controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMClusterModelSource, aimv1alpha1.AIMClusterModelSourceStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &source,
		FieldOwner: aimClusterModelSourceFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &source)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var observation *SourceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*SourceObservation)
				if !ok {
					return nil, baseutils.LogErrorf(logger, "unexpected observation type", nil)
				}
			}
			return r.plan(ctx, &source, observation)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var observation *SourceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*SourceObservation)
				if !ok {
					return baseutils.LogErrorf(logger, "unexpected observation type", nil)
				}
			}
			return r.projectStatus(ctx, &source, observation, errs)
		},

		FinalizeFn: nil, // No external cleanup needed - models are owned by source
	})

	// Add periodic requeue based on sync interval
	if err == nil && result.RequeueAfter == 0 && source.Spec.SyncInterval.Duration > 0 {
		result.RequeueAfter = source.Spec.SyncInterval.Duration
		logger.V(1).Info("Scheduling next sync", "after", source.Spec.SyncInterval.Duration)
	}

	return result, err
}

// SourceObservation holds the observed state from the registry and cluster
type SourceObservation struct {
	// DiscoveredImages maps image URIs to whether they should be synced
	DiscoveredImages map[string]bool

	// ExistingModels are the AIMClusterModel resources currently owned by this source
	ExistingModels map[string]*aimv1alpha1.AIMClusterModel

	// RegistryError is any error encountered while accessing the registry
	RegistryError error
}

// observe gathers the current state from the registry and cluster (read-only)
func (r *AIMClusterModelSourceReconciler) observe(ctx context.Context, source *aimv1alpha1.AIMClusterModelSource) (*SourceObservation, error) {
	logger := log.FromContext(ctx)

	baseutils.Debug(logger, "Observing model source", "registry", source.Spec.Registry)

	obs := &SourceObservation{
		DiscoveredImages: make(map[string]bool),
		ExistingModels:   make(map[string]*aimv1alpha1.AIMClusterModel),
	}

	// Get operator namespace for secrets
	operatorNs := shared.GetOperatorNamespace()

	// List repositories from registry
	logger.V(1).Info("Listing repositories from registry", "registry", source.Spec.Registry)
	repos, err := shared.ListRepositories(
		ctx,
		source.Spec.Registry,
		r.Clientset,
		operatorNs,
		source.Spec.ImagePullSecrets,
	)
	if err != nil {
		obs.RegistryError = err
		logger.Error(err, "Failed to list repositories from registry")
		return obs, nil // Return observation with error, don't fail the reconcile
	}

	logger.V(1).Info("Listed repositories", "count", len(repos))

	// Process each filter to discover matching images
	for _, filter := range source.Spec.Filters {
		// Filter repositories by pattern
		matchingRepos := shared.FilterByPattern(repos, []string{filter.Image}, filter.Exclude)
		logger.V(1).Info("Filtered repositories", "pattern", filter.Image, "matches", len(matchingRepos))

		for _, repo := range matchingRepos {
			// Construct full repository name
			fullRepo := fmt.Sprintf("%s/%s", source.Spec.Registry, repo)

			// List tags for this repository
			tags, err := shared.ListTags(ctx, fullRepo, r.Clientset, operatorNs, source.Spec.ImagePullSecrets)
			if err != nil {
				logger.Error(err, "Failed to list tags for repository", "repository", fullRepo)
				continue // Skip this repository but continue with others
			}

			// Determine which version constraints to use
			var versionConstraints []string
			if filter.Versions != nil {
				// Filter has explicit version constraints (could be empty array)
				versionConstraints = filter.Versions
			} else {
				// Filter has no version constraints, use global
				versionConstraints = source.Spec.Versions
			}

			// Filter tags by semver
			matchingTags, err := shared.FilterTagsBySemver(tags, versionConstraints)
			if err != nil {
				logger.Error(err, "Failed to filter tags by semver", "repository", fullRepo, "constraints", versionConstraints)
				continue
			}

			logger.V(1).Info("Filtered tags", "repository", fullRepo, "matchingTags", len(matchingTags))

			// Add matching images to discovered set
			for _, tag := range matchingTags {
				imageURI := fmt.Sprintf("%s:%s", fullRepo, tag)
				obs.DiscoveredImages[imageURI] = true
			}
		}
	}

	// List existing AIMClusterModel resources owned by this source
	var models aimv1alpha1.AIMClusterModelList
	if err := r.List(ctx, &models,
		client.MatchingLabels{
			shared.LabelKeyModelSource: source.Name,
		},
	); err != nil {
		return nil, fmt.Errorf("failed to list existing models: %w", err)
	}

	for i := range models.Items {
		model := &models.Items[i]
		obs.ExistingModels[model.Name] = model
	}

	logger.V(1).Info("Observation complete",
		"discoveredImages", len(obs.DiscoveredImages),
		"existingModels", len(obs.ExistingModels))

	return obs, nil
}

// plan determines what AIMClusterModel resources should exist (create/update, never delete)
func (r *AIMClusterModelSourceReconciler) plan(ctx context.Context, source *aimv1alpha1.AIMClusterModelSource, obs *SourceObservation) ([]client.Object, error) {
	logger := log.FromContext(ctx)

	if obs == nil {
		return nil, nil
	}

	var desired []client.Object

	// Create/update AIMClusterModel for each discovered image
	for imageURI := range obs.DiscoveredImages {
		modelName := generateModelName(imageURI)

		// Check if a model with this image already exists (not necessarily owned by this source)
		existingModel := obs.ExistingModels[modelName]

		// Only manage models that:
		// 1. Don't exist yet, OR
		// 2. Are already owned by this source
		if existingModel != nil && existingModel.Labels[shared.LabelKeyModelSource] != source.Name {
			logger.V(1).Info("Skipping existing model not owned by this source",
				"modelName", modelName,
				"image", imageURI)
			continue
		}

		model := &aimv1alpha1.AIMClusterModel{
			ObjectMeta: metav1.ObjectMeta{
				Name: modelName,
				Labels: map[string]string{
					shared.LabelKeyModelSource:   source.Name,
					shared.LabelKeyAutoGenerated: shared.LabelValueAutoGenerated,
				},
			},
			Spec: aimv1alpha1.AIMModelSpec{
				Image:            imageURI,
				ImagePullSecrets: source.Spec.ImagePullSecrets,
				Discovery: &aimv1alpha1.AIMModelDiscoveryConfig{
					Enabled:             ptr.To(true),
					AutoCreateTemplates: ptr.To(true),
				},
			},
		}

		// Set owner reference so models are deleted when source is deleted
		if err := ctrl.SetControllerReference(source, model, r.Scheme); err != nil {
			return nil, fmt.Errorf("failed to set owner reference for model %s: %w", modelName, err)
		}

		desired = append(desired, model)
	}

	logger.V(1).Info("Planned models", "count", len(desired))
	return desired, nil
}

// projectStatus updates the AIMClusterModelSource status based on observations
func (r *AIMClusterModelSourceReconciler) projectStatus(ctx context.Context, source *aimv1alpha1.AIMClusterModelSource, obs *SourceObservation, errs controllerutils.ReconcileErrors) error {
	logger := log.FromContext(ctx)

	if obs == nil {
		return nil
	}

	// Update last sync time
	now := metav1.Now()
	source.Status.LastSyncTime = &now

	// Update synced model count
	source.Status.SyncedModelCount = int32(len(obs.DiscoveredImages))

	// Update conditions
	var conditions []metav1.Condition

	// RegistryReachable condition
	if obs.RegistryError != nil {
		conditions = append(conditions, metav1.Condition{
			Type:               "RegistryReachable",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: source.Generation,
			LastTransitionTime: now,
			Reason:             "RegistryError",
			Message:            fmt.Sprintf("Failed to access registry: %v", obs.RegistryError),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               "RegistryReachable",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: source.Generation,
			LastTransitionTime: now,
			Reason:             "RegistryAccessible",
			Message:            fmt.Sprintf("Successfully accessed registry and discovered %d images", len(obs.DiscoveredImages)),
		})
	}

	// Ready condition
	if errs.ApplyErr != nil {
		conditions = append(conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: source.Generation,
			LastTransitionTime: now,
			Reason:             "ApplyFailed",
			Message:            fmt.Sprintf("Failed to apply models: %v", errs.ApplyErr),
		})
	} else if obs.RegistryError != nil {
		conditions = append(conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: source.Generation,
			LastTransitionTime: now,
			Reason:             "RegistryError",
			Message:            "Unable to sync due to registry access error",
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: source.Generation,
			LastTransitionTime: now,
			Reason:             "SyncSuccessful",
			Message:            fmt.Sprintf("Successfully synced %d models", len(obs.DiscoveredImages)),
		})
	}

	source.Status.Conditions = conditions
	source.Status.ObservedGeneration = source.Generation

	logger.V(1).Info("Updated status",
		"syncedModelCount", source.Status.SyncedModelCount,
		"ready", conditions[len(conditions)-1].Status == metav1.ConditionTrue)

	return nil
}

// generateModelName creates a unique model name from an image URI
// Example: "ghcr.io/silogen/llama-3-8b:1.2.3" -> "silogen-llama-3-8b-1-2-3"
func generateModelName(imageURI string) string {
	// Remove registry prefix
	parts := strings.SplitN(imageURI, "/", 2)
	if len(parts) == 2 {
		imageURI = parts[1]
	}

	// Replace special characters with hyphens
	name := imageURI
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Convert to lowercase for k8s compliance
	name = strings.ToLower(name)

	// Trim to max 253 characters (k8s name limit)
	if len(name) > 253 {
		name = name[:253]
	}

	// Remove trailing hyphens
	name = strings.TrimSuffix(name, "-")

	return name
}

// SetupWithManager sets up the controller with the Manager
func (r *AIMClusterModelSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aim-cluster-model-source-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterModelSource{}).
		Owns(&aimv1alpha1.AIMClusterModel{}).
		Named("aim-cluster-model-source").
		Complete(r)
}
