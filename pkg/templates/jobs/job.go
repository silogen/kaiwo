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

package jobs

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silogen/ai-workload-orchestrator/pkg/utils"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const ENTRYPOINT_FILENAME = "entrypoint"

type JobLoader struct {
	Entrypoint string
}

func (r *JobLoader) Load(path string) error {

	contents, err := os.ReadFile(filepath.Join(path, ENTRYPOINT_FILENAME))

	if contents == nil {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = strings.ReplaceAll(string(contents), "\n", " ") // Flatten multiline string
	r.Entrypoint = strings.ReplaceAll(r.Entrypoint, "\"", "\\\"") // Escape double quotes
	r.Entrypoint = fmt.Sprintf("\"%s\"", r.Entrypoint) // Wrap the entire command in quotes



	return nil
}

func (r *JobLoader) DefaultTemplate() []byte {
	return JobTemplate
}

func (r *JobLoader) IgnoreFiles() []string {
	return []string{ENTRYPOINT_FILENAME, utils.KAIWOCONFIG_FILENAME}
}
