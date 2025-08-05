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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

// LokiQueryResponse represents the response from Loki API
type LokiQueryResponse struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// queryLokiViaProxy queries Loki through Kubernetes service proxy
func (r *ChainsawTestRunner) queryLokiViaProxy(query string, startTime, endTime time.Time) (*LokiQueryResponse, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build Loki query URL
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", fmt.Sprintf("%d", startTime.UnixNano()))
	params.Set("end", fmt.Sprintf("%d", endTime.UnixNano()))
	params.Set("limit", "1000")

	// Use kubectl proxy to access the service
	proxyURL := r.kubernetesClient.CoreV1().RESTClient().Get().
		Namespace(r.LokiNamespace).
		Resource("services").
		Name(fmt.Sprintf("%s:%d", r.LokiServiceName, r.LokiServicePort)).
		SubResource("proxy").
		Suffix("loki/api/v1/query_range").
		URL()

	// Add query parameters
	proxyURL.RawQuery = params.Encode()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", proxyURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Use the same transport as the Kubernetes client
	transport, err := rest.TransportFor(r.kubernetesConfig)
	if err != nil {
		return nil, fmt.Errorf("creating transport: %w", err)
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("loki returned status: %d", resp.StatusCode)
	}

	var lokiResp LokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, fmt.Errorf("parsing Loki response: %w", err)
	}

	return &lokiResp, nil
}

// parseLogLine parses a log line and extracts timestamp, level, and message
func (r *ChainsawTestRunner) parseLogLine(line, source, container, pod string) LogEntry {
	entry := LogEntry{
		Source:    source,
		Container: container,
		Pod:       pod,
		Raw:       line,
		Level:     "INFO", // default
	}

	// 1) If there's a JSON object anywhere, try to unmarshal it.
	if idx := strings.Index(line, "{"); idx != -1 {
		jsonPart := line[idx:]
		var payload struct {
			Ts    string `json:"ts"`
			Msg   string `json:"msg"`
			Level string `json:"level"`
		}
		if err := json.Unmarshal([]byte(jsonPart), &payload); err == nil {
			// parsed JSON ok → trust ts, msg, level from payload
			if ts, err := time.Parse(time.RFC3339Nano, payload.Ts); err == nil {
				entry.Timestamp = ts
			}
			entry.Message = payload.Msg
			entry.Level = strings.ToUpper(payload.Level)
			switch entry.Level {
			case "LEVEL(-2)":
				entry.Level = "TRACE"
			}

			return entry
		}
		// if JSON unmarshal fails, fall through to kubectl logic
	}

	// 2) Fallback: parse a leading RFC3339 timestamp (kubectl --timestamps)
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 2 {
		if ts, err := time.Parse(time.RFC3339, parts[0]); err == nil {
			entry.Timestamp = ts
			entry.Message = parts[1]
		} else {
			entry.Message = line
		}
	} else {
		entry.Message = line
	}

	// 3) Level‐sniffing on the final message
	upper := strings.ToUpper(entry.Message)
	switch {
	case strings.Contains(upper, "ERROR"), strings.Contains(upper, "FATAL"):
		entry.Level = "ERROR"
	case strings.Contains(upper, "WARN"), strings.Contains(upper, "WARNING"):
		entry.Level = "WARN"
	case strings.Contains(upper, "DEBUG"):
		entry.Level = "DEBUG"
	case strings.Contains(upper, "INFO"):
		entry.Level = "INFO"
	}

	return entry
}
