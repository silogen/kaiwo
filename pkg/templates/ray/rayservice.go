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

//go:embed rayservice.yaml.tmpl
var RayServiceTemplate []byte

const SERVECONFIG_FILENAME = "serveconfig"

type RayServiceLoader struct {
	Serveconfig string
}

func (r *RayServiceLoader) Load(path string) error {

	contents, err := os.ReadFile(filepath.Join(path, SERVECONFIG_FILENAME))

	if err != nil {
		return fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	r.Serveconfig = string(contents)

	return nil
}

func (r *RayServiceLoader) DefaultTemplate() []byte {
	return RayServiceTemplate
}

func (r *RayServiceLoader) IgnoreFiles() []string {
	return []string{SERVECONFIG_FILENAME}
}
