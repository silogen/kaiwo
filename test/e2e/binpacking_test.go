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
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

func RunBinpackingTest() {
	It("should create three kaiwojobs with GPU requests and assert that they're running on the same node", func() {
		testNamespace := "kaiwo-test"
		jobNames := []string{"binpacking-kaiwojob-1", "binpacking-kaiwojob-2", "binpacking-kaiwojob-3"}

		By("Creating three Jobs with GPU requests and the kaiwo-managed label")
		for _, jobName := range jobNames {
			jobManifest := fmt.Sprintf(`
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: %s
  namespace: %s
spec:
  user: test@amd.com
  gpuVendor: "nvidia"
  image: busybox:latest
  entrypoint: "sleep 30"
  resources:
    requests:
      cpu: "1"
      memory: "1Gi"
      nvidia.com/gpu: "1"
    limits:
      cpu: "1"
      memory: "1Gi"
      nvidia.com/gpu: "1"
`, jobName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(jobManifest)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply job %s: %s", jobName, string(output)))
		}

		By("Waiting for the jobs' pods to be scheduled")
		Eventually(func(g Gomega) {
			for _, jobName := range jobNames {
				cmd := exec.Command("kubectl", "get", "pods", "-l", fmt.Sprintf("job-name=%s", jobName), "-n", testNamespace, "-o", "jsonpath={.items[0].status.phase}")
				output, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get pod status for job %s: %s", jobName, string(output)))
				g.Expect(string(output)).To(Equal("Running"), fmt.Sprintf("Pod for job %s is not running", jobName))
			}
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("Ensuring all pods are running on the same node")
		var nodeName string
		Eventually(func(g Gomega) {
			nodeNames := make(map[string]bool)

			for _, jobName := range jobNames {
				cmd := exec.Command("kubectl", "get", "pods", "-l", fmt.Sprintf("job-name=%s", jobName), "-n", testNamespace, "-o", "jsonpath={.items[0].spec.nodeName}")
				output, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get node for job %s: %s", jobName, string(output)))
				podNode := strings.TrimSpace(string(output))
				g.Expect(podNode).NotTo(BeEmpty(), fmt.Sprintf("Pod for job %s is not scheduled on a node", jobName))
				nodeNames[podNode] = true
			}

			g.Expect(nodeNames).To(HaveLen(1), fmt.Sprintf("Jobs are running on multiple nodes: %s", strings.Join(utils.MapKeys(nodeNames), ", ")))
			nodeName = utils.MapKeys(nodeNames)[0]
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By(fmt.Sprintf("All jobs are running on the same node: %s", nodeName))

		By("Deleting the jobs and ensuring they are removed")
		for _, jobName := range jobNames {
			cmd := exec.Command("kubectl", "delete", "job", jobName, "-n", testNamespace)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to delete job %s: %s", jobName, string(output)))
		}
	})
}
