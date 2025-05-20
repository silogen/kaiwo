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

package testutils

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func DebugTest(ctx context.Context, clientset *kubernetes.Clientset, namespace string, printLevel string) error {
	// List resources (Pods and Deployments) and their events.

	var logs []LogEntry

	// List Kaiwo controller logs
	if kaiwoControllerLogs, err := listKaiwoControllerLogsForNamespace(ctx, clientset, "kaiwo-system"); err != nil {
		return fmt.Errorf("error listing Kaiwo controller logs: %w", err)
	} else {
		var kaiwoLogs []LogEntry
		for _, log := range kaiwoControllerLogs {
			// Only include logs from the given namespace
			if len(log.ParsedContext) > 0 {
				if logNamespace, exists := log.ParsedContext["namespace"]; exists && logNamespace == namespace {
					kaiwoLogs = append(kaiwoLogs, log)
				}
			}
		}
		if len(kaiwoLogs) == 0 {
			fmt.Println("No Kaiwo controller logs found")
		} else {
			logs = append(logs, kaiwoLogs...)
		}
	}

	// List Kueue controller logs
	if kaiwoControllerLogs, err := listKueueControllerLogsForNamespace(ctx, clientset, "kueue-system"); err != nil {
		return fmt.Errorf("error listing Kueue controller logs: %w", err)
	} else {
		var kueueLogs []LogEntry
		for _, log := range kaiwoControllerLogs {
			// Only include logs from the given namespace
			if len(log.ParsedContext) > 0 {
				if logNamespace, exists := log.ParsedContext["namespace"]; exists && logNamespace == namespace {
					kueueLogs = append(kueueLogs, log)
				}
			}
		}
		if len(kueueLogs) == 0 {
			fmt.Println("No Kueue controller logs found")
		} else {
			logs = append(logs, kueueLogs...)
		}
	}

	// List namespace pod logs
	if namespacePodLogs, err := listAllNamespacePodLogs(ctx, clientset, namespace); err != nil {
		return fmt.Errorf("error listing all namespace pod logs: %w", err)
	} else {
		if len(namespacePodLogs) == 0 {
			fmt.Println("No namespace pod logs found")
		}
		logs = append(logs, namespacePodLogs...)
	}

	if eventLogs, err := listEventsAsLogs(ctx, clientset, namespace); err != nil {
		return fmt.Errorf("error listing events logs: %w", err)
	} else {
		if len(eventLogs) == 0 {
			fmt.Println("No events logs found")
		}
		logs = append(logs, eventLogs...)
	}

	printLogs(logs, printLevel, "")

	return nil
}

type LogLevel string

func (l LogLevel) Number() int {
	switch l {
	case LogLevelTrace:
		return 2
	case LogLevelDebug:
		return 1
	case LogLevelInfo:
		return 0
	case LogLevelWarn:
		return -1
	case LogLevelError:
		return -2
	default:
		return 0
	}
}

const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warning"
	LogLevelError LogLevel = "error"
)

type LogEntry struct {
	Level         LogLevel `json:"level"`
	Message       string   `json:"message"`
	Time          string   `json:"time"`
	Context       string   `json:"context"`
	ParsedContext map[string]any
	Logger        string `json:"logger"`
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
	Namespace     string `json:"namespace"`
}

var contextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))

var errorCodeRegex = regexp.MustCompile(`(\w)\d+`)

func listEventsAsLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]LogEntry, error) {
	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing events: %w", err)
	}

	var logs []LogEntry

	for _, event := range events.Items {
		timestamp := event.LastTimestamp.Format(time.RFC3339Nano)
		// Add the nanoseconds so sorting works with logs from pods
		timestamp = strings.ReplaceAll(timestamp, "Z", ".000000000Z")
		log := LogEntry{
			Message:       event.Message,
			Time:          timestamp,
			Level:         LogLevel(strings.ToLower(event.Type)),
			Namespace:     event.Namespace,
			PodName:       event.InvolvedObject.Kind,
			ContainerName: event.InvolvedObject.Name,
			Logger:        fmt.Sprintf("EVENT: %s", event.Reason),
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func listKaiwoControllerLogsForNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]LogEntry, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kaiwo,control-plane=kaiwo-controller-manager",
	})
	if err != nil {
		return nil, fmt.Errorf("error listing kaiwo-controller pods: %v", err)
	}
	var logs []LogEntry
	for _, pod := range pods.Items {
		if podLogs, err := listPodLogs(ctx, clientset, namespace, pod); err != nil {
			return nil, fmt.Errorf("error listing kaiwo-controller pod logs: %v", err)
		} else {
			logs = append(logs, podLogs...)
		}
	}
	return logs, nil
}

func listPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string, pod corev1.Pod) ([]LogEntry, error) {
	var logs []LogEntry

	for _, container := range pod.Spec.Containers {
		if containerLogs, err := listContainerLogs(ctx, clientset, namespace, pod, container.Name); err != nil {
			return nil, err
		} else {
			logs = append(logs, containerLogs...)
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if containerLogs, err := listContainerLogs(ctx, clientset, namespace, pod, container.Name); err != nil {
			return nil, err
		} else {
			for _, log := range containerLogs {
				log.ContainerName = "[init]" + log.ContainerName
				logs = append(logs, log)
			}
		}
	}
	return logs, nil
}

func listContainerLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string, pod corev1.Pod, containerName string) ([]LogEntry, error) {
	var logs []LogEntry
	podLogOpts := &corev1.PodLogOptions{
		Container:  containerName,
		Timestamps: true,
	}
	req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		if _, err := fmt.Fprintf(os.Stderr, "error opening stream: %v\n", err); err != nil {
			panic(err)
		}
		return nil, nil
	}
	defer func(podLogs io.ReadCloser) {
		err := podLogs.Close()
		if err != nil {
			panic(err)
		}
	}(podLogs)

	scanner := bufio.NewScanner(podLogs)
	for scanner.Scan() {

		line := scanner.Text()

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			fmt.Println("Malformed log line:", line)
			continue
		}

		timestamp := parts[0] // First part is the timestamp
		line = parts[1]       // Second part is the log content

		// Try to parse each line as JSON.
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			// If the log line isn’t valid JSON, print it normally.
			match := errorCodeRegex.FindStringSubmatch(line)
			var level LogLevel
			if len(match) > 1 {
				levelChar := strings.ToUpper(match[1])
				switch levelChar {
				case "E":
					level = LogLevelError
				case "W":
					level = LogLevelWarn
				case "I":
					level = LogLevelInfo
				}
			}
			logs = append(logs, LogEntry{
				Time:          timestamp,
				Level:         level,
				Message:       line,
				PodName:       pod.Name,
				ContainerName: containerName,
				Namespace:     namespace,
			})
			continue
		}

		level, hasLevel := logEntry["level"].(string)
		message, hasMessage := logEntry["msg"].(string)
		_, hasTime := logEntry["ts"].(string)
		logger, hasLogger := logEntry["logger"].(string)

		var logContext string

		if !hasLevel || !hasMessage {
			message = line
		} else {
			if hasTime {
				delete(logEntry, "ts")
			}
			delete(logEntry, "level")
			delete(logEntry, "msg")
			rest, err := json.Marshal(logEntry)
			if err != nil {
				return nil, fmt.Errorf("error marshalling logEntry: %v", err)
			}
			logContext = string(rest)
		}
		if hasLogger {
			delete(logEntry, "logger")
		}

		logs = append(logs, LogEntry{
			Level:         LogLevel(level),
			Message:       message,
			Time:          timestamp,
			Logger:        logger,
			Context:       logContext,
			PodName:       pod.Name,
			ContainerName: containerName,
			Namespace:     namespace,
			ParsedContext: logEntry,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning logs: %w", err)
	}
	return logs, nil
}

// listPodLogs lists all pods in the given namespace and streams their logs.
func listAllNamespacePodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]LogEntry, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing pods: %w", err)
	}

	var logs []LogEntry

	for _, pod := range pods.Items {
		if podLogs, err := listPodLogs(ctx, clientset, namespace, pod); err != nil {
			return nil, fmt.Errorf("error listing pod logs: %w", err)
		} else {
			logs = append(logs, podLogs...)
		}
	}
	return logs, nil
}

func printLogs(logs []LogEntry, printLevel string, writeTo string) {
	printLevelNumber := LogLevel(printLevel).Number()
	var writer *bufio.Writer
	if writeTo != "" {
		dir := filepath.Dir(writeTo)

		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("Error creating directories: %v\n", err)
			return
		}

		file, err := os.Create(writeTo)
		if err != nil {
			fmt.Println("Error creating file:", err)
			return
		}
		// Ensure the file is closed when we're done.
		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				panic(err)
			}
		}(file)

		writer = bufio.NewWriter(file)
	}

	podColors := map[string]lipgloss.Color{}

	randRange := func(min, max int) int {
		return rand.Intn(max-min) + min
	}

	sort.Slice(logs[:], func(i, j int) bool {
		return logs[i].Time < logs[j].Time
	})

	for _, log := range logs {
		logLevelNumber := log.Level.Number()
		if logLevelNumber > printLevelNumber {
			continue
		}
		if _, exists := podColors[log.PodName]; !exists {
			r := randRange(125, 256)
			g := randRange(125, 256)
			b := randRange(125, 256)

			color := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
			podColors[log.PodName] = color
		}

		rowColor := podColors[log.PodName]

		var background lipgloss.Color

		switch log.Level {
		case LogLevelError:
			background = "#d62020"
		case LogLevelWarn:
			background = "#c4510a"
		}

		baseStyle := lipgloss.NewStyle().Background(background).Foreground(rowColor)
		loggerStyle := lipgloss.NewStyle().Background(background).Foreground(rowColor).Italic(true)
		messageStyle := lipgloss.NewStyle().Background(background).Foreground(rowColor).Bold(true)

		timeWidth := 33
		levelWidth := 10

		columns := []string{
			baseStyle.Render(fmt.Sprintf("%-*s", timeWidth, log.Time)),
			baseStyle.Render(fmt.Sprintf("%-*s", levelWidth, log.Level)),
			baseStyle.Render(fmt.Sprintf("%s/%s/%s", log.Namespace, log.PodName, log.ContainerName)),
		}
		if log.Logger != "" {
			columns = append(columns, loggerStyle.Render(log.Logger))
		}
		columns = append(columns, messageStyle.Render(log.Message))

		formattedRow := strings.Join(columns, baseStyle.Render("  "))

		if log.Context != "" && log.Context != "{}" {
			formattedRow += " " + contextStyle.Render(log.Context)
		}

		fmt.Println(formattedRow)

		if writer != nil {
			if _, err := fmt.Fprintln(writer, formattedRow); err != nil {
				panic(err)
			}
		}
	}
	if writer != nil {
		if err := writer.Flush(); err != nil {
			panic(err)
		}
	}
}

func listKueueControllerLogsForNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]LogEntry, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kueue,control-plane=controller-manager",
	})
	if err != nil {
		return nil, fmt.Errorf("error listing kueue controller pods: %v", err)
	}
	var logs []LogEntry
	for _, pod := range pods.Items {
		if podLogs, err := listPodLogs(ctx, clientset, namespace, pod); err != nil {
			return nil, fmt.Errorf("error listing kueuecontroller pod logs: %v", err)
		} else {
			logs = append(logs, podLogs...)
		}
	}
	return logs, nil
}
