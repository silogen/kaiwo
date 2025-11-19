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
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
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
		FinalizeFn: func(ctx context.Context, obs any) error {
			_, err := r.finalize(ctx, &kvc)
			return err
		},
	})
}

// observation holds read-only snapshot of dependent resources
type kvObservation struct {
	statefulSetFound bool
	statefulSet      appsv1.StatefulSet
	serviceFound     bool
	service          corev1.Service

	// derived states
	statefulSetReady bool
	serviceReady     bool
}

func (r *AIMKVCacheReconciler) observe(ctx context.Context, kvc *aimv1alpha1.AIMKVCache) (*kvObservation, error) {
	logger := log.FromContext(ctx)
	obs := &kvObservation{}

	// Observe StatefulSet
	statefulSetName := r.getStatefulSetName(kvc)
	var statefulSet appsv1.StatefulSet
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: kvc.Namespace,
		Name:      statefulSetName,
	}, &statefulSet); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get StatefulSet", "name", statefulSetName)
			return nil, err
		}
	} else {
		obs.statefulSetFound = true
		obs.statefulSet = statefulSet
		obs.statefulSetReady = statefulSet.Status.ReadyReplicas > 0
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

func (r *AIMKVCacheReconciler) getStatefulSetName(kvc *aimv1alpha1.AIMKVCache) string {
	return fmt.Sprintf("%s-%s", kvc.Name, kvc.Spec.KVCacheType)
}

func (r *AIMKVCacheReconciler) getServiceName(kvc *aimv1alpha1.AIMKVCache) string {
	return fmt.Sprintf("%s-%s-svc", kvc.Name, kvc.Spec.KVCacheType)
}

func (r *AIMKVCacheReconciler) getStorageSize(kvc *aimv1alpha1.AIMKVCache) resource.Quantity {
	// Return user-specified size if provided
	if kvc.Spec.Storage != nil && kvc.Spec.Storage.Size != nil {
		return *kvc.Spec.Storage.Size
	}
	// Default to 1Gi
	return resource.MustParse("1Gi")
}

func (r *AIMKVCacheReconciler) getStorageClassName(kvc *aimv1alpha1.AIMKVCache) *string {
	// Return user-specified storage class if provided
	if kvc.Spec.Storage != nil && kvc.Spec.Storage.StorageClassName != nil {
		return kvc.Spec.Storage.StorageClassName
	}
	// Return nil to use cluster default
	return nil
}

func (r *AIMKVCacheReconciler) getStorageAccessModes(kvc *aimv1alpha1.AIMKVCache) []corev1.PersistentVolumeAccessMode {
	// Return user-specified access modes if provided
	if kvc.Spec.Storage != nil && len(kvc.Spec.Storage.AccessModes) > 0 {
		return kvc.Spec.Storage.AccessModes
	}
	// Default to ReadWriteOnce for StatefulSet PVCs
	return []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
}

func (r *AIMKVCacheReconciler) plan(ctx context.Context, kvc *aimv1alpha1.AIMKVCache, obs *kvObservation) ([]client.Object, error) {
	var desired []client.Object

	// Only support Redis for now
	if kvc.Spec.KVCacheType != "redis" && kvc.Spec.KVCacheType != "" {
		return nil, fmt.Errorf("unsupported KVCacheType: %s, only 'redis' is supported", kvc.Spec.KVCacheType)
	}

	// Build Redis Service (must be created before StatefulSet)
	service := r.buildRedisService(kvc)
	desired = append(desired, service)

	// Build Redis StatefulSet
	statefulSet := r.buildRedisStatefulSet(kvc)
	desired = append(desired, statefulSet)

	return desired, nil
}

// finalize handles cleanup when the resource is being deleted
func (r *AIMKVCacheReconciler) finalize(ctx context.Context, kvc *aimv1alpha1.AIMKVCache) (bool, error) {
	// Currently no finalization logic needed
	return true, nil
}

func (r *AIMKVCacheReconciler) buildRedisStatefulSet(kvc *aimv1alpha1.AIMKVCache) *appsv1.StatefulSet {
	name := r.getStatefulSetName(kvc)
	serviceName := r.getServiceName(kvc)
	labels := map[string]string{
		"app":                         "redis",
		"aim.silogen.ai/kvcache":      kvc.Name,
		"aim.silogen.ai/kvcache-type": kvc.Spec.KVCacheType,
	}

	replicas := int32(1)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: kvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: serviceName,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:latest",
							Command: []string{
								"redis-server",
								"--appendonly", "yes",
								"--save", "60", "1",
								"--loglevel", "notice",
							},
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "redis-data",
									MountPath: "/data",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "redis-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      r.getStorageAccessModes(kvc),
						StorageClassName: r.getStorageClassName(kvc),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: r.getStorageSize(kvc),
							},
						},
					},
				},
			},
		},
	}

	// Set controller reference
	ctrl.SetControllerReference(kvc, statefulSet, r.Scheme)

	return statefulSet
}

func (r *AIMKVCacheReconciler) buildRedisService(kvc *aimv1alpha1.AIMKVCache) *corev1.Service {
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
			ClusterIP: "None", // Headless service for StatefulSet
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Name:       "redis",
				},
			},
			PublishNotReadyAddresses: true, // Allow DNS records for pods before they are ready
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

	// Set statefulset and service names
	status.StatefulSetName = r.getStatefulSetName(kvc)
	status.ServiceName = r.getServiceName(kvc)

	// Determine overall status
	if errs.HasError() {
		status.Status = aimv1alpha1.AIMKVCacheStatusFailed
		r.setFailureCondition(status, errs)
	} else if obs != nil && obs.statefulSetReady && obs.serviceReady {
		status.Status = aimv1alpha1.AIMKVCacheStatusReady
		r.setReadyCondition(status)
	} else if obs != nil && obs.statefulSetFound {
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
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetReady,
		Message: "Redis StatefulSet and service are ready",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetReady,
		Message: "StatefulSet is ready",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonNoFailure,
		Message: "No failures detected",
	})
}

func (r *AIMKVCacheReconciler) setProgressingCondition(status *aimv1alpha1.AIMKVCacheStatus, obs *kvObservation) {
	message := "Redis StatefulSet is progressing"
	reason := aimv1alpha1.AIMKVCacheReasonWaitingForPods

	if obs != nil {
		if obs.statefulSetFound && !obs.statefulSetReady {
			message = "StatefulSet exists but pods are not ready"
			reason = aimv1alpha1.AIMKVCacheReasonWaitingForPods
		} else if obs.statefulSetFound && !obs.serviceFound {
			message = "StatefulSet ready, creating service"
			reason = aimv1alpha1.AIMKVCacheReasonStatefulSetCreated
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
		Message: "StatefulSet is progressing",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonNoFailure,
		Message: "No failures detected",
	})
}

func (r *AIMKVCacheReconciler) setPendingCondition(status *aimv1alpha1.AIMKVCacheStatus, obs *kvObservation) {
	message := "Waiting for StatefulSet to be ready"
	if obs != nil {
		if !obs.statefulSetFound {
			message = "StatefulSet not found, creating"
		} else if !obs.statefulSetReady {
			message = "StatefulSet found but not ready"
		} else if !obs.serviceFound {
			message = "Service not found, creating"
		}
	}

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetPending,
		Message: message,
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetPending,
		Message: "StatefulSet not started yet",
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
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetFailed,
		Message: "StatefulSet failed",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionProgressing,
		Status:  metav1.ConditionFalse,
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetFailed,
		Message: "StatefulSet failed",
	})

	r.setCondition(status, metav1.Condition{
		Type:    aimv1alpha1.AIMKVCacheConditionFailure,
		Status:  metav1.ConditionTrue,
		Reason:  aimv1alpha1.AIMKVCacheReasonStatefulSetFailed,
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
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Named("aimkvcache-controller").
		Complete(r)
}
