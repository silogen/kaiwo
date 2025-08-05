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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogEntry represents a timestamped log entry with source information
type LogEntry struct {
	Timestamp time.Time
	Source    string // "container", "kaiwo-controller", "kueue-controller", "event"
	Level     string // "INFO", "WARN", "ERROR", "DEBUG"
	Container string // Container name (for container logs)
	Pod       string // Pod name (for container logs)
	Message   string
	Raw       string // Original log line
}

// collectDebugInformationFromLoki gathers all logs and events from Loki for the failed test
func (r *ChainsawTestRunner) collectDebugInformationFromLoki(failure TestFailure) []LogEntry {
	var allEntries []LogEntry

	// Calculate time range around the failure (extended since namespace is deleted)
	startTime := failure.Timestamp.Add(-5 * time.Minute)
	endTime := failure.Timestamp.Add(10 * time.Minute)

	// 1. Get all container logs from the test namespace
	// fmt.Printf("   📦 Fetching container logs from Loki for namespace %s...\n", failure.Namespace)
	containerLogs := r.getLokiLogsWithQuery(fmt.Sprintf(`{namespace="%s"}`, failure.Namespace), startTime, endTime, "container")
	allEntries = append(allEntries, containerLogs...)

	// 2. Get Kaiwo controller logs - check if running locally first
	kaiwoLogs := r.getKaiwoControllerLogs(failure.Namespace, startTime, endTime)
	allEntries = append(allEntries, kaiwoLogs...)

	// 3. Get Kueue controller logs filtered to this namespace
	// fmt.Printf("   📊 Fetching Kueue controller logs from Loki...\n")
	kueueQuery := fmt.Sprintf(`{namespace="kueue-system", pod=~"kueue-controller-manager.*"} |~ "(?i)%s"`, failure.Namespace)
	kueueLogs := r.getLokiLogsWithQuery(kueueQuery, startTime, endTime, "kueue-controller-manager")
	allEntries = append(allEntries, kueueLogs...)

	// fmt.Printf("   📊 Fetching Ray controller logs from Loki...\n")
	rayQuery := fmt.Sprintf(`{namespace="default", pod=~"kuberay-operator.*"} |~ "(?i)%s"`, failure.Namespace)
	rayLogs := r.getLokiLogsWithQuery(rayQuery, startTime, endTime, "kuberay-operator")
	allEntries = append(allEntries, rayLogs...)

	// 4. Get Kubernetes events from the test namespace
	// fmt.Printf("   📅 Fetching Kubernetes events from Loki...\n")
	eventQuery := fmt.Sprintf(`{namespace="%s", container="kube-apiserver"} |~ "(?i)event"`, failure.Namespace)
	eventLogs := r.getLokiLogsWithQuery(eventQuery, startTime, endTime, "event")
	allEntries = append(allEntries, eventLogs...)

	// fmt.Printf("   📊 Collected %d log entries from Loki\n", len(allEntries))
	return allEntries
}

// displayAggregatedLogs shows all collected logs in chronological order with color coding
func (r *ChainsawTestRunner) displayAggregatedLogs(entries []LogEntry, namespace string) {
	if len(entries) == 0 {
		fmt.Printf("   No logs found for namespace %s\n", namespace)
		return
	}

	// Sort by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	fmt.Printf("\n🔍 AGGREGATED LOGS AND EVENTS (namespace: %s)\n", namespace)
	fmt.Printf("=" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("📊 Total entries: %d\n", len(entries))

	// Group by source for summary
	sourceCount := make(map[string]int)
	for _, entry := range entries {
		sourceCount[entry.Source]++
	}

	fmt.Printf("📋 Sources: ")
	for source, count := range sourceCount {
		fmt.Printf("%s(%d) ", source, count)
	}
	fmt.Printf("\n%s\n", strings.Repeat("-", 80))

	// Display logs with color coding
	for _, entry := range entries {
		if entry.Level == "TRACE" {
			continue
		}
		r.displayColoredLogEntry(entry)
	}

	fmt.Printf("%s\n", strings.Repeat("=", 80))
}

// parseReportToLogEntries converts a Chainsaw JSON report into LogEntry format
// for integration with other logs in chronological order, filtered by namespace
func (r *ChainsawTestRunner) parseReportToLogEntries(reportPath, filterNamespace string) []LogEntry {
	return r.parseReportToLogEntriesWithContext(reportPath, filterNamespace, -1, -1, "")
}

// parseReportToLogEntriesWithContext converts a Chainsaw JSON report into LogEntry format
// with context information for a specific failing step/operation
func (r *ChainsawTestRunner) parseReportToLogEntriesWithContext(reportPath, filterNamespace string, failedStepIndex, failedOpIndex int, failedOpType string) []LogEntry {
	var entries []LogEntry

	report, err := r.parseReport(reportPath)
	if err != nil {
		fmt.Printf("Warning: Could not parse Chainsaw report %s: %v\n", reportPath, err)
		return entries
	}

	for _, test := range report.Tests {
		// Skip tests that don't match the filter namespace
		if filterNamespace != "" && test.Namespace != filterNamespace {
			continue
		}
		// Add test start entry
		entries = append(entries, LogEntry{
			Timestamp: test.StartTime,
			Source:    "chainsaw",
			Level:     "INFO",
			Container: "",
			Pod:       "",
			Message:   fmt.Sprintf("🧪 Test started: %s (namespace: %s)", test.Name, test.Namespace),
			Raw:       fmt.Sprintf("test=%s namespace=%s basePath=%s", test.Name, test.Namespace, test.BasePath),
		})

		for stepIdx, step := range test.Steps {
			// Add step start entry
			entries = append(entries, LogEntry{
				Timestamp: step.StartTime,
				Source:    "chainsaw",
				Level:     "INFO",
				Container: "",
				Pod:       "",
				Message:   fmt.Sprintf("📋 Step %d started: %s", stepIdx+1, step.Name),
				Raw:       fmt.Sprintf("step=%d name=\"%s\" test=%s", stepIdx+1, step.Name, test.Name),
			})

			for opIdx, operation := range step.Operations {
				level := "INFO"
				message := fmt.Sprintf("⚙️  Operation %d.%d: %s (%s)", stepIdx+1, opIdx+1, operation.Type, operation.Name)

				if operation.Failure != nil {
					level = "ERROR"
					message = fmt.Sprintf("❌ Operation %d.%d FAILED: %s (%s)", stepIdx+1, opIdx+1, operation.Type, operation.Name)
				}

				// Create dynamic source name based on step and operation
				sourceName := fmt.Sprintf("chainsaw/step-%d/%d-%s", stepIdx+1, opIdx+1, operation.Type)

				// Add operation entry
				entries = append(entries, LogEntry{
					Timestamp: operation.EndTime,
					Source:    sourceName,
					Level:     level,
					Container: "",
					Pod:       "",
					Message:   message,
					Raw: fmt.Sprintf("step=%d operation=%d type=%s name=\"%s\" duration=%s",
						stepIdx+1, opIdx+1, operation.Type, operation.Name,
						operation.EndTime.Sub(operation.StartTime).String()),
				})
			}

			// Add step completion entry
			stepDuration := step.EndTime.Sub(step.StartTime)
			entries = append(entries, LogEntry{
				Timestamp: step.EndTime,
				Source:    "chainsaw",
				Level:     "INFO",
				Container: "",
				Pod:       "",
				Message:   fmt.Sprintf("✅ Step %d completed: %s (took %s)", stepIdx+1, step.Name, stepDuration.String()),
				Raw:       fmt.Sprintf("step=%d completed duration=%s", stepIdx+1, stepDuration.String()),
			})
		}

		// Add test completion entry
		testDuration := test.EndTime.Sub(test.StartTime)
		testLevel := "INFO"
		testStatus := "✅ PASSED"

		// Check if any operations failed
		for _, step := range test.Steps {
			for _, operation := range step.Operations {
				if operation.Failure != nil {
					testLevel = "ERROR"
					testStatus = "❌ FAILED"
					goto testCompleted
				}
			}
		}

	testCompleted:
		entries = append(entries, LogEntry{
			Timestamp: test.EndTime,
			Source:    "chainsaw",
			Level:     testLevel,
			Container: "",
			Pod:       "",
			Message:   fmt.Sprintf("🏁 Test %s: %s (took %s)", testStatus, test.Name, testDuration.String()),
			Raw:       fmt.Sprintf("test=%s status=%s duration=%s namespace=%s", test.Name, testStatus, testDuration.String(), test.Namespace),
		})
	}

	return entries
}

// parseReport parses a Chainsaw JSON report file
func (r *ChainsawTestRunner) parseReport(reportPath string) (*ChainsawReport, error) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("reading report file: %w", err)
	}

	var report ChainsawReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &report, nil
}

// displayColoredLogEntry displays a single log entry with color coding based on level
func (r *ChainsawTestRunner) displayColoredLogEntry(entry LogEntry) {
	// ANSI color codes
	const (
		reset  = "\033[0m"
		red    = "\033[31m"
		yellow = "\033[33m"
		green  = "\033[32m"
		blue   = "\033[34m"
		purple = "\033[35m"
		cyan   = "\033[36m"
		gray   = "\033[37m"

		brightBlue = "\033[94m"
	)

	// Choose color based on log level
	var levelColor, sourceColor string
	switch entry.Level {
	case "ERROR":
		levelColor = red
	case "WARN":
		levelColor = yellow
	case "INFO":
		levelColor = green
	case "DEBUG":
		levelColor = gray
	default:
		levelColor = reset
	}

	// Choose color for source
	switch entry.Source {
	case "container":
		sourceColor = blue
	case "kaiwo-controller-manager":
		sourceColor = purple
	case "kueue-controller-manager":
		sourceColor = cyan
	case "kuberay-operator":
		sourceColor = brightBlue
	case "event":
		sourceColor = yellow
	case "loki":
		sourceColor = cyan
	default:
		sourceColor = reset
	}

	if strings.HasPrefix(entry.Source, "chainsaw") {
		sourceColor = green
	}

	// Format timestamp
	timeStr := entry.Timestamp.Format("15:04:05.000")

	// Format source info
	sourceInfo := entry.Source
	if entry.Container != "" && entry.Pod != "" {
		sourceInfo = fmt.Sprintf("%s/%s/%s", entry.Source, entry.Pod, entry.Container)
	} else if entry.Container != "" {
		sourceInfo = fmt.Sprintf("%s/%s", entry.Source, entry.Container)
	}

	fmt.Printf("%s %s%-5s%s %s%-25s%s %s %s%s%s\n",
		timeStr,
		levelColor, entry.Level, reset,
		sourceColor, sourceInfo, reset,
		entry.Message,
		gray, entry.Raw, reset,
	)
}

// getKaiwoControllerLogs fetches Kaiwo controller logs either from Loki or locally
func (r *ChainsawTestRunner) getKaiwoControllerLogs(namespace string, startTime, endTime time.Time) []LogEntry {
	// Check if we should fetch logs locally
	if localLogsPath := os.Getenv("KAIWO_LOG_FILE"); localLogsPath != "" {
		// fmt.Printf("   🎛️ Fetching Kaiwo controller logs locally from %s...\n", localLogsPath)
		return r.getLocalKaiwoLogs(localLogsPath, namespace, startTime, endTime)
	}

	// Default: fetch from Loki
	// fmt.Printf("   🎛️ Fetching Kaiwo controller logs from Loki...\n")
	kaiwoQuery := fmt.Sprintf(`{namespace="kaiwo-system", pod=~"kaiwo-controller-manager.*"} |~ "(?i)%s"`, namespace)
	return r.getLokiLogsWithQuery(kaiwoQuery, startTime, endTime, "kaiwo-controller-manager")
}

// getLocalKaiwoLogs reads Kaiwo controller logs from a local file
func (r *ChainsawTestRunner) getLocalKaiwoLogs(logPath, namespace string, startTime, endTime time.Time) []LogEntry {
	var entries []LogEntry

	// Check if the log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Printf("   ⚠️  Local log file not found: %s\n", logPath)
		return entries
	}

	// Read the log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		fmt.Printf("   ⚠️  Error reading local log file: %v\n", err)
		return entries
	}

	// Parse log lines and filter by time range and namespace
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Only include lines that mention the test namespace
		if !strings.Contains(strings.ToLower(line), strings.ToLower(namespace)) {
			continue
		}

		entry := r.parseLogLine(line, "kaiwo-controller-manager", "manager", "kaiwo-controller-manager-local")

		// Filter by time range if timestamp was parsed
		if !entry.Timestamp.IsZero() {
			if entry.Timestamp.Before(startTime) || entry.Timestamp.After(endTime) {
				continue
			}
		}

		entries = append(entries, entry)
	}

	// fmt.Printf("   📊 Found %d relevant local Kaiwo controller log entries\n", len(entries))
	return entries
}

// getLocalProcessLogs attempts to get logs from a local running process
func (r *ChainsawTestRunner) getLocalProcessLogs(namespace string, startTime, endTime time.Time) []LogEntry {
	var entries []LogEntry

	// Try to find recent log files that might be from the local operator
	logPatterns := []string{
		"/tmp/kaiwo-operator*.log",
		"./kaiwo-operator*.log",
		"./operator*.log",
		"/tmp/operator*.log",
	}

	var logFile string
	for _, pattern := range logPatterns {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			// Use the most recent file
			var newest string
			var newestTime time.Time
			for _, match := range matches {
				if info, err := os.Stat(match); err == nil {
					if info.ModTime().After(newestTime) {
						newest = match
						newestTime = info.ModTime()
					}
				}
			}
			if newest != "" {
				logFile = newest
				break
			}
		}
	}

	if logFile == "" {
		fmt.Printf("   ⚠️  No local operator log files found in common locations\n")
		fmt.Printf("   💡 Tip: Set KAIWO_LOCAL_LOGS_PATH to specify the log file location\n")
		return entries
	}

	fmt.Printf("   📁 Using local log file: %s\n", logFile)
	return r.getLocalKaiwoLogs(logFile, namespace, startTime, endTime)
}

// getLokiLogsWithQuery retrieves logs from Loki using Kubernetes service proxy
func (r *ChainsawTestRunner) getLokiLogsWithQuery(query string, startTime, endTime time.Time, source string) []LogEntry {
	var entries []LogEntry

	if r.kubernetesClient == nil || r.LokiServiceName == "" {
		return entries
	}

	// Query Loki via proxy
	lokiResp, err := r.queryLokiViaProxy(query, startTime, endTime)
	if err != nil {
		fmt.Printf("   ⚠️  Error querying Loki: %v\n", err)
		return entries
	}

	// Parse Loki response
	for _, result := range lokiResp.Data.Result {
		for _, value := range result.Values {
			if len(value) >= 2 {
				// Parse timestamp from Loki (nanoseconds)
				tsNano, err := strconv.ParseInt(value[0], 10, 64)
				if err != nil {
					continue
				}
				timestamp := time.Unix(0, tsNano)

				// Parse the log line
				entry := r.parseLogLine(value[1], source, result.Stream["container"], result.Stream["pod"])
				entry.Timestamp = timestamp

				// Override source if we have stream metadata
				if result.Stream["namespace"] != "" {
					if result.Stream["pod"] != "" && result.Stream["container"] != "" {
						entry.Source = "container"
						entry.Pod = result.Stream["pod"]
						entry.Container = result.Stream["container"]
					}
				}

				entries = append(entries, entry)
			}
		}
	}

	return entries
}
