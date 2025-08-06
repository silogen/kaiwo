/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

// Basic AMD GPU tests
var _ = Describe("AMD GPU tests", Label("gpu", "amd"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	BeforeAll(func() {
		_ = testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
	})
	testHelper.Register("test/chainsaw/tests/amd-gpu/general", "amd-gpu-tests")
})

// AMD GPU Partitioning tests with setup/teardown
var _ = Describe("AMD GPU Partitioning tests", Label("gpu", "amd", "partitioning"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	BeforeAll(func() {
		By("Running setup: partition-to-cpx")
		setupConfig := &utils.ChainsawExecutionConfig{
			ConfigPath:     chainsawConfigPath,
			Tests:          []string{"test/chainsaw/tests/amd-gpu/partitioning-setup/partition-to-cpx"},
			Values:         chainsawValues,
			BaseValuesFile: os.Getenv("KAIWO_TEST_BASE_VALUES_FILE"),
		}
		setupErr := setupConfig.Run("")
		Expect(setupErr).NotTo(HaveOccurred())

		By("Running main partitioning tests")
		_ = testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
	})

	testHelper.Register("test/chainsaw/tests/amd-gpu/partitioning", "amd-gpu-partitioning-tests")

	AfterAll(func() {
		By("Running teardown: partition-to-spx")
		teardownConfig := &utils.ChainsawExecutionConfig{
			ConfigPath:     chainsawConfigPath,
			Tests:          []string{"test/chainsaw/tests/amd-gpu/partitioning-setup/partition-to-spx"},
			Values:         chainsawValues,
			BaseValuesFile: os.Getenv("KAIWO_TEST_BASE_VALUES_FILE"),
		}
		teardownErr := teardownConfig.Run("")
		Expect(teardownErr).NotTo(HaveOccurred())
	})
})
