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
	"os/exec"

	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

var (
	kaiwoBuildPath = "builds"
	kaiwoCliPath   = kaiwoBuildPath + "/kaiwo"
)

var _ = Describe("CLI-specific tests", Label("cli"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	// CLI build MUST succeed before any tests run - this will stop everything if it fails
	BeforeAll(func() {
		By("building the Kaiwo CLI")
		Expect(exec.Command("go", "build", "-o", kaiwoCliPath, "cmd/cli/main.go").Run()).To(Succeed())

		// Execute Chainsaw tests after successful CLI build
		_ = testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
	})

	// Register individual test checks - uses Expect() pattern so they continue even if one fails
	testHelper.Register("test/chainsaw/tests/cli", "cli-tests")
})
