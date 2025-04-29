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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//go:embed querytemplates/cpu.promql
var cpuTemplate string

//go:embed querytemplates/gpu.promql
var gpuTemplate string

// Environment variable keys.
const (
	envPrefix = "RESOURCE_MONITORING_"

	EnvMonitoringEnabled = envPrefix + "ENABLED"

	// EnvPrometheusEndpoint is the Prometheus endpoint to use
	EnvPrometheusEndpoint = envPrefix + "PROMETHEUS_ENDPOINT"

	// EnvPollingInterval is the polling interval to run the GPU metrics poller
	EnvPollingInterval = envPrefix + "POLLING_INTERVAL"

	// EnvAveragingTime is the time to use to average the GPU metrics over
	EnvAveragingTime = envPrefix + "AVERAGING_INTERVAL"

	// EnvMinAliveTime is the time that a pod must have been alive for in order to qualify for inspection
	EnvMinAliveTime = envPrefix + "MIN_ALIVE_TIME"

	// EnvLowUtilizationThreshold is the threshold which, if the metric goes under, the GPU is considered underutilized
	EnvLowUtilizationThreshold = envPrefix + "LOW_UTILIZATION_THRESHOLD"

	// EnvTargetNamespaces is a comma-separated list of namespaces to filter on (e.g. "ns1,ns2" )
	EnvTargetNamespaces = envPrefix + "TARGET_NAMESPACES"

	// EnvProfile selects the monitoring profile (gpu or cpu)
	EnvProfile = envPrefix + "PROFILE"
)

const (
	MetricsComponentName = "KaiwoResourceMonitor"
	gpuProfileName       = "gpu"
)

func IsMetricsMonitoringEnabled() bool {
	return os.Getenv(EnvMonitoringEnabled) == "true"
}

// MetricsWatcher polls Prometheus and updates Kaiwo statuses.
type MetricsWatcher struct {
	logger logr.Logger

	prometheusEndpoint      string
	pollInterval            time.Duration
	averagingInterval       time.Duration
	minAliveForInterval     time.Duration
	lowUtilizationThreshold float64
	query                   string
	profile                 string // "gpu" or "cpu"

	statusCache map[string]bool

	k8sClient client.Client
	scheme    *runtime.Scheme
	recorder  record.EventRecorder
}

// NewMetricsWatcherFromEnv constructs a MetricsWatcher from environment variables.
func NewMetricsWatcherFromEnv(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
) (*MetricsWatcher, error) {
	endpoint := os.Getenv(EnvPrometheusEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("environment variable %s not set", EnvPrometheusEndpoint)
	}

	var namespaces []string
	nsFilter := os.Getenv(EnvTargetNamespaces)
	if nsFilter != "" {
		namespaces = strings.Split(nsFilter, ",")
	}

	pollInterval, err := parseIntervalFromEnv(EnvPollingInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse poll interval: %v", err)
	}

	averagingInterval, err := parseIntervalFromEnv(EnvAveragingTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse averaging interval: %v", err)
	}

	minAliveForInterval, err := parseIntervalFromEnv(EnvMinAliveTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse min alive interval: %v", err)
	}

	lowUtilizationThresholdStr := os.Getenv(EnvLowUtilizationThreshold)
	if lowUtilizationThresholdStr == "" {
		return nil, fmt.Errorf("environment variable %s not set", EnvLowUtilizationThreshold)
	}
	lowUtilizationThreshold, err := strconv.ParseFloat(lowUtilizationThresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse low utilization threshold %s: %v", lowUtilizationThresholdStr, err)
	}

	// Read the profile; default to "gpu" if not specified.
	profile := os.Getenv(EnvProfile)
	if profile == "" {
		profile = gpuProfileName
	}

	return NewMetricsWatcher(
		profile,
		endpoint,
		namespaces,
		pollInterval,
		averagingInterval,
		minAliveForInterval,
		lowUtilizationThreshold,
		k8sClient,
		scheme,
		recorder,
	)
}

// parseIntervalFromEnv retrieves a duration from an environment variable.
func parseIntervalFromEnv(envVar string) (time.Duration, error) {
	interval := os.Getenv(envVar)
	if interval == "" {
		return 0, fmt.Errorf("environment variable %s not set", envVar)
	}
	return time.ParseDuration(interval)
}

// NewMetricsWatcher creates a watcher; check template.Err on parse.
func NewMetricsWatcher(
	profile string,
	endpoint string,
	namespaces []string,
	pollInterval time.Duration,
	averagingInterval time.Duration,
	minAliveForInterval time.Duration,
	lowUtilizationThreshold float64,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
) (*MetricsWatcher, error) {
	tmplSrc := selectTemplate(profile)
	tmpl, err := template.New(profile).Parse(tmplSrc)
	if err != nil {
		return nil, fmt.Errorf("parsing %s template: %w", profile, err)
	}

	data := struct {
		NsFilter string
		Range    string
		MinAge   int
	}{
		NsFilter: buildNsFilter(namespaces),
		Range:    averagingInterval.String(),
		MinAge:   int(minAliveForInterval.Seconds()),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing %s template: %w", profile, err)
	}

	logger := log.Log.WithName(fmt.Sprintf("%sMetricsWatcher", profile))

	return &MetricsWatcher{
		logger:                  logger,
		prometheusEndpoint:      endpoint,
		pollInterval:            pollInterval,
		averagingInterval:       averagingInterval,
		minAliveForInterval:     minAliveForInterval,
		lowUtilizationThreshold: lowUtilizationThreshold,
		query:                   buf.String(),
		profile:                 strings.ToLower(profile),
		statusCache:             map[string]bool{},
		k8sClient:               k8sClient,
		scheme:                  scheme,
		recorder:                recorder,
	}, nil
}

// selectTemplate picks the PromQL template.
func selectTemplate(profile string) string {
	switch strings.ToLower(profile) {
	case "cpu":
		return cpuTemplate
	default:
		return gpuTemplate
	}
}

// buildNsFilter builds the namespace filter clause.
func buildNsFilter(namespaces []string) string {
	switch len(namespaces) {
	case 0:
		// Exclude kube-system namespace by default
		return `, namespace!="kube-system"`
	case 1:
		return fmt.Sprintf(`, namespace="%s"`, namespaces[0])
	default:
		return fmt.Sprintf(`, namespace=~"%s"`, strings.Join(namespaces, "|"))
	}
}

// Start runs the periodic poll loop until ctx.Done().
func (m *MetricsWatcher) Start(ctx context.Context) error {
	m.logger.Info("starting", "query", m.query)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("shutting down")
			return nil
		case <-ticker.C:
			if err := m.pollMetrics(ctx); err != nil {
				m.logger.Error(err, "pollMetrics")
			}
		}
	}
}

// pollMetrics fetches Prometheus data and processes each result.
func (m *MetricsWatcher) pollMetrics(ctx context.Context) error {
	resp, err := QueryPrometheusMetrics(ctx, m.prometheusEndpoint, m.query)
	if err != nil {
		return err
	}
	if len(resp.Data.Result) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	for _, r := range resp.Data.Result {
		if err := m.processResult(ctx, r); err != nil {
			m.logger.Error(err, "processResult", "metric", r.Metric)
		}
		key := m.resultKey(r.Metric)
		seen[key] = struct{}{}
	}

	// remove stale keys
	for key := range m.statusCache {
		if _, ok := seen[key]; !ok {
			delete(m.statusCache, key)
		}
	}
	return nil
}

// processResult translates one PromQL result into a status update + event.
func (m *MetricsWatcher) processResult(ctx context.Context, r PrometheusResult) error {
	percentageUtilization, err := strconv.ParseFloat(r.Value[1].(string), 64)
	if err != nil {
		return fmt.Errorf("parsing %v: %w", r.Value[1], err)
	}
	pod := r.Metric["pod"]
	ns := r.Metric["namespace"]
	gpu := r.Metric["gpu_id"] // empty for CPU
	return m.handlePodUtilization(ctx, ns, pod, gpu, percentageUtilization)
}

// resultKey builds the cache key.
func (m *MetricsWatcher) resultKey(met map[string]string) string {
	if m.profile == gpuProfileName {
		return fmt.Sprintf("%s/%s/%s", met["namespace"], met["pod"], met["gpu_id"])
	}
	return fmt.Sprintf("%s/%s", met["namespace"], met["pod"])
}

// handlePodUtilization updates status + emits event if crossing threshold.
func (m *MetricsWatcher) handlePodUtilization(
	ctx context.Context,
	namespace string,
	podName string,
	gpuID string,
	percentageUtilization float64,
) error {
	key := namespace + "/" + podName
	if gpuID != "" {
		key += "/" + gpuID
	}

	wasHealthy, seen := m.statusCache[key]
	isHealthy := percentageUtilization >= m.lowUtilizationThreshold

	if seen && isHealthy == wasHealthy {
		// no change
		return nil
	}
	m.statusCache[key] = isHealthy

	// decide reason / message / eventType
	var reason v1alpha1.KaiwoResourceUtilizationStatus
	var msg, eventType string
	if isHealthy {
		reason = mapProfile(m.profile, v1alpha1.GpuResourceUtilizationNormal, v1alpha1.CpuResourceUtilizationNormal)
		msg = fmt.Sprintf("%s utilization normal", strings.ToUpper(m.profile))
		eventType = corev1.EventTypeNormal
	} else {
		reason = mapProfile(m.profile, v1alpha1.GpuResourceUtilizationLow, v1alpha1.CpuResourceUtilizationLow)
		msg = fmt.Sprintf("%s under threshold", strings.ToUpper(m.profile))
		eventType = corev1.EventTypeWarning
	}

	kaiwoWorkload, err := GetKaiwoWorkloadFromPod(ctx, m.k8sClient, namespace, podName)
	if err != nil {
		return fmt.Errorf("getting Kaiwo workload: %w", err)
	}
	if kaiwoWorkload == nil {
		// Skip if no workload found from pod
		return nil
	}

	// update status on Kaiwo CR
	if err := m.syncKaiwoStatus(ctx, kaiwoWorkload, reason, msg, isHealthy); err != nil {
		return err
	}

	// record event via EventRecorder
	if err := m.recordEvent(kaiwoWorkload, reason, msg, eventType); err != nil {
		return fmt.Errorf("recording event: %w", err)
	}
	return nil
}

func GetKaiwoWorkloadFromPod(ctx context.Context, k8sClient client.Client, namespace string, podName string) (common.KaiwoWorkload, error) {
	logger := log.FromContext(ctx).WithValues("pod", podName, "namespace", namespace)

	var pod corev1.Pod
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, &pod); err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}
	workloadType, workloadTypeFound := pod.Labels[common.KaiwoTypeLabel]
	workloadName, workloadNameFound := pod.Labels[common.KaiwoNameLabel]
	if !workloadTypeFound || !workloadNameFound || workloadName == "" {
		logger.V(1).Info("not a Kaiwo workload, skipping")
		return nil, nil
	}

	return GetKaiwoWorkload(ctx, k8sClient, workloadName, namespace, workloadType)
}

func GetKaiwoWorkload(ctx context.Context, k8sClient client.Client, name string, namespace string, workloadType string) (common.KaiwoWorkload, error) {
	var obj client.Object
	switch workloadType {
	case "job":
		obj = &v1alpha1.KaiwoJob{}
	case "service":
		obj = &v1alpha1.KaiwoService{}
	default:
		return nil, fmt.Errorf("workload type %s not recognized", workloadType)
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return nil, fmt.Errorf("failed to fetch Kaiwo workload: %w", err)
	}
	return obj.(common.KaiwoWorkload), nil
}

// mapProfile picks GPU or CPU variant.
func mapProfile(profile string, gpuVal v1alpha1.KaiwoResourceUtilizationStatus, cpuVal v1alpha1.KaiwoResourceUtilizationStatus) v1alpha1.KaiwoResourceUtilizationStatus {
	if profile == gpuProfileName {
		return gpuVal
	}
	return cpuVal
}

// syncKaiwoStatus finds the parent Kaiwo resource, applies a condition, and updates status.subresource.
func (m *MetricsWatcher) syncKaiwoStatus(
	ctx context.Context,
	kaiwoWorkload common.KaiwoWorkload,
	reason v1alpha1.KaiwoResourceUtilizationStatus,
	msg string,
	healthy bool,
) error {
	// 3. Set the status condition.
	cond := metav1.Condition{
		Type:               v1alpha1.KaiwoResourceUtilizationType,
		Status:             healthyString(healthy),
		Reason:             string(reason),
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	}

	switch o := kaiwoWorkload.(type) {
	case *v1alpha1.KaiwoJob:
		meta.SetStatusCondition(&o.Status.Conditions, cond)
	case *v1alpha1.KaiwoService:
		meta.SetStatusCondition(&o.Status.Conditions, cond)
	default:
		// this should never happen, but guard just in case
		return fmt.Errorf("unexpected type %T", o)
	}

	// 4. Push status subresource
	if err := m.k8sClient.Status().Update(ctx, kaiwoWorkload.(client.Object)); err != nil {
		return fmt.Errorf("status update: %w", err)
	}
	return nil
}

func healthyString(healthy bool) metav1.ConditionStatus {
	if healthy {
		return metav1.ConditionFalse // False means “not underutilized”
	}
	return metav1.ConditionTrue
}

// recordEvent uses EventRecorder for correct aggregation.
func (m *MetricsWatcher) recordEvent(
	workload common.KaiwoWorkload,
	reason v1alpha1.KaiwoResourceUtilizationStatus,
	msg string,
	eventType string,
) error {
	obj := workload.(client.Object)
	gvk, err := apiutil.GVKForObject(obj, m.scheme)
	if err != nil {
		return fmt.Errorf("could not get GVK for %T: %w", obj, err)
	}

	ref := &corev1.ObjectReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}
	m.recorder.Eventf(ref, eventType, string(reason), "%s", msg)
	return nil
}
