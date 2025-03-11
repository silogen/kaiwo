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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/silogen/kaiwo/test/utils"
)

// namespaces where the project is deployed in
const (
	namespace      = "kaiwo-system"
	test_namespace = "kaiwo-test"
)

var runningInCI = func() bool {
	return os.Getenv("CI") != ""
}()

func getChainsawArgs() ([]string, error) {
	args := []string{
		"test",
		"--test-dir",
	}
	chainsawDir := filepath.Join("../", "chainsaw")
	absoluteChainsawDir, err := filepath.Abs(chainsawDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute chainsaw dir: %w", err)
	}
	chainsawDir = absoluteChainsawDir

	if runningInCI {
		args = append(args, filepath.Join(chainsawDir, "tests/standard"), "--config", filepath.Join(chainsawDir, "configs/ci.yaml"))
	} else {
		hfToken := os.Getenv("HF_TOKEN")
		if hfToken == "" {
			return nil, fmt.Errorf("cannot run tests without the environmental variable HF_TOKEN set")
		}
		values := map[string]string{
			"hf_token_base64": base64.StdEncoding.EncodeToString([]byte(hfToken)),
		}
		yamlFile, err := yaml.Marshal(&values)
		if err != nil {
			return nil, fmt.Errorf("failed to generate yaml file: %w", err)
		}
		valuesFile := filepath.Join(chainsawDir, "values/sensitive/values.yaml")

		f, err := os.Create(valuesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create values file: %w", err)
		}
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {
				panic(err)
			}
		}(f)

		_, err = f.Write(yamlFile)
		if err != nil {
			return nil, fmt.Errorf("failed to write values file: %w", err)
		}
		args = append(
			args,
			filepath.Join(chainsawDir, "tests"),
			"--config",
			filepath.Join(chainsawDir, "configs/local.yaml"),
			"--values",
			valuesFile,
		)
	}
	return args, nil
}

func validateChainsawErr(err error) error {
	if err != nil {
		panic(err)
	}
	return err
}

var (
	chainsawArgs, chainsawErr = getChainsawArgs()
	_                         = validateChainsawErr(chainsawErr)
)

// serviceAccountName created for the project
const serviceAccountName = "kaiwo-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "kaiwo-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "kaiwo-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		RecreateNamespace(namespace)
		RecreateNamespace(test_namespace)

		By("labeling the namespace to enforce the restricted security policy")
		cmd := exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("Generating boilerplate code for CRDs")
		cmd = exec.Command("make", "generate")
		cmd.Stdout = nil
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to generate boilerplate code for CRDs")

		By("Generating manifests for CRDs")
		cmd = exec.Command("make", "manifests")
		cmd.Stdout = nil
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to make manifests for CRDs")

		By("Building install.yaml for kaiwo-operator")
		cmd = exec.Command("make", "build-installer", fmt.Sprintf("TAG=%s", strings.Split(projectImage, ":")[1]))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to build install.yaml for kaiwo-operator")

		By("Making a copy of install.yaml so that kustomize can do test patching")
		cmd = exec.Command("cp", "dist/install.yaml", "test/merged.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to make a copy of install.yaml")

		By("deploying kaiwo-operator")
		cmd = exec.Command("kubectl", "apply", "--server-side", "-k", "test/")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy kaiwo-operator")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		runCmd := func(description string, args ...string) {
			By(description)
			cmd := exec.Command("kubectl", args...)
			_, _ = utils.Run(cmd)
		}

		By("Removing finalizers from KaiwoJobs before deletion")
		cmd := exec.Command("kubectl", "get", "kaiwojob", "-A", "-o", "json")
		output, err := utils.Run(cmd)
		if err == nil {
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(strings.ReplaceAll(output, `"finalizers":["kaiwo.silogen.ai/finalizer"]`, `"finalizers":[]`))
			_, _ = utils.Run(cmd)
		}

		go runCmd("Deleting curl pod for metrics", "delete", "pod", "curl-metrics", "-n", namespace, "--force", "--grace-period=0", "--ignore-not-found")
		go runCmd("Deleting ClusterRoleBinding for metrics", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found")
		go runCmd("Deleting all KaiwoJobs", "delete", "kaiwojob", "-A", "--all", "--timeout=30s", "--force", "--grace-period=0")
		go runCmd("Undeploying kaiwo-controller-manager", "delete", "-k", "test", "--force", "--grace-period=0")
		go runCmd("Removing namespace for test workloads", "delete", "ns", test_namespace, "--force", "--grace-period=0", "--timeout=30s")

		time.Sleep(5 * time.Second) // Small delay to start background deletions
		By("Cleanup process started. Waiting for final cleanup.")
		time.Sleep(30 * time.Second) // Ensure background jobs finish
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the kaiwo-controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the kaiwo-controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=kaiwo-controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve kaiwo-controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("kaiwo-controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect kaiwo-controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=kaiwo-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			// By("validating that the ServiceMonitor for Prometheus is applied in the namespace")
			// cmd = exec.Command("kubectl", "get", "ServiceMonitor", "-n", namespace)
			// _, err = utils.Run(cmd)
			// Expect(err).NotTo(HaveOccurred(), "ServiceMonitor should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=alpine/curl:8.12.1",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "alpine/curl:8.12.1",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"kaiwo-job-mutating",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should run all the chainsaw tests", func() {
			By("executing chainsaw tests")

			cmd := exec.Command("chainsaw", chainsawArgs...)
			var outb, errb bytes.Buffer
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			fmt.Println("Chainsaw stdout:\n\n" + outb.String())
			fmt.Println("Chainsaw stderr:\n\n" + errb.String())
			Expect(err).NotTo(HaveOccurred(), "Chainsaw test(s) failed")
		})

		// It("create a job with kaiwo label and verify deletion of kaiwojob via kubectl delete job", func() {
		// 	testCRName := "job-with-label-1"

		// 	By("applying a basic job with kaiwo label")
		// 	cmd := exec.Command("kubectl", "apply", "-f", "test/test-manifests/job-with-label-1.yaml", "-n", test_namespace)
		// 	utils.Run(cmd) // nolint:errcheck

		// 	By("waiting for the Custom Resource to be reconciled")
		// 	verifyCustomResource := func(g Gomega) {
		// 		cmd = exec.Command("kubectl", "get", "kaiwojob", testCRName, "-n", test_namespace, "-o", "jsonpath={.status.Status}")
		// 		_, err := utils.Run(cmd)
		// 		g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve kaiwojob status")
		// 	}
		// 	Eventually(verifyCustomResource, 2*time.Minute, 10*time.Second).Should(Succeed())

		// 	By("deleting the job and ensuring KaiwoJob is deleted")
		// 	cmd = exec.Command("kubectl", "delete", "job", testCRName, "-n", test_namespace)
		// 	_, err := utils.Run(cmd)
		// 	Expect(err).NotTo(HaveOccurred(), "Failed to delete test Job")

		// 	verifyKaiwoJobDeletion := func(g Gomega) {
		// 		cmd = exec.Command("kubectl", "get", "kaiwojob", testCRName, "-n", test_namespace)
		// 		_, err := utils.Run(cmd)
		// 		g.Expect(err).To(HaveOccurred(), "KaiwoJob still exists after Job deletion")
		// 	}
		// 	Eventually(verifyKaiwoJobDeletion, 2*time.Minute, 10*time.Second).Should(Succeed())
		// })

		// It("should create a Job with labels and see the KaiwoJob is created with the same labels", func() {
		// 	testJobName := "job-with-label-2-label-propagation"

		// 	By("applying the Job that contains the desired labels")

		// 	cmd := exec.Command(
		// 		"kubectl", "apply", "-f",
		// 		"test/test-manifests/job-with-label-2-label-propagation.yaml",
		// 		"-n", test_namespace,
		// 	)
		// 	_, err := utils.Run(cmd)
		// 	Expect(err).NotTo(HaveOccurred(), "Failed to apply the labeled Job")

		// 	By("verifying that the KaiwoJob was created with the same labels")
		// 	verifyKaiwoJobLabels := func(g Gomega) {
		// 		cmd := exec.Command("kubectl", "get", "kaiwojob", testJobName,
		// 			"-n", test_namespace,
		// 			"-o", "jsonpath={.metadata.labels}")
		// 		out, err := utils.Run(cmd)
		// 		g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve KaiwoJob labels")

		// 		g.Expect(out).To(ContainSubstring("kaiwo.silogen.ai/managed"),
		// 			"KaiwoJob is missing expected managed label or value")
		// 		g.Expect(out).To(ContainSubstring("custom.label/baz"),
		// 			"KaiwoJob is missing the custom label or value")
		// 	}

		// 	Eventually(verifyKaiwoJobLabels, 90*time.Second, 5*time.Second).Should(Succeed())
		// })

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput := getMetricsOutput()
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

func RecreateNamespace(namespace string) {
	By(fmt.Sprintf("Checking if namespace '%s' exists", namespace))
	cmd := exec.Command("kubectl", "get", "ns", namespace)
	err := cmd.Run()

	if err == nil {
		By(fmt.Sprintf("Namespace '%s' exists, deleting it...", namespace))
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to delete existing namespace '%s'", namespace))

		By(fmt.Sprintf("Waiting for namespace '%s' to be fully removed", namespace))
		Eventually(func() error {
			cmd = exec.Command("kubectl", "get", "ns", namespace)
			return cmd.Run() // Should return an error when namespace no longer exists
		}, "60s", "5s").Should(HaveOccurred(), "Namespace deletion took too long")
	}

	By(fmt.Sprintf("Creating namespace '%s'", namespace))
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create namespace '%s'", namespace))
}
