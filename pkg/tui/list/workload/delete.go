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

package workloadlist

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func runDeleteWorkload(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	confirmDelete := false
	obj := state.Workload.GetKaiwoWorkloadObject()
	resourceDescription := fmt.Sprintf("Confirm that you want to delete the %s workload '%s' in namespace %s. "+
		"This will also remove any linked resources, such as automatically created PVCs and ConfigMaps",
		state.WorkloadType, obj.GetName(), state.Namespace)

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

	obj, ok := state.Workload.(client.Object)
	if !ok {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to assert workload client to type %T", state.Workload)
	}

	deleteWorkload := func() {
		err = clients.Client.Delete(ctx, obj, deletePropagationPolicy)
	}

	if spinnerErr := spinner.New().Title("Deleting workload").Action(deleteWorkload).Run(); spinnerErr != nil {
		return tuicomponents.StepResultErr, nil, spinnerErr
	}

	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to delete workload: %w", err)
	}

	logrus.Infof("Successfully deleted workload %s/%s", state.WorkloadType, obj.GetName())

	// Quit as otherwise would need to return two screens, TODO make it possible to implement a custom number of return steps later
	return tuicomponents.StepResultQuit, nil, nil
}
