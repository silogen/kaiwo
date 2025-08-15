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

package podspec

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// AddEntrypoint updates the entrypoint command in the PodTemplateSpec.
func AddEntrypoint(entrypoint string, podTemplateSpec *corev1.PodTemplateSpec) error {
	if entrypoint == "" {
		// logger.Info("Entrypoint is empty, skipping modification")
		return nil
	}

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	podTemplateSpec.Spec.Containers[0].Command = baseutils.ConvertMultilineEntrypoint(entrypoint, false).([]string)

	return nil
}
