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

package tui

import (
	"context"
	"fmt"

	tuicomponents2 "github.com/silogen/kaiwo/pkg/cli/tui/components"
	"github.com/silogen/kaiwo/pkg/cli/tui/list/workload"

	"github.com/silogen/kaiwo/pkg/kube/utils"
)

func RunList(workloadType string, workloadName string, namespace string, user string) error {
	if workloadType == "" && workloadName != "" {
		return fmt.Errorf("cannot determine workload from name without a type")
	}

	ctx := context.Background()

	runState := &tuicomponents2.RunState{
		WorkloadType: workloadType,
		User:         user,
		Namespace:    namespace,
	}

	clients, err := utils.GetKubernetesClients()
	if err != nil {
		return fmt.Errorf("failed to get k8s clients: %w", err)
	}

	if err := tuicomponents2.RunSteps(ctx, *clients, runState, workloadlist.RunSelectWorkloadType); err != nil {
		return fmt.Errorf("failed to run steps: %w", err)
	}
	return nil
}
