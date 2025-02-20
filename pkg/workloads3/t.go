// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workloads3

import (
	"context"
	"fmt"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workloadshared "github.com/silogen/kaiwo/pkg/workloads/shared"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Object    T
}

// Reconciler is the
type Reconciler[T client.Object] interface {
	// BuildAll returns a list of all the objects that would be created
	// BuildAll(ctx context.Context, k8sClient client.Client) ([]client.Object, error)

	// Run runs through the reconciliation loop
	Run(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, dryRun bool) (ctrl.Result, []client.Object, error)
}

type KaiwoJobReconciler struct {
	ReconcilerBase[*v1alpha1.KaiwoJob]
	DownloadJob    DownloadJobReconciler
	HuggingFacePVC PvcReconciler
	DataPVC        PvcReconciler
	BatchJob       BatchJobReconciler
	RayJob         RayJobReconciler
}

func NewKaiwoJobReconciler(kaiwoJob *v1alpha1.KaiwoJob) KaiwoJobReconciler {
	sanitize(kaiwoJob)

	reconciler := KaiwoJobReconciler{
		ReconcilerBase: ReconcilerBase[*v1alpha1.KaiwoJob]{
			Object:    kaiwoJob,
			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
		},
		DownloadJob: NewDownloadJob(),
	}

	return reconciler
}

func (r *KaiwoJobReconciler) Run(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, dryRun bool) (ctrl.Result, []client.Object, error) {
	kaiwoJob := r.Object
	var manifests []client.Object

	// Stop if finished or error?
	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {

		if storageSpec.HasObjectStorageDownloads() {
			dataPvc, _, err := r.DataPVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, err
			}
			manifests = append(manifests, dataPvc)
		}

		if storageSpec.HasHfDownloads() {
			// Add HuggingFace PVC
			hfPvc, _, err := r.HuggingFacePVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, err
			}
			manifests = append(manifests, hfPvc)
		}

		if storageSpec.HasDownloads() {
			downloadJob, downloadJobResult, err := r.DownloadJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, err
			}
			if downloadJobResult != nil {
				return *downloadJobResult, nil, nil
			}
			manifests = append(manifests, downloadJob)
		}
	}

	var status v1alpha1.KaiwoJobStatus

	if kaiwoJob.Spec.IsBatchJob() {
		batchJob, _, err := r.BatchJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		manifests = append(manifests, batchJob)
		if !dryRun {
			status = getStatusFromBatchJob(batchJob)
		}
	} else if kaiwoJob.Spec.IsRayJob() {
		rayJob, _, err := r.RayJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
		if err != nil {
			return ctrl.Result{}, nil, err
		}
		manifests = append(manifests, rayJob)
		if !dryRun {
			status = getStatusFromRayJob(rayJob)
		}
	} else {
		panic("Unsupported job configuration")
	}

	if dryRun {
		return ctrl.Result{}, manifests, nil
	}

	// Reload kaiwojob?

	kaiwoJob.Status = status
	if err := k8sClient.Status().Update(ctx, kaiwoJob); err != nil {
		return ctrl.Result{}, nil, err
	}

	return ctrl.Result{}, nil, nil
}

func getStatusFromBatchJob(job *batchv1.Job) v1alpha1.KaiwoJobStatus {
	return v1alpha1.KaiwoJobStatus{}
}

func getStatusFromRayJob(job *rayv1.RayJob) v1alpha1.KaiwoJobStatus {
	return v1alpha1.KaiwoJobStatus{}
}

func sanitize(kaiwoJob *v1alpha1.KaiwoJob) {
	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {

		// Ensure mount paths are set
		if storageSpec.Data != nil && storageSpec.Data.IsRequested() && storageSpec.Data.MountPath == "" {
			// logger.Info("Data storage mount path not set, using default:" + defaultDataMountPath)
			storageSpec.Data.MountPath = workloadshared.DefaultDataMountPath
		}
		if storageSpec.HuggingFace != nil && storageSpec.HuggingFace.IsRequested() && storageSpec.HuggingFace.MountPath == "" {
			// logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
			storageSpec.HuggingFace.MountPath = workloadshared.DefaultHfMountPath
		}
	}

	if baseutils.ValueOrDefault(kaiwoJob.Spec.Image) == "" {
		kaiwoJob.Spec.Image = baseutils.Pointer(baseutils.DefaultRayImage)
	}
}

type DependentBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Desired   T
	Actual    T
	Self      Dependent[T]
}

func (d *DependentBase[T]) Create(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, desired T, owner client.Object) error {
	if owner != nil {
		if err := ctrl.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}
	return k8sClient.Create(ctx, desired)
}

func (d *DependentBase[T]) Update(ctx context.Context, k8sClient client.Client, desired T) error {
	// TODO
	return nil
}

func (d *DependentBase[T]) Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, owner client.Object, dryRun bool) (actual T, result *ctrl.Result, err error) {
	var empty T // nil or default value
	desired, err := d.Self.Build(ctx, k8sClient)
	if err != nil {
		return empty, nil, fmt.Errorf("failed to build object: %w", err)
	}

	if dryRun {
		return desired, nil, nil
	}

	actual, err = d.Self.Get(ctx, k8sClient)
	if err == nil {
		// Object exists
		err = d.Update(ctx, k8sClient, desired)
		if err != nil {
			return empty, nil, fmt.Errorf("failed to update object: %w", err)
		}
		return desired, nil, nil
	} else if errors.IsNotFound(err) {
		// Object doesn't exist
		err = d.Create(ctx, k8sClient, scheme, desired, owner)
		if err != nil {
			return empty, nil, fmt.Errorf("failed to create object: %w", err)
		}
		return actual, d.ShouldContinue(), nil
	} else {
		return empty, nil, fmt.Errorf("failed to get object: %w", err)
	}
}

func (d *DependentBase[T]) Get(ctx context.Context, k8sClient client.Client) (actual *T, err error) {
	var obj T
	if err := k8sClient.Get(ctx, d.ObjectKey, obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (d *DependentBase[T]) ShouldContinue() *ctrl.Result {
	return nil
}

type Dependent[T client.Object] interface {
	// Build will build the client object without creating it
	Build(ctx context.Context, k8sClient client.Client) (desired T, err error)

	// Get will fetch the client object
	Get(ctx context.Context, k8sClient client.Client) (actual T, err error)

	// Create will create the client object
	Create(ctx context.Context, k8sClient client.Client, desired T) error

	// Update will update the client object
	Update(ctx context.Context, k8sClient client.Client, desired T) error

	// Reconcile will build and then create
	Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, dryRun bool) (actual T, result *ctrl.Result, err error)

	// ShouldContinue returns a reconciliation result if there is an intermediate result to return
	ShouldContinue() *ctrl.Result
}

type DownloadJobReconciler struct {
	DependentBase[*batchv1.Job]
	Value string
}

func NewDownloadJob() DownloadJobReconciler {
	return DownloadJobReconciler{
		DependentBase: DependentBase[*batchv1.Job]{
			ObjectKey: client.ObjectKey{},
		},
		Value: "hi",
	}
}

func (d *DownloadJobReconciler) Build(ctx context.Context, k8sClient client.Client) (desired *batchv1.Job, err error) {
	return nil, nil
}

type PvcReconciler struct {
	DependentBase[*corev1.PersistentVolumeClaim]
}

func (p *PvcReconciler) Build(ctx context.Context, k8sClient client.Client) (desired *corev1.PersistentVolumeClaim, err error) {
	return nil, nil
}

type BatchJobReconciler struct {
	DependentBase[*batchv1.Job]
}

func (j *BatchJobReconciler) Build(ctx context.Context, k8sClient client.Client) (desired *rayv1.RayJob, err error) {
	return nil, nil
}

type RayJobReconciler struct {
	DependentBase[*rayv1.RayJob]
}

func (j *RayJobReconciler) Build(ctx context.Context, k8sClient client.Client) (desired *rayv1.RayJob, err error) {
	return nil, nil
}
