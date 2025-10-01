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

package common

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// StorageHandler handles the storage-related reconcilers (PVCs and download config map / job)
type StorageHandler struct {
	ObjectKey      client.ObjectKey
	CommonMetaSpec v1alpha1.CommonMetaSpec
}

func NewStorageHandler(workloadHandler WorkloadHandler) *StorageHandler {
	commonSpec := workloadHandler.Workload.GetCommonSpec()
	if commonSpec.Storage == nil || !commonSpec.Storage.StorageEnabled {
		return nil
	}
	return &StorageHandler{
		ObjectKey:      client.ObjectKeyFromObject(workloadHandler.Workload.GetKaiwoWorkloadObject()),
		CommonMetaSpec: workloadHandler.Workload.GetCommonSpec(),
	}
}

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

func (h StorageHandler) GetResourceReconcilers() []ResourceReconciler {
	var reconcilers []ResourceReconciler
	storageSpec := h.CommonMetaSpec.Storage

	if storageSpec.HasData() {
		reconcilers = append(reconcilers, NewPvcReconciler(
			client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(h.ObjectKey.Name, DataStoragePostfix),
				Namespace: h.ObjectKey.Namespace,
			},
			storageSpec.Data.StorageSize,
			storageSpec.StorageClassName,
			storageSpec.AccessMode,
		))
	}
	if storageSpec.HasHfDownloads() {
		reconcilers = append(reconcilers, NewPvcReconciler(
			client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(h.ObjectKey.Name, HfStoragePostfix),
				Namespace: h.ObjectKey.Namespace,
			},
			storageSpec.HuggingFace.StorageSize,
			storageSpec.StorageClassName,
			storageSpec.AccessMode,
		))
	}
	if storageSpec.HasDownloads() {
		reconcilers = append(
			reconcilers,
			NewDownloadConfigMapReconciler(
				h.ObjectKey,
				*storageSpec,
			),
			NewDownloadJobReconciler(
				h.ObjectKey,
				*storageSpec,
				h.CommonMetaSpec.Env,
			))
	}

	return reconcilers
}

func (h StorageHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, previousWorkloadStatus v1alpha1.WorkloadStatus) (v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	status, conditions, err := ObserveOverallStatus(ctx, k8sClient, h.GetResourceReconcilers(), previousWorkloadStatus)
	if status == nil {
		return "", conditions, err
	}
	return *status, conditions, err
}
