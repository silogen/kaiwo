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

// Utility functions for working with YAML files

package utils

import (
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

// StringReplaceParams is a struct that holds the path and value to replace in a YAML file
type StringReplaceParams struct {
	Path  string
	Value string
}

// Replace multiple string values in a YAML file
func ReplaceStringValues(content *ast.File, params []StringReplaceParams) error {

	for _, target := range params {
		if err := ReplaceStringValue(content, target); err != nil {
			return err
		}
	}

	return nil
}

// Replace a single string value in a YAML file
func ReplaceStringValue(content *ast.File, params StringReplaceParams) error {

	// Create a path object
	path, err := yaml.PathString(params.Path)
	if err != nil {
		return err
	}

	// Replace the value using ReplaceWithReader
	if err := path.ReplaceWithReader(content, strings.NewReader(params.Value)); err != nil {
		return err
	}

	return nil
}
