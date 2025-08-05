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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/silogen/kaiwo/test/utils"
)

// Basic AMD GPU tests
var _ = Describe("AMD GPU tests", Label("gpu", "amd"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	BeforeAll(func() {
		if err := testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues); err != nil {
			// pass, errors are handled separately
		}
	})
	testHelper.Register("test/chainsaw/tests/amd-gpu/general", "amd-gpu-tests")
})

// AMD GPU Partitioning tests with setup/teardown
var _ = Describe("AMD GPU Partitioning tests", Label("gpu", "amd", "partitioning"), Ordered, ContinueOnFailure, func() {
	setupHelper := utils.NewTestRunnerHelper()
	testHelper := utils.NewTestRunnerHelper()
	teardownHelper := utils.NewTestRunnerHelper()

	BeforeAll(func() {
		setupErr := setupHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
		Expect(setupErr).NotTo(HaveOccurred())
		if err := testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues); err != nil {
			// pass, errors are handled separately
		}
	})

	setupHelper.Register("test/chainsaw/tests/amd-gpu/partitioning-setup/partition-to-cpx", "amd-gpu-partitioning-setup")
	testHelper.Register("test/chainsaw/tests/amd-gpu/partitioning", "amd-gpu-partitioning-tests")
	teardownHelper.Register("test/chainsaw/tests/amd-gpu/partitioning-setup/partition-to-spx", "amd-gpu-partitioning-teardown")

	AfterAll(func() {
		teardownErr := teardownHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
		Expect(teardownErr).NotTo(HaveOccurred())
	})
})

// MI300X-specific environment tests
var _ = Describe("MI300X Development Environment tests", Label("gpu", "mi300x", "amd"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	Context("MI300X dev environment tests", func() {
		It("should run MI300X dev environment chainsaw tests", func() {
			Expect(testHelper.RunTestBlock("mi300x-dev-tests", "should run MI300X dev environment chainsaw tests", []string{
				"test/chainsaw/tests/environments/mi300x-dev",
			}, chainsawConfigPath, chainsawValues)).To(Succeed())
		})
	})

	AfterAll(func() {
		testHelper.DisplaySummary()
	})
})
