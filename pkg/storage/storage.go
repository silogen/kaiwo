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

package storage

import (
	"context"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/api"
	"github.com/silogen/kaiwo/pkg/config"
	"github.com/silogen/kaiwo/pkg/jobs/download"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StorageHandler handles the storage-related reconcilers (PVCs and download config map / job)
type StorageHandler struct {
	ObjectKey      client.ObjectKey
	CommonMetaSpec v1alpha1.CommonMetaSpec
}

func NewStorageHandler(objectKey client.ObjectKey, commonSpec v1alpha1.CommonMetaSpec) *StorageHandler {
	if commonSpec.Storage == nil || !commonSpec.Storage.StorageEnabled {
		return nil
	}
	return &StorageHandler{
		ObjectKey:      objectKey,
		CommonMetaSpec: commonSpec,
	}
}

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

func (h StorageHandler) GetResourceReconcilers(ctx context.Context) []api.ResourceReconciler {
	var reconcilers []api.ResourceReconciler
	storageSpec := h.CommonMetaSpec.Storage

	config := config.ConfigFromContext(ctx)
	storageClassName := storageSpec.StorageClassName
	if storageClassName == "" {
		storageClassName = config.Storage.DefaultStorageClass
	}

	if storageSpec.HasData() {
		reconcilers = append(reconcilers, NewPvcReconciler(
			client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(h.ObjectKey.Name, DataStoragePostfix),
				Namespace: h.ObjectKey.Namespace,
			},
			storageSpec.Data.StorageSize,
			storageClassName,
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
			storageClassName,
			storageSpec.AccessMode,
		))
	}
	if storageSpec.HasDownloads() {
		reconcilers = append(
			reconcilers,
			download.NewDownloadConfigMapReconciler(
				h.ObjectKey,
				*storageSpec,
			),
			download.NewDownloadJobReconciler(
				h.ObjectKey,
				*storageSpec,
				h.CommonMetaSpec.Env,
			))
	}

	return reconcilers
}
