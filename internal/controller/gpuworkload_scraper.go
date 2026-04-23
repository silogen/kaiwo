/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

const (
	EnvMetricsEndpoint     = EnvGpuPreemptionPrefix + "METRICS_ENDPOINT"
	EnvPollingInterval     = EnvGpuPreemptionPrefix + "POLLING_INTERVAL"
	gpuActivityMetric      = "gpu_gfx_activity"
	scraperHTTPTimeout     = 5 * time.Second
	utilizationEpsilon     = 0.01
	utilizationPrecision   = 100
	defaultPollingInterval = "15s"
)

// GpuMetricsScraper is a manager.Runnable that periodically scrapes AMD GPU
// metrics and writes per-pod utilization data into GpuWorkload CR statuses.
type GpuMetricsScraper struct {
	client          client.Client
	metricsEndpoint string
	pollInterval    time.Duration
}

// NewGpuMetricsScraper creates a scraper from environment variables.
func NewGpuMetricsScraper(c client.Client) (*GpuMetricsScraper, error) {
	endpoint := os.Getenv(EnvMetricsEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("environment variable %s is required when GPU preemption is enabled", EnvMetricsEndpoint)
	}

	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%s must use http or https scheme, got %q", EnvMetricsEndpoint, endpoint)
	}

	intervalStr := os.Getenv(EnvPollingInterval)
	if intervalStr == "" {
		intervalStr = defaultPollingInterval
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s=%q: %w", EnvPollingInterval, intervalStr, err)
	}

	return &GpuMetricsScraper{
		client:          c,
		metricsEndpoint: endpoint,
		pollInterval:    interval,
	}, nil
}

// Start implements manager.Runnable.
func (s *GpuMetricsScraper) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("GpuMetricsScraper")
	logger.Info("starting GPU metrics scraper", "endpoint", s.metricsEndpoint, "interval", s.pollInterval)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down GPU metrics scraper")
			return nil
		case <-ticker.C:
			if err := s.poll(ctx); err != nil {
				logger.Error(err, "scrape cycle failed")
			}
		}
	}
}

type gpuSample struct {
	namespace   string
	podName     string
	gpuID       string
	utilization float64
}

func (s *GpuMetricsScraper) poll(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("GpuMetricsScraper")

	metrics, err := s.scrape()
	if err != nil {
		return fmt.Errorf("failed to scrape metrics endpoint: %w", err)
	}

	family, ok := metrics[gpuActivityMetric]
	if !ok {
		logger.Info("scrape completed: no gpu_gfx_activity metric in response", "metricsReturned", len(metrics))
		return nil
	}

	samples := parseGpuSamples(family)
	if len(samples) == 0 {
		logger.Info("scrape completed: metric found but no valid samples", "metric", gpuActivityMetric)
		return nil
	}

	type podKey struct {
		namespace string
		podName   string
	}
	samplesByPod := make(map[podKey][]gpuSample)
	for _, sample := range samples {
		key := podKey{namespace: sample.namespace, podName: sample.podName}
		samplesByPod[key] = append(samplesByPod[key], sample)
	}

	var allGW kaiwo.GpuWorkloadList
	if err := s.client.List(ctx, &allGW); err != nil {
		return fmt.Errorf("failed to list GpuWorkloads: %w", err)
	}

	// Build a reverse map from (namespace, podName) → GpuWorkload using the
	// TrackedPods list that the reconciler maintains in each GpuWorkload status.
	gwByPod := make(map[podKey]*kaiwo.GpuWorkload)
	activeGW := 0
	for i := range allGW.Items {
		gw := &allGW.Items[i]
		if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempted || gw.Status.Phase == kaiwo.GpuWorkloadPhaseDeleted {
			continue
		}
		activeGW++
		for _, tp := range gw.Status.TrackedPods {
			gwByPod[podKey{namespace: gw.Namespace, podName: tp.PodName}] = gw
		}
	}

	matched := 0
	updated := 0
	for pk, podSamples := range samplesByPod {
		gw, ok := gwByPod[pk]
		if !ok {
			continue
		}
		matched++
		if s.updatePodMetrics(ctx, gw, pk.podName, podSamples) {
			updated++
		}
	}

	logger.Info("scrape completed",
		"gpuSamples", len(samples),
		"pods", len(samplesByPod),
		"trackedGpuWorkloads", activeGW,
		"podsMatched", matched,
		"gpuWorkloadsUpdated", updated,
	)

	return nil
}

func (s *GpuMetricsScraper) applyUtilizationSamples(gw *kaiwo.GpuWorkload, podName string, samples []gpuSample, now metav1.Time) bool {
	var tp *kaiwo.TrackedPod
	for i := range gw.Status.TrackedPods {
		if gw.Status.TrackedPods[i].PodName == podName {
			tp = &gw.Status.TrackedPods[i]
			break
		}
	}
	if tp == nil {
		return false
	}

	changed := false
	for _, sample := range samples {
		rounded := math.Round(sample.utilization*utilizationPrecision) / utilizationPrecision
		found := false
		for i := range tp.GpuMetrics {
			gm := &tp.GpuMetrics[i]
			if gm.GpuID == sample.gpuID {
				if math.Abs(gm.Utilization-rounded) > utilizationEpsilon {
					gm.Utilization = rounded
					gm.LastUpdate = now
					changed = true
				}
				found = true
				break
			}
		}
		if !found {
			tp.GpuMetrics = append(tp.GpuMetrics, kaiwo.GpuMetric{
				GpuID:       sample.gpuID,
				Utilization: rounded,
				LastUpdate:  now,
			})
			changed = true
		}
	}
	if changed {
		gw.Status.LastMetricsUpdate = &now
	}
	return changed
}

func (s *GpuMetricsScraper) updatePodMetrics(ctx context.Context, gw *kaiwo.GpuWorkload, podName string, samples []gpuSample) bool {
	logger := log.FromContext(ctx).WithName("GpuMetricsScraper")
	now := metav1.Now()

	if !s.applyUtilizationSamples(gw, podName, samples, now) {
		return false
	}

	err := s.client.Status().Update(ctx, gw)
	if err == nil {
		return true
	}

	if !apierrors.IsConflict(err) {
		logger.V(1).Info("failed to update pod utilizations", "gpuworkload", gw.Name, "error", err)
		return false
	}

	// Re-fetch and retry once on conflict.
	fresh := &kaiwo.GpuWorkload{}
	if err := s.client.Get(ctx, client.ObjectKeyFromObject(gw), fresh); err != nil {
		logger.V(1).Info("failed to re-fetch GpuWorkload after conflict", "gpuworkload", gw.Name, "error", err)
		return false
	}
	if !s.applyUtilizationSamples(fresh, podName, samples, now) {
		return false
	}
	if err := s.client.Status().Update(ctx, fresh); err != nil {
		logger.V(1).Info("failed to update pod utilizations after retry", "gpuworkload", gw.Name, "error", err)
		return false
	}
	return true
}

func (s *GpuMetricsScraper) scrape() (map[string]*dto.MetricFamily, error) {
	httpClient := http.Client{Timeout: scraperHTTPTimeout}
	resp, err := httpClient.Get(s.metricsEndpoint)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metrics endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	parser := expfmt.TextParser{}
	return parser.TextToMetricFamilies(resp.Body)
}

// prometheusSampleValue returns the scalar sample value for metrics that may be
// encoded as gauge or untyped. Prometheus text without a # TYPE line is parsed
// as UNTYPED; reading only Gauge left utilization stuck at zero for those exporters.
func prometheusSampleValue(m *dto.Metric) (float64, bool) {
	if m == nil {
		return 0, false
	}
	if g := m.GetGauge(); g != nil && g.Value != nil {
		return *g.Value, true
	}
	if u := m.GetUntyped(); u != nil && u.Value != nil {
		return *u.Value, true
	}
	return 0, false
}

func firstNonEmpty(labels map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := labels[k]; v != "" {
			return v
		}
	}
	return ""
}

func parseGpuSamples(family *dto.MetricFamily) []gpuSample {
	var samples []gpuSample
	for _, m := range family.Metric {
		labels := make(map[string]string)
		for _, lp := range m.Label {
			if lp.Name != nil && lp.Value != nil {
				labels[*lp.Name] = *lp.Value
			}
		}

		ns := firstNonEmpty(labels,
			"namespace",
			"kubernetes_namespace",
			"k8s_namespace_name",
			"k8s.namespace.name",
		)
		pod := firstNonEmpty(labels,
			"pod",
			"pod_name",
			"kubernetes_pod_name",
			"k8s_pod_name",
			"k8s.pod.name",
		)
		gpuID := firstNonEmpty(labels, "gpu_id", "gpu")

		if ns == "" || pod == "" {
			continue
		}

		util, ok := prometheusSampleValue(m)
		if !ok {
			continue
		}

		samples = append(samples, gpuSample{
			namespace:   ns,
			podName:     pod,
			gpuID:       gpuID,
			utilization: util,
		})
	}
	return samples
}
