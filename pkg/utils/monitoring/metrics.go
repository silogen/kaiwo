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
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/api/errors"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"k8s.io/apimachinery/pkg/api/meta"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Environment variable keys.
const (
	envPrefix = "RESOURCE_MONITORING_"

	EnvMonitoringEnabled = envPrefix + "ENABLED"

	// EnvMetricsEndpoint is the metrics endpoint to use
	EnvMetricsEndpoint = envPrefix + "METRICS_ENDPOINT"

	// EnvPollingInterval is the polling interval to run the GPU metrics poller
	EnvPollingInterval = envPrefix + "POLLING_INTERVAL"
)

const (
	MetricsComponentName = "KaiwoResourceMonitor"
)

func IsMetricsMonitoringEnabled() bool {
	return os.Getenv(EnvMonitoringEnabled) == "true"
}

// MetricsWatcher polls Prometheus and updates Kaiwo statuses.
type MetricsWatcher struct {
	logger logr.Logger

	metricsEndpoint string
	pollInterval    time.Duration

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
	endpoint := os.Getenv(EnvMetricsEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("environment variable %s not set", EnvMetricsEndpoint)
	}

	pollInterval, err := ParseIntervalFromEnv(EnvPollingInterval)
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

// ParseIntervalFromEnv retrieves a duration from an environment variable.
func ParseIntervalFromEnv(envVar string) (time.Duration, error) {
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
		logger:          logger,
		metricsEndpoint: endpoint,
		pollInterval:    pollInterval,
		k8sClient:       k8sClient,
		scheme:          scheme,
		recorder:        recorder,
	}, nil
}

// Start runs the periodic poll loop until ctx.Done().
func (m *MetricsWatcher) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("MetricsWatcher")
	logger.Info("Starting metrics watcher")
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("shutting down")
			return nil
		case <-ticker.C:
			ctx2, err := common.GetContextWithConfig(ctx, m.k8sClient)
			if err != nil {
				m.logger.Error(err, "failed to get Kaiwo config")
				continue
			}
			if err := m.pollMetrics(ctx2); err != nil {
				m.logger.Error(err, "pollMetrics")
			}
		}
	}
}

type GpuMetricsEntry struct {
	Namespace             string
	PodName               string
	ContainerName         string
	GpuID                 string
	GpuPartitionID        string
	UtilizationPercentage float64
	AssociatedPod         *corev1.Pod
}

func (entry *GpuMetricsEntry) Key() string {
	return fmt.Sprintf("%s/%s", entry.Namespace, entry.PodName)
}

func (entry *GpuMetricsEntry) IsValid() bool {
	return entry.Namespace != "" && entry.PodName != "" && entry.GpuID != "" && entry.GpuPartitionID != "" && entry.ContainerName != ""
}

// pollMetrics fetches Prometheus data and processes each result.
func (m *MetricsWatcher) pollMetrics(ctx context.Context) error {
	config := common.ConfigFromContext(ctx)
	logger := log.FromContext(ctx).WithName("MetricsWatcher")
	baseutils.Debug(logger, "Polling for metrics")
	metrics, err := scrapeMetrics(m.metricsEndpoint)
	if err != nil {
		return fmt.Errorf("failed to scrape metrics: %v", err)
	}

	kaiwoPods := filterForKaiwoPods(config, metrics["gpu_gfx_activity"])
	kaiwoWorkloads, err := fetchKaiwoWorkloads(ctx, m.k8sClient, kaiwoPods)
	if err != nil {
		return fmt.Errorf("failed to fetch Kaiwo workloads: %v", err)
	}
	if err := m.handleKaiwoWorkloads(ctx, config, kaiwoWorkloads); err != nil {
		return fmt.Errorf("failed to handle Kaiwo workloads: %v", err)
	}
	return nil
}

func filterForKaiwoPods(config common.KaiwoConfigContext, metrics *dto.MetricFamily) map[string]GpuMetricsEntry {
	kaiwoPods := make(map[string]GpuMetricsEntry)

	// Find all pods that are underutilizing the GPU
	for _, m := range metrics.Metric {
		labels := parseLabels(m)

		utilizationPercentage := *m.Gauge.Value

		entry := GpuMetricsEntry{
			Namespace:             labels["namespace"],
			PodName:               labels["pod"],
			ContainerName:         labels["container"],
			GpuID:                 labels["gpu_id"],
			GpuPartitionID:        labels["gpu_partition_id"],
			UtilizationPercentage: utilizationPercentage,
		}

		// Skip if not a valid entry
		if !entry.IsValid() {
			continue
		}

		// Skip if namespace not applicable
		if targetNamespaces := config.ResourceMonitoring.TargetNamespaces; len(targetNamespaces) > 0 && !slices.Contains(targetNamespaces, entry.Namespace) {
			continue
		}

		// Add entry to lookup
		kaiwoPods[entry.Key()] = entry
	}
	return kaiwoPods
}

func fetchKaiwoWorkloads(ctx context.Context, k8sClient client.Client, kaiwoPods map[string]GpuMetricsEntry) (map[string][]GpuMetricsEntry, error) {
	logger := log.FromContext(ctx)
	kaiwoWorkloads := map[string][]GpuMetricsEntry{}
	for key := range kaiwoPods {
		entry := kaiwoPods[key]
		pod := &corev1.Pod{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: entry.Namespace, Name: entry.PodName}, pod); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Pod not found", "namespace", entry.Namespace, "name", entry.PodName)
				continue
			} else {
				return nil, fmt.Errorf("failed to get pod %s/%s: %v", entry.Namespace, entry.PodName, err)
			}
		}

		// Skip if not a Kaiwo workload pod
		kaiwoWorkloadId, isKaiwoWorkload := pod.Labels[common.KaiwoRunIdLabel]
		if !isKaiwoWorkload {
			continue
		}

		entry.AssociatedPod = pod
		if _, ok := kaiwoWorkloads[kaiwoWorkloadId]; !ok {
			kaiwoWorkloads[kaiwoWorkloadId] = []GpuMetricsEntry{}
		}
		kaiwoWorkloads[kaiwoWorkloadId] = append(kaiwoWorkloads[key], entry)
	}
	return kaiwoWorkloads, nil
}

func (m *MetricsWatcher) handleKaiwoWorkloads(ctx context.Context, config common.KaiwoConfigContext, kaiwoWorkloads map[string][]GpuMetricsEntry) error {
	for kaiwoWorkloadId, entries := range kaiwoWorkloads {
		pod := kaiwoWorkloads[kaiwoWorkloadId][0].AssociatedPod
		kaiwoWorkloadType := pod.Labels[common.KaiwoTypeLabel]
		kaiwoWorkloadName := pod.Labels[common.KaiwoNameLabel]

		kaiwoWorkload, err := GetKaiwoWorkload(ctx, m.k8sClient, kaiwoWorkloadName, pod.Namespace, kaiwoWorkloadType)
		if err != nil {
			if errors.IsNotFound(err) {
				m.logger.Info("failed to get Kaiwo workload", "workloadName", kaiwoWorkloadName)
				continue
			} else {
				return fmt.Errorf("failed to get Kaiwo workload %s: %v", kaiwoWorkloadName, err)
			}
		}

		switch kaiwoWorkload.GetCommonStatusSpec().Status {
		case kaiwo.WorkloadStatusTerminating, kaiwo.WorkloadStatusTerminated, kaiwo.WorkloadStatusFailed, kaiwo.WorkloadStatusComplete:
			continue
		}

		obj := kaiwoWorkload.GetKaiwoWorkloadObject()

		anyUnderutilizing := false

		for _, entry := range entries {
			if entry.UtilizationPercentage < config.ResourceMonitoring.LowUtilizationThreshold {
				anyUnderutilizing = true
				m.recorder.Eventf(
					obj,
					corev1.EventTypeWarning,
					string(common.GpuResourceUtilizationLow),
					"Pod '%s' is underutilizing GPU %s/%s",
					entry.PodName,
					entry.GpuID,
					entry.GpuPartitionID,
				)
			}
		}

		if anyUnderutilizing {
			err = m.syncKaiwoStatus(
				ctx,
				kaiwoWorkload,
				common.GpuResourceUtilizationLow,
				"One or more GPUs are underutilizing requested resources",
				false,
			)
		} else {
			err = m.syncKaiwoStatus(
				ctx,
				kaiwoWorkload,
				common.GpuResourceUtilizationNormal,
				"GPU utilization is normal",
				true,
			)
		}
		if err != nil {
			return fmt.Errorf("failed to sync Kaiwo workload status: %v", err)
		}
		if config.ResourceMonitoring.TerminateUnderutilized {
			if err := m.flagIfUnderutilized(ctx, kaiwoWorkload); err != nil {
				return fmt.Errorf("failed to flag underutilized Kaiwo workload status: %v", err)
			}
		}
	}
	return nil
}

// scrapeMetrics scrapes the given URL, parses all families,
// and returns a map from metric name → MetricFamily.
func scrapeMetrics(url string) (map[string]*dto.MetricFamily, error) {
	httpClient := http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)

	// Prometheus text or OpenMetrics parsing
	parser := expfmt.TextParser{}
	mf, err := parser.TextToMetricFamilies(resp.Body)
	return mf, err
}

func parseLabels(metric *dto.Metric) map[string]string {
	labelLookup := make(map[string]string)
	for _, label := range metric.Label {
		labelLookup[*label.Name] = *label.Value
	}
	return labelLookup
}

func GetKaiwoWorkload(ctx context.Context, k8sClient client.Client, name string, namespace string, workloadType string) (common.KaiwoWorkload, error) {
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
	return obj.(common.KaiwoWorkload), nil
}

// syncKaiwoStatus updates the condition of the Kaiwo workload
func (m *MetricsWatcher) syncKaiwoStatus(
	ctx context.Context,
	kaiwoWorkload common.KaiwoWorkload,
	reason common.ResourceUnderutilizationStatus,
	msg string,
	healthy bool,
) error {
	cond := metav1.Condition{
		Type:               common.KaiwoResourceUtilizationType,
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

	if err := m.k8sClient.Status().Update(ctx, kaiwoWorkload.(client.Object)); err != nil {
		return fmt.Errorf("status update: %w", err)
	}
	return nil
}

func (m *MetricsWatcher) flagIfUnderutilized(
	ctx context.Context,
	workload common.KaiwoWorkload,
) error {
	common.SetEarlyTerminationIfLowUtilizationThresholdExceeded(ctx, workload)
	obj := workload.GetKaiwoWorkloadObject()
	if err := m.k8sClient.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}

func healthyString(healthy bool) metav1.ConditionStatus {
	if healthy {
		return metav1.ConditionFalse // False means “not underutilized”
	}
	return metav1.ConditionTrue
}
