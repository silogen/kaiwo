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

package ray

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed rayjob.yaml.tmpl
var RayJobTemplate []byte

const ENTRYPOINT_FILENAME = "entrypoint"

type RayJobLoader struct {
	Entrypoint string
}

func (r *RayJobLoader) Load(path string) error {

	contents, err := os.ReadFile(filepath.Join(path, ENTRYPOINT_FILENAME))

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = string(contents)

	return nil
}

func (r *RayJobLoader) DefaultTemplate() []byte {
	return RayJobTemplate
}

func (r *RayJobLoader) IgnoreFiles() []string {
	return []string{ENTRYPOINT_FILENAME}
}
