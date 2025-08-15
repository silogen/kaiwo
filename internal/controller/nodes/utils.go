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

package nodeutils

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
)

type ReconcileTask[T any] interface {
	Name() string
	Run(ctx context.Context, obj T) (*ctrl.Result, error)
}

type KaiwoNodeWrapper struct {
	Node      *corev1.Node
	KaiwoNode *v1alpha1.KaiwoNode
}
