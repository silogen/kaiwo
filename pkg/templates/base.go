/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package templates

import (
	_ "embed"
	"fmt"

	"github.com/silogen/kaiwo/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1 "k8s.io/api/core/v1"
)

// WorkloadArgs is a struct that holds the high-level arguments used for all workloads

func ValidateWorkloadArgs(args utils.WorkloadArgs) error {
	if args.GPUs <= 0 {
		return fmt.Errorf("invalid flags: --gpus must be greater than 0")
	}

	if args.Image == "" && args.Path == "" {
		return fmt.Errorf("invalid flags: at least one of --image or --path must be provided")
	}

	return nil
}

type WorkloadLoader interface {
	// Load loads a workload from a path
	Load(args utils.WorkloadArgs, envVars []corev1.EnvVar) error

	// DefaultTemplate returns the default template for the workloader
	DefaultTemplate() []byte

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string

	// AdditionalResources adds additional resources needed by the worker
	AdditionalResources(resources *[]*unstructured.Unstructured, args utils.WorkloadArgs) error
}
