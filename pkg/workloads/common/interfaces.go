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

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KaiwoWorkload interface {
	client.Object
	GetUser() string
	GetObjectMeta() *metav1.ObjectMeta
	GetStatusString() string
	GetType() string
	GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error)
	GetDuration() *metav1.Duration
	GetStartTime() *metav1.Time
	GetClusterQueue() string
	GetGPUVendor() string
	GetCommonStatusSpec() *v1alpha1.CommonStatusSpec
}
