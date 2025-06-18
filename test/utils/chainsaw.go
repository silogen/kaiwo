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

type ChainsawConfig struct {
	// ConfigPath, if set, is the Chainsaw Config file which is passed to Chainsaw when running tests
	ConfigPath string `json:"configPath"`

	// Tests is a list of paths with Chainsaw tests that should be run
	Tests []string `json:"tests"`

	// Values is the list of values to pass to Chainsaw for the tests
	Values []ChainsawValue `json:"values"`
}

func (c *ChainsawConfig) Run(additionalEnv string) error {
	//tempDir, err := os.MkdirTemp("", "kaiwo-test-")
	//if err != nil {
	//	panic(err)
	//}
	//
	//defer func(path string) {
	//	err := os.RemoveAll(path)
	//	if err != nil {
	//		panic(err)
	//	}
	//}(tempDir)
	//
	//fmt.Println("Created temp directory for chainsaw tests:", tempDir)
	//
	//if err := c.generate(tempDir); err != nil {
	//	return fmt.Errorf("generating chainsaw tests: %w", err)
	//}

	moduleRootPath := GetModuleRoot()

	args := []string{
		"test",
	}

	if c.ConfigPath != "" {
		args = append(args, "--config", c.ConfigPath)
	}

	if len(c.Values) > 0 {

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

//// generate generates the chainsaw tests to run
//func (c *ChainsawConfig) generate(path string) error {
//	if len(c.Tests) == 0 {
//		return fmt.Errorf("no tests specified")
//	}
//	for _, testPath := range c.Tests {
//		srcPath := filepath.Join(GetModuleRoot(), testPath)
//		destPath := filepath.Join(path, testPath)
//		if err := ProcessDirectory(srcPath, destPath); err != nil {
//			return fmt.Errorf("processing tests for path %s: %w", testPath, err)
//		}
//	}
//	return nil
//}

func (c *ChainsawConfig) createValuesFile(path string) error {
	if len(c.Values) > 0 {
		chainsawValues := map[string]string{}

		for _, chainsawValue := range c.Values {
			value, err := chainsawValue.GetValue()
			if err != nil {
				panic(err)
			}
			chainsawValues[chainsawValue.Name] = value
		}

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

//// ProcessDirectory walks srcDir recursively, recreates the directory tree under dstDir,
//// copies every file, and for any file named "chainsaw-test.yaml" applies a transformation.
//func ProcessDirectory(srcDir, dstDir string) error {
//	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
//		if err != nil {
//			return err
//		}
//		rel, err := filepath.Rel(srcDir, path)
//		if err != nil {
//			return err
//		}
//		destPath := filepath.Join(dstDir, rel)
//		if d.IsDir() {
//			// Create the directory in destination
//			return os.MkdirAll(destPath, 0o755)
//		}
//		// It's a file
//		if d.Name() == "chainsaw-test.yaml" {
//			return processChainsawTestYAML(path, destPath)
//		}
//		return copyFile(path, destPath)
//	})
//}
//
//// copyFile copies a file from src to dst, preserving permissions.
//func copyFile(src, dst string) error {
//	in, err := os.Open(src)
//	if err != nil {
//		return err
//	}
//	defer in.Close()
//	info, err := in.Stat()
//	if err != nil {
//		return err
//	}
//	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
//	if err != nil {
//		return err
//	}
//	defer out.Close()
//	_, err = io.Copy(out, in)
//	return err
//}
//
//// processChainsawTestYAML reads the source YAML, injects catch blocks, and writes to dst.
//func processChainsawTestYAML(src, dst string) error {
//	data, err := os.ReadFile(src)
//	if err != nil {
//		return err
//	}
//	var doc yaml.Node
//	if err := yaml.Unmarshal(data, &doc); err != nil {
//		return fmt.Errorf("parsing YAML: %w", err)
//	}
//	if err := injectCatchBlocks(&doc); err != nil {
//		return fmt.Errorf("modifying YAML AST: %w", err)
//	}
//	out, err := os.Create(dst)
//	if err != nil {
//		return err
//	}
//	defer out.Close()
//	encoder := yaml.NewEncoder(out)
//	encoder.SetIndent(2)
//	if err := encoder.Encode(&doc); err != nil {
//		return fmt.Errorf("encoding YAML: %w", err)
//	}
//	return nil
//}
//
//// injectCatchBlocks locates .spec.steps and for each step mapping containing a "try" key,
//// appends a "catch" entry immediately after it.
//func injectCatchBlocks(doc *yaml.Node) error {
//	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
//		return fmt.Errorf("unexpected YAML structure")
//	}
//	root := doc.Content[0]
//	// Find "spec" mapping
//	for i := 0; i < len(root.Content); i += 2 {
//		if root.Content[i].Value == "spec" {
//			spec := root.Content[i+1]
//			// Find "steps" under spec
//			for j := 0; j < len(spec.Content); j += 2 {
//				if spec.Content[j].Value == "steps" && spec.Content[j+1].Kind == yaml.SequenceNode {
//					steps := spec.Content[j+1]
//					for _, item := range steps.Content {
//						if item.Kind == yaml.MappingNode {
//							addCatchToStep(item)
//						}
//					}
//				}
//			}
//		}
//	}
//	return nil
//}
//
//// addCatchToStep checks for a "try" key in the mapping node and, if found, inserts a catch block after it.
//func addCatchToStep(mapping *yaml.Node) {
//	const catchYAML = `
//catch:
//  - command:
//      entrypoint: kaiwo-dev
//      env:
//        - name: NAMESPACE
//          value: ($namespace)
//        - name: PRINT_LEVEL
//          value: ($values.print_level)
//      args: ["debug","chainsaw","--namespace=$NAMESPACE","--print-level=$PRINT_LEVEL"]
//`
//	var snippet yaml.Node
//	if err := yaml.Unmarshal([]byte(catchYAML), &snippet); err != nil {
//		// Should never happen
//		return
//	}
//	// snippet is a document node whose Content[0] is a mapping node with one key "catch"
//	catchMap := snippet.Content[0]
//	// Locate "try" in mapping.Content
//	for idx := 0; idx < len(mapping.Content); idx += 2 {
//		if mapping.Content[idx].Value == "try" {
//			// Insert catchMap.Content (which is [keyNode, valueNode]) after idx+1
//			// build new slice
//			before := mapping.Content[:idx+2]
//			after := mapping.Content[idx+2:]
//			mapping.Content = append(before, append(catchMap.Content, after...)...)
//			// Only one catch per try, so break
//			break
//		}
//	}
//}
