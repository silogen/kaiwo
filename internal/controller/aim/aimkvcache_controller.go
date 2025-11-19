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

	"k8s.io/client-go/tools/record"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

// AIMKVCacheReconciler reconciles a AIMKVCache object
type AIMKVCacheReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

const (
	kvCacheFieldOwner = "aimkvcache-controller"
)

// RBAC markers
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimkvcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimkvcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimkvcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *AIMKVCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch CR
	var kvc aimv1alpha1.AIMKVCache
	if err := r.Get(ctx, req.NamespacedName, &kvc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	baseutils.Debug(logger, "Reconciling AIMKVCache", "name", kvc.Name, "namespace", kvc.Namespace, "kvCacheType", kvc.Spec.KVCacheType)

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMKVCache, aimv1alpha1.AIMKVCacheStatus]{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Object:   &kvc,
		Recorder: r.Recorder,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &kvc)
		},
		FieldOwner: kvCacheFieldOwner,
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *kvObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*kvObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &kvc, o)
		},
		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *kvObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*kvObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &kvc, o, errs)
		},
		FinalizeFn: nil,
	})
}

// observation holds read-only snapshot of dependent resources
type kvObservation struct {
	deploymentFound bool
	deployment      appsv1.Deployment
	serviceFound    bool
	service         corev1.Service

	// derived states
	deploymentReady bool
	serviceReady    bool
}

func (r *AIMKVCacheReconciler) observe(ctx context.Context, kvc *aimv1alpha1.AIMKVCache) (*kvObservation, error) {
	logger := log.FromContext(ctx)
	obs := &kvObservation{}

	// Observe Deployment
	deploymentName := r.getDeploymentName(kvc)
	var deployment appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: kvc.Namespace,
		Name:      deploymentName,
	}, &deployment); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Deployment", "name", deploymentName)
			return nil, err
		}
	} else {
		obs.deploymentFound = true
		obs.deployment = deployment
		obs.deploymentReady = deployment.Status.ReadyReplicas > 0
	}

	// Observe Service
	serviceName := r.getServiceName(kvc)
	var service corev1.Service
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: kvc.Namespace,
		Name:      serviceName,
	}, &service); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Service", "name", serviceName)
			return nil, err
		}
	} else {
		obs.serviceFound = true
		obs.service = service
		obs.serviceReady = true // Services are ready when they exist
	}

	return obs, nil
}

func (r *AIMKVCacheReconciler) getDeploymentName(kvc *aimv1alpha1.AIMKVCache) string {
	return fmt.Sprintf("%s-%s", kvc.Name, kvc.Spec.KVCacheType)
}

func (r *AIMKVCacheReconciler) getServiceName(kvc *aimv1alpha1.AIMKVCache) string {
	return fmt.Sprintf("%s-%s-svc", kvc.Name, kvc.Spec.KVCacheType)
}

func (r *AIMKVCacheReconciler) plan(ctx context.Context, kvc *aimv1alpha1.AIMKVCache, obs *kvObservation) ([]client.Object, error) {
	var desired []client.Object

	// Only support Redis for now
	if kvc.Spec.KVCacheType != "redis" && kvc.Spec.KVCacheType != "" {
		return nil, fmt.Errorf("unsupported KVCacheType: %s, only 'redis' is supported", kvc.Spec.KVCacheType)
	}

	// Create Redis Deployment
	deployment := r.createRedisDeployment(kvc)
	desired = append(desired, deployment)

	// Create Redis Service
	service := r.createRedisService(kvc)
	desired = append(desired, service)

	return desired, nil
}

func (r *AIMKVCacheReconciler) createRedisDeployment(kvc *aimv1alpha1.AIMKVCache) *appsv1.Deployment {
	name := r.getDeploymentName(kvc)
	labels := map[string]string{
		"app":                         "redis",
		"aim.silogen.ai/kvcache":      kvc.Name,
		"aim.silogen.ai/kvcache-type": kvc.Spec.KVCacheType,
	}

	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: kvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:latest",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}

	// Set controller reference
	ctrl.SetControllerReference(kvc, deployment, r.Scheme)

	return deployment
}

func (r *AIMKVCacheReconciler) createRedisService(kvc *aimv1alpha1.AIMKVCache) *corev1.Service {
	name := r.getServiceName(kvc)
	labels := map[string]string{
		"app":                         "redis",
		"aim.silogen.ai/kvcache":      kvc.Name,
		"aim.silogen.ai/kvcache-type": kvc.Spec.KVCacheType,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: kvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Name:       "redis",
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Set controller reference
	ctrl.SetControllerReference(kvc, service, r.Scheme)

	return service
}

func (r *AIMKVCacheReconciler) projectStatus(ctx context.Context, kvc *aimv1alpha1.AIMKVCache, obs *kvObservation, errs controllerutils.ReconcileErrors) error {
	status := kvc.GetStatus()

	// Set observed generation
	status.ObservedGeneration = kvc.Generation

	// Set deployment and service names
	status.DeploymentName = r.getDeploymentName(kvc)
	status.ServiceName = r.getServiceName(kvc)

	// Determine overall status
	if errs.HasError() {
		status.Status = aimv1alpha1.AIMKVCacheStatusFailed
		r.setFailureCondition(status, errs)
	} else if obs != nil && obs.deploymentReady && obs.serviceReady {
		status.Status = aimv1alpha1.AIMKVCacheStatusReady
		r.setReadyCondition(status)
	} else if obs != nil && obs.deploymentFound {
		status.Status = aimv1alpha1.AIMKVCacheStatusProgressing
		r.setProgressingCondition(status, obs)
	} else {
		status.Status = aimv1alpha1.AIMKVCacheStatusPending
		r.setPendingCondition(status, obs)
	}

	return nil
}

func (r *AIMKVCacheReconciler) setReadyCondition(status *aimv1alpha1.AIMKVCacheStatus) {
	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentReady,
		Message: "Redis deployment and service are ready",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentReady,
		Message: "Deployment is ready",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonNoFailure,
		Message: "No failures detected",
	})
}

func (r *AIMKVCacheReconciler) setProgressingCondition(status *aimv1alpha1.AIMKVCacheStatus, obs *kvObservation) {
	message := "Redis deployment is progressing"
	reason := aimv1alpha1.AIMKVCacheReasonWaitingForPods

	if obs != nil {
		if obs.deploymentFound && !obs.deploymentReady {
			message = "Deployment exists but pods are not ready"
			reason = aimv1alpha1.AIMKVCacheReasonWaitingForPods
		} else if obs.deploymentFound && !obs.serviceFound {
			message = "Deployment ready, creating service"
			reason = aimv1alpha1.AIMKVCacheReasonDeploymentCreated
		}
	}

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonWaitingForPods,
		Message: "Deployment is progressing",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonNoFailure,
		Message: "No failures detected",
	})
}

func (r *AIMKVCacheReconciler) setPendingCondition(status *aimv1alpha1.AIMKVCacheStatus, obs *kvObservation) {
	message := "Waiting for deployment to be ready"
	if obs != nil {
		if !obs.deploymentFound {
			message = "Deployment not found, creating"
		} else if !obs.deploymentReady {
			message = "Deployment found but not ready"
		} else if !obs.serviceFound {
			message = "Service not found, creating"
		}
	}

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentPending,
		Message: message,
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentPending,
		Message: "Deployment not started yet",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonNoFailure,
		Message: "No failures detected",
	})
}

func (r *AIMKVCacheReconciler) setFailureCondition(status *aimv1alpha1.AIMKVCacheStatus, errs controllerutils.ReconcileErrors) {
	message := "Unknown failure"
	if errs.PlanErr != nil {
		message = fmt.Sprintf("Planning failed: %v", errs.PlanErr)
	} else if errs.ApplyErr != nil {
		message = fmt.Sprintf("Apply failed: %v", errs.ApplyErr)
	} else if errs.ObserveErr != nil {
		message = fmt.Sprintf("Observation failed: %v", errs.ObserveErr)
	}

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentFailed,
		Message: "Deployment failed",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentFailed,
		Message: "Deployment failed",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionTrue,
		Reason:  aimv1alpha1.AIMKVCacheReasonDeploymentFailed,
		Message: message,
	})
}

func (r *AIMKVCacheReconciler) setCondition(status *aimv1alpha1.AIMKVCacheStatus, newCondition metav1.Condition) {
	newCondition.LastTransitionTime = metav1.Now()

	// Find existing condition
	for i, existing := range status.Conditions {
		if existing.Type == newCondition.Type {
			// Update if status changed
			if existing.Status != newCondition.Status || existing.Reason != newCondition.Reason {
				status.Conditions[i] = newCondition
			}
			return
		}
	}

	// Add new condition
	status.Conditions = append(status.Conditions, newCondition)
}

func (r *AIMKVCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aimkvcache-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMKVCache{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Named("aimkvcache-controller").
		Complete(r)
}
