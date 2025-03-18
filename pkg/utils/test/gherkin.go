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

package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"github.com/imdario/mergo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GherkinFeature represents a full Gherkin feature.
type GherkinFeature struct {
	ScenarioWrapper GherkinScenarioWrapper `yaml:",inline"`
	Tags            []string               `yaml:"tags,omitempty"`
	Reference       *ResourceRef           `yaml:"reference,omitempty"`
	Rules           []Rule                 `yaml:"rules,omitempty"`
}

func (f *GherkinFeature) CreateChainsawTests(folder string, inputFolder string) error {
	reference, err := f.Reference.Contents(inputFolder)
	if err != nil {
		return fmt.Errorf("failed to read gherkin reference contents: %w", err)
	}
	sanitizedName := baseutils.MakeRFC1123Compliant(f.ScenarioWrapper.Name)
	featureFolder := filepath.Join(folder, sanitizedName)

	if err := f.ScenarioWrapper.Build(featureFolder, reference, sanitizedName); err != nil {
		return fmt.Errorf("failed to build gherkin feature: %w", err)
	}

	for _, rule := range f.Rules {
		sanitizedRuleName := baseutils.MakeRFC1123Compliant(rule.ScenarioWrapper.Name)
		ruleFolder := filepath.Join(featureFolder, "rules", sanitizedRuleName)
		if err := rule.ScenarioWrapper.Build(ruleFolder, reference, fmt.Sprintf("%s--%s", sanitizedName, sanitizedRuleName)); err != nil {
			return fmt.Errorf("failed to build gherkin feature rule: %w", err)
		}
	}

	return nil
}

type ResourceRef struct {
	File     string         `yaml:"file,omitempty"`
	Resource map[string]any `yaml:"resource,omitempty"`
}

func (r *ResourceRef) Contents(rootDir string) (*unstructured.Unstructured, error) {
	var reference *unstructured.Unstructured

	if r != nil {
		if r.File != "" {
			contents, err := os.ReadFile(filepath.Join(rootDir, r.File))
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", r.File, err)
			}
			var data map[string]interface{}
			if err := yaml.Unmarshal(contents, &data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal file %s: %w", r.File, err)
			}
			reference = &unstructured.Unstructured{Object: data}
		} else if r.Resource != nil {
			reference = &unstructured.Unstructured{Object: r.Resource}
		}
	}
	return reference, nil
}

// Scenario represents a Gherkin scenario, which can modify the reference.
type Scenario struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Steps       []Step   `yaml:"steps"`
}

// Step represents a Gherkin step.
type Step struct {
	// Gherkin fields
	Given string `yaml:"given,omitempty"`
	When  string `yaml:"when,omitempty"`
	Then  string `yaml:"then,omitempty"`
	And   string `yaml:"and,omitempty"`
	But   string `yaml:"but,omitempty"`

	Chainsaw ChainsawStep `yaml:",inline"`

	// ApplyPatch includes the patch to apply on top of the current base reference
	ApplyPatch *map[string]any `yaml:"applyPatch,omitempty"`

	AssertPatch *map[string]any `yaml:"assertPatch,omitempty"`
}

type ChainsawStep struct {
	Apply    *map[string]interface{} `yaml:"apply,omitempty"`
	Assert   *map[string]interface{} `yaml:"assert,omitempty"`
	Command  *map[string]interface{} `yaml:"command,omitempty"`
	Create   *map[string]interface{} `yaml:"create,omitempty"`
	Delete   *map[string]interface{} `yaml:"delete,omitempty"`
	Describe *map[string]interface{} `yaml:"describe,omitempty"`
	Error    *map[string]interface{} `yaml:"error,omitempty"`
	Events   *map[string]interface{} `yaml:"events,omitempty"`
	Get      *map[string]interface{} `yaml:"get,omitempty"`
	Patch    *map[string]interface{} `yaml:"patch,omitempty"`
	PodLogs  *map[string]interface{} `yaml:"podLogs,omitempty"`
	Proxy    *map[string]interface{} `yaml:"proxy,omitempty"`
	Script   *map[string]interface{} `yaml:"script,omitempty"`
	Sleep    *map[string]interface{} `yaml:"sleep,omitempty"`
	Update   *map[string]interface{} `yaml:"update,omitempty"`
	Wait     *map[string]interface{} `yaml:"wait,omitempty"`
}

func (s *Step) IsBuildStep() bool {
	return s.Given != "" || s.When != ""
}

func (s *Step) IsAssertStep() bool {
	return !s.IsBuildStep()
}

func (s *Step) GetName() (string, error) {
	name := ""
	prefix := ""
	if s.Given != "" {
		name = s.Given
		prefix = "given"
	} else if s.When != "" {
		name = s.When
		prefix = "when"
	} else if s.Then != "" {
		name = s.Then
		prefix = "then"
	} else if s.And != "" {
		name = s.And
		prefix = "and"
	} else if s.But != "" {
		name = s.But
		prefix = "but"
	} else {
		return "", fmt.Errorf("no name for step, must give one of given, when, then, and or but")
	}

	return fmt.Sprintf("%s: %s", prefix, name), nil
}

//type ScenarioOutline struct {
//	Tags     []string                `yaml:"tags,omitempty"`
//	Steps    []Step                  `yaml:"steps"`
//	Examples ScenarioOutlineExamples `yaml:"examples"`
//}
//
//type ScenarioOutlineExamples struct {
//	// Fixture allows inline defining of the test fixtures for this scenario outline
//	Fixture Fixture `yaml:"fixture,omitempty"`
//
//	// Parameters lists the fixture parameters that are requested by this scenario outline
//	Parameters []string `yaml:"parameters,omitempty"`
//}

type GherkinScenarioWrapper struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description,omitempty"`
	Background  []Step     `yaml:"background,omitempty"`
	Scenarios   []Scenario `yaml:"scenarios"`
}

func (g GherkinScenarioWrapper) Build(folder string, baseObject *unstructured.Unstructured, nameBase string) error {
	if baseObject != nil {
		baseObject = baseObject.DeepCopy()
	}

	var applyObjectKeywords []string

	backgroundSteps, applyObject, keywords, err := BuildChainsawSteps(g.Background, baseObject, applyObjectKeywords, nameBase)
	if err != nil {
		return fmt.Errorf("failed to build background steps: %w", err)
	}
	applyObjectKeywords = keywords
	baseObject = applyObject

	for _, scenario := range g.Scenarios {
		scenarioNameBase := fmt.Sprintf("%s--%s", nameBase, baseutils.MakeRFC1123Compliant(scenario.Name))
		scenarioSteps, _, _, err := BuildChainsawSteps(scenario.Steps, baseObject, applyObjectKeywords, scenarioNameBase)
		if err != nil {
			return fmt.Errorf("failed to build scenario steps: %w", err)
		}

		steps := append(backgroundSteps, scenarioSteps...)

		chainsawTest := map[string]any{
			"apiVersion": "chainsaw.kyverno.io/v1alpha1",
			"kind":       "Test",
			"metadata": map[string]any{
				"name": scenarioNameBase,
			},
			"spec": map[string]any{
				"steps": []map[string]any{
					{
						"try": steps,
						"catch": []map[string]interface{}{
							{"command": map[string]any{
								"entrypoint": "kaiwo",
								"env": []map[string]string{
									{
										"name":  "NAMESPACE",
										"value": "$(namespace)",
									},
									{
										"name":  "PRINT_LEVEL",
										"value": "$(values.print_level)",
									},
								},
								"args": []string{
									"tests",
									"debug",
									"chainsaw",
									"--namespace=$NAMESPACE",
									"--print-level=$PRINT_LEVEL",
								},
							}},
						},
					},
				},
			},
		}

		scenarioFolder := filepath.Join(folder, baseutils.MakeRFC1123Compliant(scenario.Name))
		if err := os.MkdirAll(scenarioFolder, 0o755); err != nil {
			return fmt.Errorf("Error creating directories: %v\n", err)
		}
		header := "This is an autogenerated test, do not edit"
		if err := saveYAMLToFile(chainsawTest, filepath.Join(scenarioFolder, "chainsaw-test.yaml"), 2, header); err != nil {
			return fmt.Errorf("failed to save chainsaw-test.yaml: %w", err)
		}

	}
	return nil
}

func saveYAMLToFile(data interface{}, filePath string, indent int, header string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	// Write the header if it's not empty
	if header != "" {
		_, err := file.WriteString("# " + header + "\n\n")
		if err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
	}

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(indent) // Set indentation to 2
	defer func(encoder *yaml.Encoder) {
		err := encoder.Close()
		if err != nil {
			panic(err)
		}
	}(encoder)

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}

func BuildChainsawSteps(gherkinSteps []Step, applyObject *unstructured.Unstructured, applyObjectKeywords []string, testName string) ([]any, *unstructured.Unstructured, []string, error) {
	var chainsawSteps []any

	buildPhase := true
	applyObject = applyObject.DeepCopy()

	for i, step := range gherkinSteps {

		description, err := step.GetName()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get name for step: %w", err)
		}
		if step.IsAssertStep() {
			if buildPhase && applyObject != nil {
				// Finished the build phase (given, when) and there is an object that should be applied
				if err := ensureMetadataName(applyObject, testName); err != nil {
					return nil, nil, nil, fmt.Errorf("failed to ensure metadata name for step: %w", err)
				}
				chainsawSteps = append(chainsawSteps, map[string]interface{}{
					"apply": map[string]interface{}{
						"resource": applyObject.Object,
					},
					"description": strings.Join(applyObjectKeywords, "\n"),
				})
			}
			buildPhase = false

			if step.AssertPatch != nil {
				if applyObject == nil {
					return nil, nil, nil, fmt.Errorf("no assertion found for step %d", i)
				}
				apiVersion, _, _ := unstructured.NestedString(applyObject.Object, "apiVersion")
				kind, _, _ := unstructured.NestedString(applyObject.Object, "kind")
				metadata, _, _ := unstructured.NestedMap(applyObject.Object, "metadata")

				assert := &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": apiVersion,
					"kind":       kind,
					"metadata":   metadata,
				}}

				assertPatch := &unstructured.Unstructured{Object: *step.AssertPatch}

				if err := mergo.Merge(assert, assertPatch, mergo.WithOverride); err != nil {
					return nil, nil, nil, fmt.Errorf("failed to merge patch step %d: %w", i, err)
				}

				chainsawStep := map[string]interface{}{
					"assert": map[string]interface{}{
						"resource": assert.Object,
					},
					"description": strings.Join(applyObjectKeywords, "\n"),
				}
				chainsawSteps = append(chainsawSteps, chainsawStep)
				continue
			}

		} else {
			if !buildPhase {
				return nil, nil, nil, fmt.Errorf("cannot include a build step (given, when) after an assert step (then, and, but)")
			}

			name, err := step.GetName()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to get name for step %d: %w", i, err)
			}
			applyObjectKeywords = append(applyObjectKeywords, name)

			if step.ApplyPatch != nil {
				applyPatchReference := &unstructured.Unstructured{Object: *step.ApplyPatch}
				fmt.Println("Adding", applyPatchReference)
				if err := mergo.Merge(applyObject, applyPatchReference, mergo.WithOverride); err != nil {
					return nil, nil, nil, fmt.Errorf("failed to merge patch step %d: %w", i, err)
				}
				fmt.Println("Merged patch reference:", applyObject)
				continue
			}
		}
		chainsawStep := struct {
			ChainsawStep `yaml:",inline"`
			Description  string `yaml:"description,omitempty"`
		}{
			ChainsawStep: step.Chainsaw,
			Description:  description,
		}
		chainsawSteps = append(chainsawSteps, chainsawStep)
	}
	return chainsawSteps, applyObject, applyObjectKeywords, nil
}

func ensureMetadataName(obj *unstructured.Unstructured, defaultName string) error {
	// Try to get the metadata.name field
	name, found, err := unstructured.NestedString(obj.Object, "metadata", "name")
	if err != nil {
		return fmt.Errorf("error retrieving metadata.name: %w", err)
	}

	// If metadata.name is missing or empty, set it to the default value
	if !found || name == "" {
		if err := unstructured.SetNestedField(obj.Object, defaultName, "metadata", "name"); err != nil {
			return fmt.Errorf("error setting metadata.name: %w", err)
		}
		fmt.Printf("Set metadata.name to default: %s\n", defaultName)
	} else {
		fmt.Printf("metadata.name is already set: %s\n", name)
	}

	return nil
}

// Rule represents a Gherkin rule grouping related scenarios with modifications.
type Rule struct {
	ScenarioWrapper GherkinScenarioWrapper `json:",inline" yaml:",inline"`
}

//type Fixture struct {
//	Permutations [][]map[string]any `yaml:"permutations,omitempty"`
//	Combinations []map[string]any   `yaml:"combinations,omitempty"`
//}
//
//// GetAll returns a list of all combinations and permutations defined in the fixture.
//func (f *Fixture) GetAll() []map[string]any {
//	var results []map[string]any
//
//	// Compute permutation combinations if any are defined.
//	if len(f.Permutations) > 0 {
//		perms := computePermutations(f.Permutations)
//		results = append(results, perms...)
//	}
//
//	// Add explicit combinations.
//	if len(f.Combinations) > 0 {
//		results = append(results, f.Combinations...)
//	}
//
//	return results
//}
//
//// computePermutations takes a slice of permutation groups and returns the Cartesian product
//// of all groups as a slice of merged maps.
//func computePermutations(groups [][]map[string]any) []map[string]any {
//	// Start with an initial empty combination.
//	result := []map[string]any{make(map[string]any)}
//
//	// For each group, build new combinations by merging each option with the existing base.
//	for _, group := range groups {
//		var newResult []map[string]any
//		for _, base := range result {
//			for _, option := range group {
//				// Copy the base combination.
//				merged := copyMap(base)
//				// Merge the option into the base. In case of key conflict, option's value overwrites.
//				for k, v := range option {
//					merged[k] = v
//				}
//				newResult = append(newResult, merged)
//			}
//		}
//		result = newResult
//	}
//
//	return result
//}
//
//// copyMap creates a shallow copy of the given map.
//func copyMap(src map[string]any) map[string]any {
//	dst := make(map[string]any)
//	for k, v := range src {
//		dst[k] = v
//	}
//	return dst
//}
