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
)

// WorkloadArgs is a struct that holds the high-level arguments used for all workloads
type WorkloadArgs struct {
	Path         string
	Image        string
	Name         string
	Namespace    string
	Type         string
	GPUs         int
	TemplatePath string
	DryRun       bool
}

func ValidateWorkloadArgs(args WorkloadArgs) error {
	if args.Path == "" || args.GPUs <= 0 {
		return fmt.Errorf("invalid flags: ensure --path, --type, and --gpus are provided")
	}
	return nil
}

type WorkloadLoader interface {
	// Loads a workload from a path
	Load(path string) error

	// Returns the default teplate for the workloader
	DefaultTemplate() []byte

	// Lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string
}
