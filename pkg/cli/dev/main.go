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

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/common/expfmt"

	"github.com/sirupsen/logrus"

	dto "github.com/prometheus/client_model/go"
	"github.com/spf13/cobra"

	k8sUtils "github.com/silogen/kaiwo/pkg/k8s"
	testutils "github.com/silogen/kaiwo/pkg/utils/test"
)

var (
	debugChainsawNamespace  string
	debugChainsawPrintLevel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kaiwo-dev",
		Short: "Use Kaiwo developer features",
	}

	debugCmd := &cobra.Command{
		Use: "debug",
	}
	rootCmd.AddCommand(debugCmd)

	debugChainsawCmd := &cobra.Command{
		Use: "chainsaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := k8sUtils.GetKubernetesClients()
			if err != nil {
				return fmt.Errorf("failed to get k8s clients: %w", err)
			}

			if err := testutils.DebugTest(context.Background(), clients.Clientset, debugChainsawNamespace, debugChainsawPrintLevel); err != nil {
				return err
			}

			return nil
		},
	}
	debugCmd.AddCommand(&cobra.Command{
		Use: "local",
		Run: localTest,
	})
	debugCmd.AddCommand(debugChainsawCmd)
	debugChainsawCmd.Flags().StringVarP(&debugChainsawNamespace, "namespace", "n", "", "Test namespace to inspect")
	debugChainsawCmd.Flags().StringVarP(&debugChainsawPrintLevel, "print-level", "", "info", "The log level to print")

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func localTest(cmd *cobra.Command, args []string) {
	m, err := scrapeMetrics("http://localhost:5555/metrics")
	fmt.Println("Here")
	if err != nil {
		logrus.Fatal(err)
		return
	}

	labelLookup := make(map[string]string)

	gpuGfxActivity := m["gpu_gfx_activity"]
	for _, m := range gpuGfxActivity.Metric {
		for _, label := range m.Label {
			labelLookup[*label.Name] = *label.Value
		}
		namespace := labelLookup["namespace"]
		pod := labelLookup["pod"]
		container := labelLookup["container"]
		gpuId := labelLookup["gpu_id"]
		gpuPartitionId := labelLookup["gpu_partition_id"]
		fmt.Println(fmt.Sprintf("%s/%s %s: %f (gpu ID: %s, partition ID: %s)", namespace, pod, container, *m.Gauge.Value, gpuId, gpuPartitionId))
	}
}

// scrapeMetrics scrapes the given URL, parses all families,
// and returns a map from metric name → MetricFamily.
func scrapeMetrics(url string) (map[string]*dto.MetricFamily, error) {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Prometheus text or OpenMetrics parsing
	parser := expfmt.TextParser{}
	mf, err := parser.TextToMetricFamilies(resp.Body)
	return mf, err
	//if err == nil {
	//	return mf, nil
	//}
	//// Fallback to streaming decode if the above fails
	//resp.Body.Seek(0, io.SeekStart)
	//decoder := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
	//mfs := make(map[string]*dto.MetricFamily)
	//for {
	//	var fam dto.MetricFamily
	//	if err := decoder.Decode(&fam); err == io.EOF {
	//		break
	//	} else if err != nil {
	//		return nil, fmt.Errorf("decode error: %w", err)
	//	}
	//	mfs[fam.GetName()] = &fam
	//}
	//return mfs, nil
}
