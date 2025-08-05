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

	"github.com/silogen/kaiwo/test/utils"
)

var _ = Describe("Kind-specific tests", Label("kind"), Ordered, ContinueOnFailure, func() {
	testHelper := utils.NewTestRunnerHelper()

	BeforeAll(func() {
		_ = testHelper.ExecuteChainsawTests(chainsawConfigPath, chainsawValues)
	})
	testHelper.Register("test/chainsaw/tests/environments/kind-mock-nvidia", "kind-tests")

	AfterAll(func() {
		// Display test execution summary
		testHelper.DisplaySummary()
	})
})
