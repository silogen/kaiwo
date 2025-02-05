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

package workloads

import (
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

// This defines the extra functions that are usable in kaiwo template files
// in addition to the go template defaults.
func GetTemplateFuncMap() template.FuncMap {
	funcMap := sprig.TxtFuncMap()
	funcMap["toYaml"] = toYaml
	return funcMap
}

func toYaml(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		logrus.Errorf("Failed to marshal object to YAML: %v", err)
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}
