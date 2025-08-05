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
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

// namespaces where the project is deployed in
const (
	kaiwoSystemNamespace = "kaiwo-system"
)

// Constants moved to individual test files as needed
const (
	metricsRoleBindingName = "kaiwo-metrics-binding"
)

func RecreateNamespace(namespace string) {
	By(fmt.Sprintf("Checking if namespace '%s' exists", namespace))
	cmd := exec.Command("kubectl", "get", "ns", namespace)
	err := cmd.Run()

	if err == nil {
		By(fmt.Sprintf("Namespace '%s' exists, deleting it...", namespace))
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found")
		_, err = utils.RunWithOutput(cmd)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to delete existing namespace '%s'", namespace))

		By(fmt.Sprintf("Waiting for namespace '%s' to be fully removed", namespace))
		Eventually(func() error {
			cmd = exec.Command("kubectl", "get", "ns", namespace)
			return cmd.Run() // Should return an error when namespace no longer exists
		}, "60s", "5s").Should(HaveOccurred(), "Namespace deletion took too long")
	}

	By(fmt.Sprintf("Creating namespace '%s'", namespace))
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.RunWithOutput(cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create namespace '%s'", namespace))
}
