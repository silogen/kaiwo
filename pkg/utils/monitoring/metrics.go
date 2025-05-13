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
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/silogen/kaiwo/apis/kaiwo/utils"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	configapi "github.com/silogen/kaiwo/apis/config/v1alpha1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

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

	prometheusEndpoint string
	pollInterval       time.Duration

	previousConfig *configapi.KaiwoResourceMonitoringConfig
	statusCache    map[string]bool

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

	pollInterval, err := parseIntervalFromEnv(EnvPollingInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse poll interval: %v", err)
	}

	return NewMetricsWatcher(
		endpoint,
		pollInterval,
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
	endpoint string,
	pollInterval time.Duration,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
) (*MetricsWatcher, error) {
	logger := log.Log.WithName("MetricsWatcher")
	return &MetricsWatcher{
		logger:             logger,
		prometheusEndpoint: endpoint,
		pollInterval:       pollInterval,
		statusCache:        map[string]bool{},
		k8sClient:          k8sClient,
		scheme:             scheme,
		recorder:           recorder,
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
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("shutting down")
			return nil
		case <-ticker.C:
			ctx, err := controllerutils.GetContextWithConfig(ctx, m.k8sClient)
			if err != nil {
				m.logger.Error(err, "failed to get Kaiwo config")
				continue
			}
			if err := m.pollMetrics(ctx); err != nil {
				m.logger.Error(err, "pollMetrics")
			}
		}
	}
}

// pollMetrics fetches Prometheus data and processes each result.
func (m *MetricsWatcher) pollMetrics(ctx context.Context) error {
	config := controllerutils.ConfigFromContext(ctx)

	if m.previousConfig != nil && !reflect.DeepEqual(m.previousConfig, &config.ResourceMonitoring) {
		// Clear cache if the configuration has changed
		m.statusCache = map[string]bool{}
	}
	m.previousConfig = &config.ResourceMonitoring

	profile := config.ResourceMonitoring.Profile
	tmplSrc := selectTemplate(profile)
	tmpl, err := template.New(profile).Parse(tmplSrc)
	if err != nil {
		return fmt.Errorf("parsing %s template: %w", profile, err)
	}

	averagingTime, err := time.ParseDuration(config.ResourceMonitoring.AveragingTime)
	if err != nil {
		return fmt.Errorf("parsing duration: %s : %w", config.ResourceMonitoring.AveragingTime, err)
	}
	minAge, err := time.ParseDuration(config.ResourceMonitoring.MinAliveTime)
	if err != nil {
		return fmt.Errorf("parsing duration: %s : %w", config.ResourceMonitoring.MinAliveTime, err)
	}

	data := struct {
		NsFilter string
		Range    string
		MinAge   int
	}{
		NsFilter: buildNsFilter(config.ResourceMonitoring.TargetNamespaces),
		Range:    averagingTime.String(),
		MinAge:   int(minAge.Seconds()),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing %s template: %w", profile, err)
	}

	query := buf.String()

	resp, err := QueryPrometheusMetrics(ctx, m.prometheusEndpoint, query)
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
		key := m.resultKey(r.Metric, profile)
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
func (m *MetricsWatcher) resultKey(met map[string]string, profile string) string {
	if profile == gpuProfileName {
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
	config := controllerutils.ConfigFromContext(ctx)
	profile := config.ResourceMonitoring.Profile

	key := namespace + "/" + podName
	if gpuID != "" {
		key += "/" + gpuID
	}

	wasHealthy, seen := m.statusCache[key]
	isHealthy := percentageUtilization >= config.ResourceMonitoring.LowUtilizationThreshold

	if seen && isHealthy == wasHealthy {
		// no change
		return nil
	}
	m.statusCache[key] = isHealthy

	// decide reason / message / eventType
	var reason kaiwo.KaiwoResourceUtilizationStatus
	var msg, eventType string
	if isHealthy {
		reason = mapProfile(profile, kaiwo.GpuResourceUtilizationNormal, kaiwo.CpuResourceUtilizationNormal)
		msg = fmt.Sprintf("%s utilization normal", strings.ToUpper(profile))
		eventType = corev1.EventTypeNormal
	} else {
		reason = mapProfile(profile, kaiwo.GpuResourceUtilizationLow, kaiwo.CpuResourceUtilizationLow)
		msg = fmt.Sprintf("%s under threshold", strings.ToUpper(profile))
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

func GetKaiwoWorkloadFromPod(ctx context.Context, k8sClient client.Client, namespace string, podName string) (utils.KaiwoWorkload, error) {
	logger := log.FromContext(ctx).WithValues("pod", podName, "namespace", namespace)

	var pod corev1.Pod
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, &pod); err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}
	workloadType, workloadTypeFound := pod.Labels[kaiwo.KaiwoTypeLabel]
	workloadName, workloadNameFound := pod.Labels[kaiwo.KaiwoNameLabel]
	if !workloadTypeFound || !workloadNameFound || workloadName == "" {
		logger.V(1).Info("not a Kaiwo workload, skipping")
		return nil, nil
	}

	return GetKaiwoWorkload(ctx, k8sClient, workloadName, namespace, workloadType)
}

func GetKaiwoWorkload(ctx context.Context, k8sClient client.Client, name string, namespace string, workloadType string) (utils.KaiwoWorkload, error) {
	var obj client.Object
	switch workloadType {
	case "job":
		obj = &kaiwo.KaiwoJob{}
	case "service":
		obj = &kaiwo.KaiwoService{}
	default:
		return nil, fmt.Errorf("workload type %s not recognized", workloadType)
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return nil, fmt.Errorf("failed to fetch Kaiwo workload: %w", err)
	}
	return obj.(utils.KaiwoWorkload), nil
}

// mapProfile picks GPU or CPU variant.
func mapProfile(profile string, gpuVal kaiwo.KaiwoResourceUtilizationStatus, cpuVal kaiwo.KaiwoResourceUtilizationStatus) kaiwo.KaiwoResourceUtilizationStatus {
	if profile == gpuProfileName {
		return gpuVal
	}
	return cpuVal
}

// syncKaiwoStatus finds the parent Kaiwo resource, applies a condition, and updates status.subresource.
func (m *MetricsWatcher) syncKaiwoStatus(
	ctx context.Context,
	kaiwoWorkload utils.KaiwoWorkload,
	reason kaiwo.KaiwoResourceUtilizationStatus,
	msg string,
	healthy bool,
) error {
	// 3. Set the status condition.
	cond := metav1.Condition{
		Type:               kaiwo.KaiwoResourceUtilizationType,
		Status:             healthyString(healthy),
		Reason:             string(reason),
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	}

	switch o := kaiwoWorkload.(type) {
	case *kaiwo.KaiwoJob:
		meta.SetStatusCondition(&o.Status.Conditions, cond)
	case *kaiwo.KaiwoService:
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
	workload utils.KaiwoWorkload,
	reason kaiwo.KaiwoResourceUtilizationStatus,
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
