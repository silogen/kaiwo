package workloadjob

import (
	"context"
	"fmt"

	workloadshared "github.com/silogen/kaiwo/pkg/workloads2/shared"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildKaiwoJobInvoker(ctx context.Context, scheme *runtime.Scheme, manifest *kaiwov1alpha1.KaiwoJob) (workloadutils.CommandInvoker, error) {
	invoker := workloadutils.CommandInvoker{}

	state := &workloadutils.CommandStateBase{}

	invoker.AddCommand(&KaiwoJobManifestCommand{KaiwoJob: manifest})

	base := workloadutils.CommandBase[workloadutils.CommandStateBase]{
		Owner:   manifest,
		Scheme:  scheme,
		State:   state,
		Context: ctx,
	}

	storageSpec := manifest.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {
		if storageSpec.Data != nil {
			// Add data PVC
			invoker.AddCommand(&workloadshared.StorageCommand[workloadutils.CommandStateBase]{
				StoreInto:        state.DataPvc,
				AccessMode:       storageSpec.AccessMode,
				Amount:           storageSpec.Data.StorageSize,
				StorageClassName: storageSpec.StorageClassName,
				PvcNamePostfix:   "data",
				CommandBase:      base,
				StoreCallback:    workloadshared.SetStorage,
			})
		}
		if storageSpec.HuggingFace != nil {
			// Add HuggingFace PVC
			invoker.AddCommand(&workloadshared.StorageCommand[workloadutils.CommandStateBase]{
				StoreInto:        state.DataPvc,
				AccessMode:       storageSpec.AccessMode,
				Amount:           storageSpec.Data.StorageSize,
				StorageClassName: storageSpec.StorageClassName,
				PvcNamePostfix:   "hf",
				CommandBase:      base,
				StoreCallback:    workloadshared.SetStorage,
			})
		}

		if storageSpec.HasObjectStorageDownloads() || storageSpec.HasObjectStorageDownloads() {
			// Add download job
			invoker.AddCommand(&workloadshared.DownloadJobConfigMapCommand{
				StorageSpec: storageSpec,
				CommandBase: base,
			})
			invoker.AddCommand(&workloadshared.DownloadJobCommand{
				StorageSpec: storageSpec,
				CommandBase: base,
			})
		}
	}

	if manifest.Spec.Image == nil || *manifest.Spec.Image == "" {
		manifest.Spec.Image = baseutils.Pointer(baseutils.DefaultRayImage)
	}

	if manifest.Spec.RayClusterSpec == nil || (manifest.Spec.Ray != nil && !*manifest.Spec.Ray) {
		invoker.AddCommand(&BatchJobCommand{
			CommandBase: base,
			KaiwoJob:    manifest,
		})
	} else if manifest.Spec.RayClusterSpec != nil || (manifest.Spec.Ray != nil && *manifest.Spec.Ray) {
		invoker.AddCommand(&RayJobCommand{
			CommandBase: base,
			// KaiwoJob:    manifest,
		})
	} else {
		return workloadutils.CommandInvoker{}, fmt.Errorf("KaiwoJob does not specify a valid Job or RayJob. You must either set ray to true or false, or provide a rayClusterSpec")
		// logger.Error(jobInvalidErr, "KaiwoJob is misconfigured", "KaiwoJob", manifest.Name)
	}

	return invoker, nil
}

//type KaiwoJobCommandState struct {
//	workloadutils.CommandStateBase
//	KaiwoJob *kaiwov1alpha1.KaiwoJob
//}

type KaiwoJobManifestCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *kaiwov1alpha1.KaiwoJob
}

func (k *KaiwoJobManifestCommand) Build() (client.Object, error) {
	job := k.KaiwoJob

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}

	user := ""
	if job.Spec.User != nil {
		user = *job.Spec.User
	}

	// TODO enable
	//if job.Spec.ClusterQueue == nil {
	//	return nil, fmt.Errorf("KaiwoJob does not specify a ClusterQueue")
	//}

	job.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(user)
	// job.Labels[kaiwov1alpha1.QueueLabel] = *job.Spec.ClusterQueue

	return nil, nil
}
