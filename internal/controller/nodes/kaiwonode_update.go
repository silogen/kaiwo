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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UpdateKaiwoNodeTask struct {
	Client client.Client
}

func (t *UpdateKaiwoNodeTask) Name() string {
	return "EnsureKaiwoNode"
}

func (t *UpdateKaiwoNodeTask) Run(ctx context.Context, wrapper *KaiwoNodeWrapper) (*ctrl.Result, error) {
	return nil, nil
}
