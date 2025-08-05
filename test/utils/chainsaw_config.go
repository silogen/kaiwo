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

package utils

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ChainsawExecutionConfig struct {
	// ConfigPath, if set, is the Chainsaw Config file which is passed to Chainsaw when running tests
	ConfigPath string `json:"configPath"`

	// Tests is a list of paths with Chainsaw tests that should be run
	Tests []string `json:"tests"`

	// Values is the list of values to pass to Chainsaw for the tests
	Values []ChainsawValue `json:"values"`

	// BaseValuesFile, if set, is a YAML file that will be merged with generated values
	BaseValuesFile string `json:"baseValuesFile,omitempty"`
}

// ChainsawValue is an entry in the Chainsaw values.yaml file which can be used in the Chainsaw tests
type ChainsawValue struct {
	// Name is the resulting name in the Chainsaw values.yaml file
	Name string `json:"name"`

	// Value is the direct value to use
	Value string `json:"value,omitempty"`

	// ValueFromEnv, if set, will fetch the value from an environmental variable
	ValueFromEnv string `json:"valueFromEnv,omitempty"`

	// EncodeBase64, if true, the value will be base-64 encoded
	EncodeBase64 bool `json:"encodeBase64,omitempty"`
}

func (c *ChainsawExecutionConfig) Run(additionalEnv string) error {
	moduleRootPath := GetModuleRoot()

	args := []string{
		"test",
	}

	if c.ConfigPath != "" {
		args = append(args, "--config", c.ConfigPath)
	}

	if len(c.Values) > 0 || c.BaseValuesFile != "" {

		valuesFile, err := os.CreateTemp("", "kaiwo-test-chainsaw-values-*.yaml")
		if err != nil {
			panic(err)
		}
		defer func() {
			name := valuesFile.Name()
			if err := valuesFile.Close(); err != nil {
				return
			}
			if err := os.Remove(name); err != nil {
				return
			}
		}()

		if err := c.createValuesFile(valuesFile.Name()); err != nil {
			return fmt.Errorf("creating values file: %w", err)
		}
		args = append(args, "--values", valuesFile.Name())
		fmt.Println("Created values file", valuesFile.Name())
	}

	var testPaths []string

	if len(c.Tests) == 0 {
		return fmt.Errorf("no tests specified")
	}

	for _, t := range c.Tests {
		testPaths = append(testPaths, filepath.Join(moduleRootPath, t))
	}

	args = append(args, testPaths...)

	env := os.Environ()

	fmt.Println(args)
	cmd := exec.Command("chainsaw", args...)
	if additionalEnv != "" {
		cmd.Env = append(env, additionalEnv)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *ChainsawExecutionConfig) createValuesFile(path string) error {
	chainsawValues := map[string]interface{}{}

	// Load base values file if specified
	if c.BaseValuesFile != "" {
		baseData, err := os.ReadFile(c.BaseValuesFile)
		if err != nil {
			return fmt.Errorf("reading base values file %s: %w", c.BaseValuesFile, err)
		}

		if err := yaml.Unmarshal(baseData, &chainsawValues); err != nil {
			return fmt.Errorf("parsing base values file %s: %w", c.BaseValuesFile, err)
		}
	}

	// Add/override with generated values
	if len(c.Values) > 0 {
		for _, chainsawValue := range c.Values {
			value, err := chainsawValue.GetValue()
			if err != nil {
				panic(err)
			}
			chainsawValues[chainsawValue.Name] = value
		}
	}

	// Only create file if we have values to write
	if len(chainsawValues) > 0 {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("error opening/creating file: %w", err)
		}
		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				panic(err)
			}
		}(file)

		enc := yaml.NewEncoder(file)

		err = enc.Encode(chainsawValues)
		if err != nil {
			return fmt.Errorf("encoding chainsaw values: %w", err)
		}
	}
	return nil
}

func (v ChainsawValue) GetValue() (string, error) {
	value := ""
	if v.Value != "" {
		value = v.Value
	} else if v.ValueFromEnv != "" {
		exist := false
		value, exist = os.LookupEnv(v.ValueFromEnv)
		if !exist {
			return "", fmt.Errorf("failed to find value for environment variable %s", v.ValueFromEnv)
		}
		value = strings.TrimSpace(value)
	}
	if v.EncodeBase64 {
		value = base64.StdEncoding.EncodeToString([]byte(value))
	}
	return value, nil
}
