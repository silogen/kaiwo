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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

var (
	projectImage = "ghcr.io/silogen/kaiwo-operator:v-e2e"

	// Test execution flags
	deployOperatorToKind = flag.Bool("deploy-to-kind", false, "Builds and deploys the operator to the Kind cluster")
	buildCli             = flag.Bool("build-cli", false, "Build CLI tools")

	// Test selection flags
	validateController = flag.Bool("validate-controller", false, "Run controller validation tests")

	// Global chainsaw configuration - set up in BeforeSuite
	chainsawConfigPath string
	chainsawValues     []utils.ChainsawValue
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting Kaiwo integration test suite\n")
	RunSpecs(t, "e2e suite")
}

func shouldDeployToKind() bool {
	return os.Getenv("KAIWO_TEST_DEPLOY_TO_KIND") == "true"
}

var _ = BeforeSuite(func() {
	// Set up Chainsaw configuration
	By("setting up Chainsaw configuration")
	chainsawConfigPath = getDefaultChainsawConfig()
	if config := os.Getenv("KAIWO_TEST_CHAINSAW_CONFIG"); config != "" {
		chainsawConfigPath = config
	}
	chainsawValues = getDefaultChainsawValues()

	kaiwoConfigPath := os.Getenv("KAIWO_TEST_KAIWOCONFIG")
	if kaiwoConfigPath == "" {
		panic("KAIWO_TEST_KAIWOCONFIG must be set")
	}

	if shouldDeployToKind() {

		SetDefaultEventuallyTimeout(2 * time.Minute)
		SetDefaultEventuallyPollingInterval(time.Second)

		By("building the manager(Operator) image")
		Expect(utils.Run(exec.Command("make", "docker-build", "IMG="+projectImage))).To(Succeed())

		By("creating kaiwo-system namespace")
		RecreateNamespace(kaiwoSystemNamespace)

		By("loading the manager(Operator) image on Kind")
		Expect(utils.LoadImageToKindClusterWithName(projectImage)).To(Succeed())

		By("labeling the namespace to enforce the restricted security policy")
		Expect(utils.Run(exec.Command("kubectl", "label", "--overwrite", "ns", kaiwoSystemNamespace,
			"pod-security.kubernetes.io/enforce=restricted"))).To(Succeed())

		By("Generating boilerplate code for CRDs")
		cmd := exec.Command("make", "generate")
		cmd.Stdout = nil
		Expect(utils.Run(cmd)).To(Succeed())

		By("Generating manifests for CRDs")
		cmd = exec.Command("make", "manifests")
		cmd.Stdout = nil
		Expect(utils.Run(cmd)).To(Succeed())

		By("Building install.yaml for kaiwo-operator")
		Expect(utils.Run(exec.Command("make", "build-installer", fmt.Sprintf("TAG=%s", "v-e2e")))).To(Succeed())

		By("Making a copy of install.yaml so that kustomize can do test patching")
		Expect(utils.Run(exec.Command("cp", "dist/install.yaml", "test/merged.yaml"))).To(Succeed())

		By("deploying kaiwo-operator")
		Expect(utils.Run(exec.Command("kubectl", "apply", "--server-side", "-k", "test/"))).To(Succeed())

		By("validating that the kaiwo-controller-manager is running as expected")
		Expect(utils.Run(exec.Command("chainsaw", "test", "test/chainsaw/tests/global/controller"))).To(Succeed())
	}

	By("applying the KaiwoConfig manifest")
	Expect(utils.Run(exec.Command("kubectl", "apply", "-f", kaiwoConfigPath))).To(Succeed())
})

//It("should apply the correct KaiwoConfig profile", func() {
// determine the profile (kind / gpu), and kubectl apply it from /test/configs/kaiwoconfig
//})
//It("should setup the Chainsaw values file", func() {
// Github token
// HuggingFace token
//})

var _ = AfterSuite(func() {
	By("Cleaning up any remaining chainsaw test namespaces")
	// kubectl get ns -o name | grep "chainsaw-" | xargs -r kubectl delete --ignore-not-found

	chainsawCmd := exec.Command("kubectl", "get", "namespaces", "-o", "name", "--no-headers")
	chainsawOutput, err := utils.RunWithOutput(chainsawCmd)
	if err == nil {
		namespaces := utils.GetNonEmptyLines(chainsawOutput)
		for _, ns := range namespaces {
			nsName := strings.TrimPrefix(ns, "namespace/")
			if strings.HasPrefix(nsName, "chainsaw-") {
				Expect(utils.Run(exec.Command("kubectl", "delete", "namespace", nsName))).To(Succeed())
			}
		}
	}

	if !shouldDeployToKind() {
		return
	}

	runCmd := func(description string, args ...string) {
		By(description)
		cmd := exec.Command("kubectl", args...)
		_, _ = utils.RunWithOutput(cmd)
	}

	By("Removing finalizers from KaiwoJobs before deletion")
	cmd := exec.Command("kubectl", "get", "kaiwojob", "-A", "-o", "json")
	output, err := utils.RunWithOutput(cmd)
	if err == nil {
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(strings.ReplaceAll(output, `"finalizers":["kaiwo.silogen.ai/finalizer"]`, `"finalizers":[]`))
		_, _ = utils.RunWithOutput(cmd)
	}

	go runCmd("Deleting curl pod for metrics", "delete", "pod", "curl-metrics", "-n", kaiwoSystemNamespace, "--force", "--grace-period=0", "--ignore-not-found")
	go runCmd("Deleting ClusterRoleBinding for metrics", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found")
	go runCmd("Deleting all KaiwoJobs", "delete", "kaiwojob", "-A", "--all", "--timeout=30s", "--force", "--grace-period=0")
	go runCmd("Undeploying kaiwo-controller-manager", "delete", "-k", "test", "--force", "--grace-period=0")

	time.Sleep(5 * time.Second) // Small delay to start background deletions
	By("Cleanup process started. Waiting for final cleanup.")
	time.Sleep(30 * time.Second) // Ensure background jobs finish
})

// getDefaultChainsawConfig returns the Chainsaw config path
func getDefaultChainsawConfig() string {
	return "test/chainsaw/configs/ci.yaml"
}

// getDefaultChainsawValues returns the appropriate values for the current build environment
func getDefaultChainsawValues() []utils.ChainsawValue {
	values := []utils.ChainsawValue{}

	// Add tokens if needed (could be extended with build tags for different environments)
	if needsTokens() {
		values = append(values,
			utils.ChainsawValue{Name: "gh_token_base64", ValueFromEnv: "GH_TOKEN", EncodeBase64: true},
			utils.ChainsawValue{Name: "hf_token_base64", ValueFromEnv: "HF_TOKEN", EncodeBase64: true},
		)
	}

	return values
}

// needsTokens returns whether tokens are needed for the current environment
func needsTokens() bool {
	return false // Default case, can be overridden with build tags
}
