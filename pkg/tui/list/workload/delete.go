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

package workloadlist

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/charmbracelet/huh/spinner"

	"github.com/charmbracelet/huh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func runDeleteWorkload(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	confirmDelete := false
	resourceDescription := fmt.Sprintf("Confirm that you want to delete the %s workload '%s' in namespace %s. "+
		"This will also remove any linked resources, such as automatically created PVCs and ConfigMaps",
		state.WorkloadType, state.WorkloadReference.GetName(), state.Namespace)

	f := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(resourceDescription).Value(&confirmDelete)))

	err := f.Run()
	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to fetch input: %w", err)
	}

	if !confirmDelete {
		logrus.Debug("Delete cancelled")
		return tuicomponents.StepResultPrevious, nil, nil
	}

	deletePropagationPolicy := client.PropagationPolicy(metav1.DeletePropagationBackground)

	deleteWorkload := func() {
		err = clients.Client.Delete(ctx, state.WorkloadReference.GetObject(), deletePropagationPolicy)
	}

	if spinnerErr := spinner.New().Title("Deleting workload").Action(deleteWorkload).Run(); spinnerErr != nil {
		return tuicomponents.StepResultErr, nil, spinnerErr
	}

	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to delete workload: %w", err)
	}

	logrus.Infof("Successfully deleted workload %s/%s", state.WorkloadType, state.WorkloadReference.GetName())

	// Quit as otherwise would need to return two screens, TODO make it possible to implement a custom number of return steps later
	return tuicomponents.StepResultQuit, nil, nil
}
