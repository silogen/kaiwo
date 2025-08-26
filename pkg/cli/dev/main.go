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
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	srcEvent        = "Event"
	srcKueue        = "Kueue"
	srcRay          = "Ray"
	srcKaiwo        = "Kaiwo"
	srcNamespacePod = "NamespacePod"

	levelTrace = "trace"
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
	levelFatal = "fatal"
)

var (
	// ANSI colors always on (Chainsaw-friendly).
	colorReset   = "\033[0m"
	colorGray    = "\033[90m"
	colorWhite   = "\033[97m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorBRed    = "\033[91m"

	sourceColors = map[string]string{
		srcEvent:        colorMagenta,
		srcKueue:        colorCyan,
		srcRay:          colorYellow,
		srcKaiwo:        colorGreen,
		srcNamespacePod: colorBlue,
	}

	levelColors = map[string]string{
		levelTrace: colorGray,
		levelDebug: colorGray,
		levelInfo:  colorGreen,
		levelWarn:  colorYellow,
		levelError: colorRed,
		levelFatal: colorBRed,
	}

	// Severity ranking for filtering
	levelRank = map[string]int{
		levelTrace: 0,
		levelDebug: 1,
		levelInfo:  2,
		levelWarn:  3,
		"warning":  3,
		levelError: 4,
		levelFatal: 5,
	}
)

type logEntry struct {
	Time      time.Time
	Level     string
	Source    string
	Namespace string
	Message   string
	Extras    map[string]any // remaining JSON fields or extracted context
}

type options struct {
	controllerDeployments []string // "namespace/name"
	kaiwoDeployment       string   // "namespace/name"
	localLogsPath         string
	limitLines            int

	// New filtering knobs
	namespacedLevel string // threshold for logs in the test namespace (default "debug")
	clusterLevel    string // threshold for logs outside the test namespace (default "info")
}

func defaultControllers() []string {
	return []string{
		"kueue-system/kueue-controller-manager",
		"default/kuberay-operator",
	}
}

func main() {
	// Avoid noisy glog flags from client-go
	_ = flag.CommandLine.Parse([]string{})

	opts := &options{
		controllerDeployments: defaultControllers(),
		kaiwoDeployment:       "kaiwo-system/kaiwo-controller-manager",
		limitLines:            0,
		namespacedLevel:       levelDebug,
		clusterLevel:          levelInfo,
	}

	root := &cobra.Command{
		Use:   "kaiwo-dump <test-namespace>",
		Short: "Collect and colorize logs and events for Kaiwo test runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			testNamespace := args[0]
			ctx := context.Background()

			clientset, _, err := buildKubeClient()
			if err != nil {
				return err
			}

			entriesChannel := make(chan *logEntry, 4096)
			var allEntries []*logEntry

			// Collector
			done := make(chan struct{})
			go func() {
				for entry := range entriesChannel {
					if entry != nil && !entry.Time.IsZero() {
						allEntries = append(allEntries, entry)
					}
				}
				close(done)
			}()

			group, ctx := errgroup.WithContext(ctx)

			// Namespace pod logs (JSON + non-JSON supported)
			group.Go(func() error {
				return collectNamespacePodLogs(ctx, clientset, testNamespace, opts.limitLines, entriesChannel)
			})

			// Events (only from test namespace)
			group.Go(func() error {
				return collectEvents(ctx, clientset, testNamespace, entriesChannel)
			})

			// Controller deployments (JSON preferred, non-JSON tolerated)
			group.Go(func() error {
				// NOTE: Now collects ALL controller logs; filtering happens at the end.
				return collectControllerLogs(ctx, clientset, opts.controllerDeployments, testNamespace, opts.limitLines, entriesChannel)
			})

			// Kaiwo logs (cluster, fallback to local file) — collect all, filter later
			group.Go(func() error {
				found, err := collectKaiwoLogs(ctx, clientset, opts.kaiwoDeployment, testNamespace, opts.limitLines, entriesChannel)
				if err != nil {
					return err
				}
				if !found {
					if opts.localLogsPath == "" {
						fmt.Fprintln(os.Stderr, "Kaiwo deployment not found and --local-logs not provided; skipping Kaiwo logs")
						return nil
					}
					return collectLocalKaiwo(opts.localLogsPath, entriesChannel)
				}
				return nil
			})

			if err := group.Wait(); err != nil {
				close(entriesChannel)
				<-done
				return err
			}
			close(entriesChannel)
			<-done

			// Sort chronologically
			sort.Slice(allEntries, func(i, j int) bool { return allEntries[i].Time.Before(allEntries[j].Time) })

			// Apply final filtering per user's rules
			filtered := filterEntries(allEntries, testNamespace, opts)
			if len(filtered) > 0 {
				baseTime := filtered[0].Time
				for _, entry := range filtered {
					printEntry(entry, baseTime)
				}
			}

			return nil
		},
	}

	root.Flags().StringSliceVar(&opts.controllerDeployments, "controllers", opts.controllerDeployments, "Controller deployments as namespace/name (JSON logs). Example: kueue-system/kueue-controller-manager,ray-system/kuberay-operator")
	root.Flags().StringVar(&opts.kaiwoDeployment, "kaiwo-deploy", opts.kaiwoDeployment, "Kaiwo controller deployment as namespace/name")
	root.Flags().StringVar(&opts.localLogsPath, "local-logs", "", "Path to local Kaiwo logs (fallback if Kaiwo deployment not present)")
	root.Flags().IntVar(&opts.limitLines, "limit", 0, "Limit log lines per container (0 = unlimited)")

	// New flags
	root.Flags().StringVar(&opts.namespacedLevel, "namespaced-level", opts.namespacedLevel, "Minimum level to include for logs in the test namespace (trace|debug|info|warn|error|fatal)")
	root.Flags().StringVar(&opts.clusterLevel, "cluster-level", opts.clusterLevel, "Minimum level to include for logs outside the test namespace (trace|debug|info|warn|error|fatal). Cluster logs are only kept between the first and last namespaced log; if there are no namespaced logs, cluster logs are omitted.")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- Kube client ---

func buildKubeClient() (*kubernetes.Clientset, *rest.Config, error) {
	// Prefer in-cluster; fall back to KUBECONFIG or default path without exposing flags.
	if cfg, err := rest.InClusterConfig(); err == nil {
		clientset, cerr := kubernetes.NewForConfig(cfg)
		return clientset, cfg, cerr
	}
	// out-of-cluster: use default loading rules (env KUBECONFIG or ~/.kube/config)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	return clientset, cfg, err
}

// --- Collectors ---

func collectNamespacePodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string, limit int, out chan<- *logEntry) error {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	group, ctx := errgroup.WithContext(ctx)
	for _, podItem := range podList.Items {
		pod := podItem
		for _, containerItem := range pod.Spec.Containers {
			container := containerItem
			group.Go(func() error {
				return streamPodLogs(ctx, clientset, pod.Namespace, pod.Name, container.Name, limit, func(ts time.Time, line string) {
					// Try JSON; fall back to non-JSON.
					if entry := parseJSONLine(ts, line); entry != nil {
						entry.Source = srcNamespacePod
						if entry.Namespace == "" {
							entry.Namespace = namespace
						}
						out <- entry
						return
					}
					entry := parseGenericLine(ts, line)
					entry.Source = srcNamespacePod
					if entry.Namespace == "" {
						entry.Namespace = namespace
					}
					out <- entry
				})
			})
		}
	}
	return group.Wait()
}

func collectEvents(ctx context.Context, clientset *kubernetes.Clientset, namespace string, out chan<- *logEntry) error {
	if err := collectEventsV1(ctx, clientset, namespace, out); err == nil {
		fmt.Println("Ok!")
		return nil
	} else {
		fmt.Println(fmt.Errorf("collectEventsV1: %w", err))
	}

	return collectEventsCoreV1(ctx, clientset, namespace, out)
}

func collectEventsV1(ctx context.Context, clientset *kubernetes.Clientset, namespace string, out chan<- *logEntry) error {
	evs, err := clientset.EventsV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ev := range evs.Items {
		// Prefer precise fields
		var ts time.Time
		switch {
		case !ev.EventTime.IsZero():
			ts = ev.EventTime.Time
		case ev.Series != nil && !ev.Series.LastObservedTime.IsZero():
			ts = ev.Series.LastObservedTime.Time
		case !ev.DeprecatedLastTimestamp.IsZero():
			ts = ev.DeprecatedLastTimestamp.Time
		case !ev.DeprecatedFirstTimestamp.IsZero():
			ts = ev.DeprecatedFirstTimestamp.Time
		case !ev.CreationTimestamp.IsZero():
			ts = ev.CreationTimestamp.Time
		default:
			ts = time.Now()
		}

		level := levelInfo
		if strings.EqualFold(ev.Type, "Warning") {
			level = levelWarn
		}

		msg := ev.Note
		if ev.Reason != "" {
			msg = fmt.Sprintf("[%s] %s", ev.Reason, ev.Note)
		}

		count := 0
		if ev.Series != nil && ev.Series.Count > 0 {
			count = int(ev.Series.Count)
		} else if ev.DeprecatedCount > 0 {
			count = int(ev.DeprecatedCount)
		}

		extras := map[string]any{
			"reason":     ev.Reason,
			"count":      count,
			"involved":   fmt.Sprintf("%s/%s", ev.Regarding.Kind, ev.Regarding.Name),
			"reporting":  ev.ReportingController,
			"event_type": ev.Type,
		}

		out <- &logEntry{
			Time:      ts,
			Level:     level,
			Source:    srcEvent,
			Namespace: namespace,
			Message:   msg,
			Extras:    extras,
		}
	}
	return nil
}

func collectEventsCoreV1(ctx context.Context, clientset *kubernetes.Clientset, namespace string, out chan<- *logEntry) error {
	eventList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	now := time.Now()
	for _, ev := range eventList.Items {
		var ts time.Time
		switch {
		case !ev.EventTime.IsZero():
			ts = ev.EventTime.Time
		case !ev.LastTimestamp.IsZero():
			ts = ev.LastTimestamp.Time
		case !ev.FirstTimestamp.IsZero():
			ts = ev.FirstTimestamp.Time
		default:
			ts = now
		}
		level := levelInfo
		if strings.EqualFold(ev.Type, "Warning") {
			level = levelWarn
		}
		extras := map[string]any{
			"reason":     ev.Reason,
			"count":      ev.Count,
			"involved":   fmt.Sprintf("%s/%s", ev.InvolvedObject.Kind, ev.InvolvedObject.Name),
			"reporting":  ev.ReportingController,
			"event_type": ev.Type,
		}
		msg := ev.Message
		if ev.Reason != "" {
			msg = fmt.Sprintf("[%s] %s", ev.Reason, ev.Message)
		}
		out <- &logEntry{
			Time:      ts,
			Level:     level,
			Source:    srcEvent,
			Namespace: namespace,
			Message:   msg,
			Extras:    extras,
		}
	}
	return nil
}

func collectControllerLogs(ctx context.Context, clientset *kubernetes.Clientset, deployments []string, _ string, limit int, out chan<- *logEntry) error {
	// Collect ALL controller logs; filtering is done later.
	group, ctx := errgroup.WithContext(ctx)
	for _, d := range deployments {
		namespace, name, err := splitNamespaceName(d)
		if err != nil {
			return err
		}
		source := srcKueue
		lower := strings.ToLower(name)
		if strings.Contains(lower, "ray") {
			source = srcRay
		} else if strings.Contains(lower, "kueue") {
			source = srcKueue
		}
		nsCopy, nameCopy, sourceCopy := namespace, name, source
		group.Go(func() error {
			return collectDeploymentLogs(ctx, clientset, nsCopy, nameCopy, sourceCopy, limit, out)
		})
	}
	return group.Wait()
}

func collectKaiwoLogs(ctx context.Context, clientset *kubernetes.Clientset, deployment string, _ string, limit int, out chan<- *logEntry) (bool, error) {
	namespace, name, err := splitNamespaceName(deployment)
	if err != nil {
		return false, err
	}
	dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, nil // not found → let caller fall back to local logs
	}
	return true, collectDeploymentLogs(ctx, clientset, dep.Namespace, dep.Name, srcKaiwo, limit, out)
}

func collectLocalKaiwo(path string, out chan<- *logEntry) error {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// try "[timestamp] json-or-text", then raw JSON, then plain text
		if ts, rest, ok := splitLeadingTimestamp(line); ok {
			if entry := parseJSONLine(ts, rest); entry != nil {
				entry.Source = srcKaiwo
				out <- entry
				continue
			}
			entry := parseGenericLine(ts, rest)
			entry.Source = srcKaiwo
			out <- entry
			continue
		}

		if entry := parseJSONLine(time.Time{}, line); entry != nil {
			entry.Source = srcKaiwo
			out <- entry
			continue
		}

		out <- &logEntry{
			Time:    time.Now(),
			Level:   levelInfo,
			Source:  srcKaiwo,
			Message: strings.TrimSpace(line),
			Extras:  map[string]any{},
		}
	}
	return scanner.Err()
}

// --- Deployment / Pod log streaming ---

func collectDeploymentLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, name, source string, limit int, out chan<- *logEntry) error {
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	selector := klabels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods for deployment %s/%s", namespace, name)
	}

	group, ctx := errgroup.WithContext(ctx)
	for _, podItem := range podList.Items {
		pod := podItem
		for _, containerItem := range pod.Spec.Containers {
			container := containerItem
			group.Go(func() error {
				return streamPodLogs(ctx, clientset, pod.Namespace, pod.Name, container.Name, limit, func(ts time.Time, line string) {
					if entry := parseJSONLine(ts, line); entry != nil {
						entry.Source = source
						out <- entry
						return
					}
					entry := parseGenericLine(ts, line)
					entry.Source = source
					out <- entry
				})
			})
		}
	}
	return group.Wait()
}

func streamPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName, containerName string, limit int, onLine func(time.Time, string)) error {
	options := &corev1.PodLogOptions{
		Container:  containerName,
		Timestamps: true, // ensures sortable lines for non-JSON logs
	}
	if limit > 0 {
		tail := int64(limit)
		options.TailLines = &tail
	}

	request := clientset.CoreV1().Pods(namespace).GetLogs(podName, options)
	stream, err := request.Stream(ctx)
	if err != nil {
		// retry briefly — pods may be transient
		backoff := wait.Backoff{Steps: 3, Duration: 300 * time.Millisecond, Factor: 1.5, Jitter: 0.1}
		_ = wait.ExponentialBackoff(backoff, func() (bool, error) {
			s, e := request.Stream(ctx)
			if e != nil {
				return false, nil
			}
			stream = s
			return true, nil
		})
	}
	if stream == nil {
		return fmt.Errorf("unable to stream logs for %s/%s[%s]", namespace, podName, containerName)
	}
	defer func(stream io.ReadCloser) {
		err := stream.Close()
		if err != nil {
			panic(err)
		}
	}(stream)

	reader := bufio.NewScanner(stream)
	for reader.Scan() {
		line := reader.Text()
		ts, rest, ok := splitLeadingTimestamp(line)
		if !ok {
			// should not happen with Timestamps=true, but be tolerant:
			onLine(time.Now(), line)
			continue
		}
		onLine(ts, rest)
	}
	return reader.Err()
}

// --- Line parsing utilities ---

var timestampStartRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[^ ]+`)

func splitLeadingTimestamp(line string) (time.Time, string, bool) {
	if !timestampStartRe.MatchString(line) {
		return time.Time{}, "", false
	}
	space := strings.IndexByte(line, ' ')
	if space <= 0 {
		return time.Time{}, "", false
	}
	raw := line[:space]
	rest := strings.TrimSpace(line[space+1:])
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		if t2, e2 := time.Parse(time.RFC3339, raw); e2 == nil {
			return t2, rest, true
		}
		return time.Time{}, "", false
	}
	return t, rest, true
}

func parseJSONLine(ts time.Time, line string) *logEntry {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "{") || !strings.HasSuffix(trim, "}") {
		return nil
	}

	decoder := json.NewDecoder(strings.NewReader(trim))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}

	entry := &logEntry{
		Time:   ts,
		Extras: map[string]any{},
	}

	// Prefer embedded time if present
	if entry.Time.IsZero() {
		if embedded := extractTimeFromMap(payload); !embedded.IsZero() {
			entry.Time = embedded
		} else {
			entry.Time = time.Now()
		}
	}

	entry.Level = lowerString(getStringAny(payload, "level", "lvl", "severity"))
	entry.Level = normalizeLevel(entry.Level)
	entry.Message = getStringAny(payload, "msg", "message", "log")
	entry.Namespace = getStringAny(payload, "namespace", "ns", "kubernetes.namespace_name")

	for k, v := range payload {
		lk := strings.ToLower(k)
		switch lk {
		case "ts", "time", "timestamp", "t", "level", "lvl", "severity", "msg", "message", "log", "namespace", "ns", "kubernetes.namespace_name":
			continue
		default:
			entry.Extras[k] = v
		}
	}
	return entry
}

func parseGenericLine(ts time.Time, line string) *logEntry {
	if ts.IsZero() {
		ts = time.Now()
	}
	trim := strings.TrimSpace(line)
	// If it actually is JSON, try to parse
	if strings.HasPrefix(trim, "{") && strings.HasSuffix(trim, "}") {
		if e := parseJSONLine(ts, trim); e != nil {
			return e
		}
	}
	return &logEntry{
		Time:    ts,
		Level:   levelInfo,
		Message: trim,
		Extras:  map[string]any{},
	}
}

func getStringAny(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprint(v)
		}
		for kk, vv := range m {
			if strings.EqualFold(kk, k) {
				if s, ok := vv.(string); ok {
					return s
				}
				return fmt.Sprint(vv)
			}
		}
	}
	return ""
}

func extractTimeFromMap(m map[string]any) time.Time {
	candidates := []string{"ts", "time", "timestamp", "t"}
	for _, k := range candidates {
		if v, ok := m[k]; ok {
			switch vv := v.(type) {
			case string:
				if t, err := time.Parse(time.RFC3339Nano, vv); err == nil {
					return t
				}
				if t, err := time.Parse(time.RFC3339, vv); err == nil {
					return t
				}
			case json.Number:
				if i, err := vv.Int64(); err == nil {
					if i > 1e12 {
						return time.UnixMilli(i)
					}
					return time.Unix(i, 0)
				}
			case float64:
				if vv > 1e12 {
					return time.UnixMilli(int64(vv))
				}
				return time.Unix(int64(vv), 0)
			}
		}
	}
	return time.Time{}
}

func lowerString(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// --- Filtering ---

func severityRank(s string) int {
	r, ok := levelRank[lowerString(s)]
	if !ok {
		// Treat unknown/missing as "info"
		return levelRank[levelInfo]
	}
	return r
}

func atLeast(level, min string) bool {
	return severityRank(level) >= severityRank(min)
}

func filterEntries(entries []*logEntry, testNamespace string, opts *options) []*logEntry {
	if len(entries) == 0 {
		return entries
	}

	// Identify first and last namespaced timestamps
	var windowStart, windowEnd time.Time
	for _, e := range entries {
		if e.Namespace == testNamespace {
			windowStart = e.Time
			break
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Namespace == testNamespace {
			windowEnd = entries[i].Time
			break
		}
	}

	hasWindow := !windowStart.IsZero() && !windowEnd.IsZero()
	out := make([]*logEntry, 0, len(entries))

	for _, e := range entries {
		isNamespaced := e.Namespace == testNamespace

		if isNamespaced {
			// Apply namespaced threshold unconditionally
			if atLeast(e.Level, opts.namespacedLevel) {
				out = append(out, e)
			}
			continue
		}

		// Cluster (i.e., anything not in the test namespace)
		if !hasWindow {
			// No namespaced logs -> exclude all cluster logs
			continue
		}
		// Only include cluster logs that fall within the namespaced time window
		if e.Time.Before(windowStart) || e.Time.After(windowEnd) {
			continue
		}
		if atLeast(e.Level, opts.clusterLevel) {
			out = append(out, e)
		}
	}

	return out
}

// --- Printing ---

func printEntry(e *logEntry, baseTime time.Time) {
	timestamp := e.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	age := e.Time.Sub(baseTime).Seconds()

	level := lowerString(e.Level)
	if level == "" {
		level = levelInfo
	}
	levelColor := levelColors[level]
	if levelColor == "" {
		levelColor = colorGray
	}

	sourceColor := sourceColors[e.Source]
	if sourceColor == "" {
		sourceColor = colorGray
	}

	// Extras (sorted) in gray
	extras := ""
	if len(e.Extras) > 0 {
		keys := make([]string, 0, len(e.Extras))
		for k := range e.Extras {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, e.Extras[k]))
		}
		extras = " " + colorGray + strings.Join(parts, " ") + colorReset
	}

	namespaceStr := ""
	if e.Namespace != "" {
		namespaceStr = fmt.Sprintf(" %sns=%s%s", colorGray, e.Namespace, colorReset)
	}

	fmt.Printf("%s (%.3fs)  %s%-9s%s  %s%s%s  %s%s%s%s\n",
		timestamp, age,
		levelColor, strings.ToUpper(levelOrDefault(level)), colorReset,
		sourceColor, e.Source, colorReset,
		colorWhite, e.Message, colorReset,
		namespaceStr+extras,
	)
}

var levelParenRe = regexp.MustCompile(`(?i)^level\((-?\d+)\)$`)

func normalizeLevel(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return levelInfo
	}
	// If it's exactly an integer like "-2", map it.
	if i, err := strconv.Atoi(s); err == nil {
		switch {
		case i <= -2:
			return levelTrace
		case i == -1:
			return levelDebug
		case i == 0:
			return levelInfo
		case i == 1:
			return levelWarn
		case i >= 5:
			return levelFatal
		default:
			return levelError // 2..4
		}
	}
	// If it's in the Zap string form "Level(-2)"
	if m := levelParenRe.FindStringSubmatch(s); len(m) == 2 {
		if i, err := strconv.Atoi(m[1]); err == nil {
			switch {
			case i <= -2:
				return levelTrace
			case i == -1:
				return levelDebug
			case i == 0:
				return levelInfo
			case i == 1:
				return levelWarn
			case i >= 5:
				return levelFatal
			default:
				return levelError
			}
		}
	}
	// normalize synonyms
	if s == "warning" {
		return levelWarn
	}
	return s
}

func levelOrDefault(l string) string {
	if l == "" {
		return "INFO"
	}
	return l
}

// --- Misc helpers

func splitNamespaceName(s string) (string, string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected namespace/name, got %q", s)
	}
	return parts[0], parts[1], nil
}
