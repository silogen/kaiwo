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

package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PrometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []PrometheusResult `json:"result"`
	} `json:"data"`
}

func QueryPrometheusMetrics(ctx context.Context, endpoint string, query string) (*PrometheusResponse, error) {
	logger := log.FromContext(ctx).WithName("PrometheusQuery")
	baseURL := fmt.Sprintf("%s/api/v1/query", endpoint)
	baseutils.Debug(logger, "Querying Prometheus API")
	prometheusUrl, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	prometheusQuery := prometheusUrl.Query()
	prometheusQuery.Set("query", query)
	prometheusUrl.RawQuery = prometheusQuery.Encode()

	resp, err := http.Get(prometheusUrl.String())
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err, "failed to close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result PrometheusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}
