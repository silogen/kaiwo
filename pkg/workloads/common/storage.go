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

func (h StorageHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, clusterCtx ClusterContext, previousWorkloadStatus v1alpha1.WorkloadStatus) (v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	status, conditions, err := ObserveOverallStatus(ctx, k8sClient, h.GetResourceReconcilers(), previousWorkloadStatus)
	if status == nil {
		return "", conditions, err
	}
	return *status, conditions, err
}
