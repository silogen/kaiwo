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

package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

func RunLabeledJobDeletionTest() {
	It("create a job with kaiwo label and verify deletion of kaiwojob via kubectl delete job", func() {
		testCRName := "job-with-label-to-delete"

		By("applying a basic job with kaiwo label")
		cmd := exec.Command("kubectl", "apply", "-f", "test/test-manifests/job-with-label-to-delete.yaml", "-n", test_namespace)
		utils.Run(cmd) // nolint:errcheck

		By("waiting for the Custom Resource to be reconciled")
		verifyCustomResource := func(g Gomega) {
			cmd = exec.Command("kubectl", "get", "kaiwojob", testCRName, "-n", test_namespace, "-o", "jsonpath={.status.Status}")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve kaiwojob status")
		}
		Eventually(verifyCustomResource, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("deleting the job and ensuring KaiwoJob is deleted")
		cmd = exec.Command("kubectl", "delete", "job", testCRName, "-n", test_namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to delete test Job")

		verifyKaiwoJobDeletion := func(g Gomega) {
			cmd = exec.Command("kubectl", "get", "kaiwojob", testCRName, "-n", test_namespace)
			_, err := utils.Run(cmd)
			g.Expect(err).To(HaveOccurred(), "KaiwoJob still exists after Job deletion")
		}
		Eventually(verifyKaiwoJobDeletion, 2*time.Minute, 10*time.Second).Should(Succeed())
	})
}
