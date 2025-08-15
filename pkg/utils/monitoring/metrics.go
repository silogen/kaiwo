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
	"slices"
	"strconv"
	"time"

	"github.com/silogen/kaiwo/pkg/api"

	"github.com/silogen/kaiwo/pkg/common"

	"github.com/silogen/kaiwo/pkg/config"

	"github.com/silogen/kaiwo/pkg/utilization"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/client-go/kubernetes"

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

// MetricsWatcher polls Prometheus and updates Kaiwo statuses.
type MetricsWatcher struct {
	logger    logr.Logger
	clientset *kubernetes.Clientset
	k8sClient client.Client
	scheme    *runtime.Scheme
	recorder  record.EventRecorder
}

// NewMetricsWatcher creates a watcher; check template.Err on parse.
func NewMetricsWatcher(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
) (*MetricsWatcher, error) {
	logger := log.Log.WithName("MetricsWatcher")
	cfg := ctrl.GetConfigOrDie()
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return &MetricsWatcher{
		logger:    logger,
		k8sClient: k8sClient,
		scheme:    scheme,
		recorder:  recorder,
		clientset: kc,
	}, nil
}

// Start runs the periodic poll loop until ctx.Done().
func (m *MetricsWatcher) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("MetricsWatcher")
	logger.Info("Starting metrics watcher")

	ctx2, err := config.GetContextWithConfig(ctx, m.k8sClient)
	if err != nil {
		return fmt.Errorf("getting config: %w", err)
	}
	cfg := config.ConfigFromContext(ctx2)
	pollInterval := cfg.ResourceMonitoring.GetPollInterval()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping metrics watcher")
			return nil

		case <-time.After(pollInterval):
			if cfg.ResourceMonitoring.Enabled {
				if err := m.pollMetrics(ctx2); err != nil {
					logger.Error(err, "failed to poll metrics")
				}
			}

			// Update polling interval
			ctx2, err = config.GetContextWithConfig(ctx, m.k8sClient)
			if err != nil {
				return fmt.Errorf("getting config: %w", err)
			}
			cfg = config.ConfigFromContext(ctx2)
			pollInterval = cfg.ResourceMonitoring.GetPollInterval()
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
	config := config.ConfigFromContext(ctx)
	if !m.isCollectorAvailable(ctx) {
		return fmt.Errorf("no service '%s' in namespace '%s'", config.ResourceMonitoring.MetricsServiceConfig.Name, config.ResourceMonitoring.MetricsServiceConfig.Namespace)
	}

	logger := log.FromContext(ctx).WithName("MetricsWatcher")
	baseutils.Debug(logger, "Polling for metrics")
	metrics, err := m.scrapeMetrics(ctx)
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

func filterForKaiwoPods(config config.KaiwoConfigContext, metrics *dto.MetricFamily) map[string]GpuMetricsEntry {
	kaiwoPods := make(map[string]GpuMetricsEntry)

	// Find all pods that are underutilizing the GPU
	for _, m := range metrics.Metric {
		labels := parseLabels(m)

		utilizationPercentage := *m.Gauge.Value

		// gpu_gfx_activity{card_model="102-G30211-0C",card_series="AMD Instinct MI300X OAM",card_vendor="AMD",cluster_name="",container="",driver_version="6.12.12",gpu_compute_partition_type="spx",gpu_id="2",gpu_memory_partition_type="nps1",gpu_partition_id="0",gpu_uuid="ccff74a1-0000-1000-80e0-c6d49f5b0d86",hostname="tw038",namespace="",pod="",serial_number="692409005408",vbios_version="022.040.003.043.000001"} 0

		// gpu_gfx_activity{card_model="102-G30211-0C",card_series="AMD Instinct MI300X OAM",card_vendor="AMD",driver_version="6.12.12",gpu_compute_partition_type="cpx",gpu_id="47",gpu_memory_partition_type="nps4",gpu_partition_id="0",gpu_uuid="c3ff74a1-0000-0000-80ff-88426fc31c9e",hostname="tw009",instance="10.42.1.221:5000",job="gpu-exporter",node="tw009",pod="default-metrics-exporter-vvvkm",serial_number="692409005296",vbios_version="022.040.003.043.000001"} 0

		// gpu_gfx_activity{card_model="102-G30211-0C",card_series="AMD Instinct MI300X OAM",card_vendor="AMD",container="workload",driver_version="6.12.12",gpu_compute_partition_type="spx",gpu_id="0",gpu_memory_partition_type="nps1",gpu_partition_id="0",gpu_uuid="3dff74a1-0000-1000-80d3-0c4c1df1df58",hostname="tw038",instance="10.42.0.176:5000",job="gpu-exporter",namespace="default",node="tw038",pod="high-utilization-57ls6",serial_number="692409005328",vbios_version="022.040.003.043.000001"} 100
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

func (m *MetricsWatcher) handleKaiwoWorkloads(ctx context.Context, config config.KaiwoConfigContext, kaiwoWorkloads map[string][]GpuMetricsEntry) error {
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

		if kaiwoWorkload.GetCommonStatusSpec().Status != kaiwo.WorkloadStatusRunning {
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
					string(utilization.GpuResourceUtilizationLow),
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
				utilization.GpuResourceUtilizationLow,
				"One or more GPUs are underutilizing requested resources",
				false,
			)
		} else {
			err = m.syncKaiwoStatus(
				ctx,
				kaiwoWorkload,
				utilization.GpuResourceUtilizationNormal,
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
func (m *MetricsWatcher) scrapeMetrics(ctx context.Context) (map[string]*dto.MetricFamily, error) {
	logger := log.FromContext(ctx)
	config := config.ConfigFromContext(ctx)
	var (
		namespace   = config.ResourceMonitoring.MetricsServiceConfig.Namespace
		serviceName = config.ResourceMonitoring.MetricsServiceConfig.Name
		metricsPort = config.ResourceMonitoring.MetricsServiceConfig.Port
	)

	portStr := strconv.Itoa(int(metricsPort))

	// Build the service-proxy request:
	// GET /api/v1/namespaces/{ns}/services/{service}:{port}/proxy/metrics
	req := m.clientset.CoreV1().RESTClient().
		Get().
		Namespace(namespace).
		Resource("services").
		Name(fmt.Sprintf("%s:%s", serviceName, portStr)).
		SubResource("proxy").
		Suffix("metrics")

	// Execute the request
	data, err := req.Do(ctx).Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to proxy metrics from service %s: %w", serviceName, err)
	}

	// Parse the Prometheus metrics
	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	baseutils.Debug(logger, "Successfully scraped metrics",
		"metric_families", len(metricFamilies))

	return metricFamilies, nil
}

func (m *MetricsWatcher) isCollectorAvailable(ctx context.Context) bool {
	config := config.ConfigFromContext(ctx)

	_, err := m.clientset.CoreV1().Services(config.ResourceMonitoring.MetricsServiceConfig.Namespace).Get(ctx, config.ResourceMonitoring.MetricsServiceConfig.Name, metav1.GetOptions{})
	return err == nil
}

func parseLabels(metric *dto.Metric) map[string]string {
	labelLookup := make(map[string]string)
	for _, label := range metric.Label {
		labelLookup[*label.Name] = *label.Value
	}
	return labelLookup
}

func GetKaiwoWorkload(ctx context.Context, k8sClient client.Client, name string, namespace string, workloadType string) (api.KaiwoWorkload, error) {
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
	return obj.(api.KaiwoWorkload), nil
}

// syncKaiwoStatus updates the condition of the Kaiwo workload
func (m *MetricsWatcher) syncKaiwoStatus(
	ctx context.Context,
	kaiwoWorkload api.KaiwoWorkload,
	reason utilization.ResourceUnderutilizationStatus,
	msg string,
	healthy bool,
) error {
	cond := metav1.Condition{
		Type:               utilization.KaiwoResourceUtilizationType,
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
	workload api.KaiwoWorkload,
) error {
	utilization.SetEarlyTerminationIfLowUtilizationThresholdExceeded(ctx, workload)
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
