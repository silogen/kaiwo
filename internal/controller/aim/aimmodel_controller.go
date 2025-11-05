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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	aimImageFieldOwner = "aim-image-controller"
)

// AIMModelReconciler reconciles an AIMModel object
type AIMModelReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AIMModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the image
	var image aimv1alpha1.AIMModel
	if err := r.Get(ctx, req.NamespacedName, &image); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling AIMModel", "name", image.Name, "namespace", image.Namespace)

	// Use framework orchestrator
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMModel, aimv1alpha1.AIMModelStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &image,
		FieldOwner: aimImageFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &image)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var observation *shared.ImageObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ImageObservation)
				if !ok {
					return nil, baseutils.LogErrorf(logger, "unexpected observation type", nil)
				}
			}
			return r.plan(ctx, &image, observation)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var observation *shared.ImageObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ImageObservation)
				if !ok {
					return baseutils.LogErrorf(logger, "unexpected observation type", nil)
				}
			}
			return r.projectStatus(ctx, &image, observation, errs)
		},

		FinalizeFn: nil, // No external cleanup needed
	})
}

// observe gathers current cluster state (read-only)
func (r *AIMModelReconciler) observe(ctx context.Context, image *aimv1alpha1.AIMModel) (*shared.ImageObservation, error) {
	logger := log.FromContext(ctx)
	return shared.ObserveImage(ctx, shared.ImageObservationOptions{
		GetRuntimeConfig: func(ctx context.Context) (*shared.RuntimeConfigResolution, error) {
			// Look for AIMRuntimeConfig named "default" in the same namespace
			resolution, err := shared.ResolveRuntimeConfig(ctx, r.Client, image.Namespace, shared.DefaultRuntimeConfigName)
			if err != nil {
				// If not found, that's okay - we'll proceed without image pull secrets
				baseutils.Debug(logger, "Runtime config not found, proceeding without image pull secrets",
					"namespace", image.Namespace, "name", shared.DefaultRuntimeConfigName)
				return &shared.RuntimeConfigResolution{}, nil
			}
			return resolution, nil
		},

		ListOwnedTemplates: func(ctx context.Context) ([]client.Object, error) {
			// List AIMServiceTemplates owned by this image
			var templates aimv1alpha1.AIMServiceTemplateList
			if err := r.List(ctx, &templates,
				client.InNamespace(image.Namespace),
				client.MatchingLabels{
					shared.LabelKeyAutoGenerated: shared.LabelValueAutoGenerated,
					shared.LabelKeyImageName:     image.Name,
				},
			); err != nil {
				return nil, err
			}

			objects := make([]client.Object, len(templates.Items))
			for i := range templates.Items {
				objects[i] = &templates.Items[i]
			}
			return objects, nil
		},

		GetCurrentStatus: func() *aimv1alpha1.AIMModelStatus {
			return &image.Status
		},

		GetImageSpec: func() aimv1alpha1.AIMModelSpec {
			return image.Spec
		},
	})
}

// plan computes desired state (pure function)
func (r *AIMModelReconciler) plan(ctx context.Context, image *aimv1alpha1.AIMModel, obs *shared.ImageObservation) ([]client.Object, error) {
	logger := log.FromContext(ctx)
	// Build owner reference
	ownerRef := []metav1.OwnerReference{
		{
			APIVersion:         image.APIVersion,
			Kind:               image.Kind,
			Name:               image.Name,
			UID:                image.UID,
			Controller:         baseutils.Pointer(true),
			BlockOwnerDeletion: baseutils.Pointer(true),
		},
	}

	// Plan resources using shared logic
	desired, _, err := shared.PlanImageResources(ctx, shared.ImagePlanInput{
		ImageName:       image.Name,
		Namespace:       image.Namespace,
		ImageSpec:       image.Spec,
		Observation:     obs,
		OwnerReference:  ownerRef,
		Clientset:       r.Clientset,
		IsClusterScoped: false,
	})

	if err != nil {
		baseutils.Debug(logger, "Plan failed for image", "error", err)
	} else {
		baseutils.Debug(logger, "Planned image resources", "desiredCount", len(desired))
	}

	return desired, err
}

// projectStatus computes status from observation + errors (modifies image.Status directly)
func (r *AIMModelReconciler) projectStatus(
	ctx context.Context,
	image *aimv1alpha1.AIMModel,
	obs *shared.ImageObservation,
	errs controllerutils.ReconcileErrors,
) error {
	logger := log.FromContext(ctx)
	// Extract metadata and error from the plan execution
	var extractedMetadata *aimv1alpha1.ImageMetadata
	var extractionErr error

	// Check if there was a plan error
	if errs.PlanErr != nil {
		extractionErr = errs.PlanErr
		logger.Error(errs.PlanErr, "Plan error occurred for image")
	}

	if obs != nil && obs.MetadataError != nil {
		extractionErr = obs.MetadataError
	}
	if obs != nil && obs.MetadataExtractionErr != nil && extractionErr == nil {
		extractionErr = obs.MetadataExtractionErr
	}

	// If we successfully extracted metadata in this reconciliation, use it
	// Otherwise, use what's already in the observation
	if obs != nil && obs.ImageMetadata != nil {
		extractedMetadata = obs.ImageMetadata
		var deploymentCount int
		if extractedMetadata.Model != nil {
			deploymentCount = len(extractedMetadata.Model.RecommendedDeployments)
		}
		baseutils.Debug(logger, "Extracted image metadata",
			"hasModel", extractedMetadata.Model != nil,
			"deploymentCount", deploymentCount)
	}

	// Re-run the plan to get the extracted metadata if extraction was attempted
	if obs != nil && !obs.MetadataAlreadyAttempted {
		ownerRef := []metav1.OwnerReference{
			{
				APIVersion:         image.APIVersion,
				Kind:               image.Kind,
				Name:               image.Name,
				UID:                image.UID,
				Controller:         baseutils.Pointer(true),
				BlockOwnerDeletion: baseutils.Pointer(true),
			},
		}

		_, metadata, err := shared.PlanImageResources(ctx, shared.ImagePlanInput{
			ImageName:       image.Name,
			Namespace:       image.Namespace,
			ImageSpec:       image.Spec,
			Observation:     obs,
			OwnerReference:  ownerRef,
			Clientset:       r.Clientset,
			IsClusterScoped: false,
		})

		if err != nil {
			extractionErr = err
		} else {
			extractedMetadata = metadata
		}
	}

	// Capture old status for comparison
	oldStatus := image.Status.Status

	// Update status using shared logic
	shared.ProjectImageStatus(
		&image.Status,
		image.Spec,
		obs,
		extractedMetadata,
		extractionErr,
		image.Generation,
	)

	// Add canonical name label if metadata was successfully extracted
	if extractedMetadata != nil && extractedMetadata.Model != nil && extractedMetadata.Model.CanonicalName != "" {
		if image.Labels == nil {
			image.Labels = make(map[string]string)
		}
		image.Labels[shared.LabelKeyModelCanonicalName] = shared.SanitizeLabelValue(extractedMetadata.Model.CanonicalName)
	}

	// Log and emit events for status transitions
	if oldStatus != image.Status.Status {
		logger.Info("Image status changed",
			"name", image.Name,
			"namespace", image.Namespace,
			"previousStatus", oldStatus,
			"newStatus", image.Status.Status)

		switch image.Status.Status {
		case aimv1alpha1.AIMModelStatusReady:
			var deploymentCount int
			if image.Status.ImageMetadata != nil && image.Status.ImageMetadata.Model != nil {
				deploymentCount = len(image.Status.ImageMetadata.Model.RecommendedDeployments)
			}
			controllerutils.EmitNormalEvent(r.Recorder, image, "ImageReady",
				fmt.Sprintf("Image %s is ready with %d recommended deployments", image.Name, deploymentCount))
		case aimv1alpha1.AIMModelStatusFailed:
			msg := "Image processing failed"
			if extractionErr != nil {
				msg = fmt.Sprintf("Image processing failed: %v", extractionErr)
			}
			controllerutils.EmitWarningEvent(r.Recorder, image, "ImageFailed", msg)
		case aimv1alpha1.AIMModelStatusProgressing:
			baseutils.Debug(logger, "Image processing in progress")
		}
	}

	return nil
}

func (r *AIMModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aim-image-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMModel{}).
		Owns(&aimv1alpha1.AIMServiceTemplate{}).
		Named("aim-image").
		Complete(r)
}
