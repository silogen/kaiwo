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
	corev1 "k8s.io/api/core/v1"
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

	// Group samples by namespace/podName
	type podKey struct {
		namespace string
		podName   string
	}
	samplesByPod := make(map[podKey][]gpuSample)
	for _, sample := range samples {
		key := podKey{namespace: sample.namespace, podName: sample.podName}
		samplesByPod[key] = append(samplesByPod[key], sample)
	}

	// Look up which pods are tracked by GpuWorkloads and update accordingly
	var allGW kaiwo.GpuWorkloadList
	if err := s.client.List(ctx, &allGW); err != nil {
		return fmt.Errorf("failed to list GpuWorkloads: %w", err)
	}

	activeGW := 0
	gwByOwner := make(map[gwLookupKey]*kaiwo.GpuWorkload)
	for i := range allGW.Items {
		gw := &allGW.Items[i]
		if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempted || gw.Status.Phase == kaiwo.GpuWorkloadPhaseDeleted {
			continue
		}
		k := gwLookupKey{namespace: gw.Namespace, uid: string(gw.Spec.WorkloadRef.UID)}
		gwByOwner[k] = gw
		activeGW++
	}

	// For each pod with metrics, find its owner pod and match to a GpuWorkload
	matched := 0
	updated := 0
	for pk, podSamples := range samplesByPod {
		pod := &corev1.Pod{}
		if err := s.client.Get(ctx, client.ObjectKey{Namespace: pk.namespace, Name: pk.podName}, pod); err != nil {
			logger.V(1).Info("pod from metrics not found in cluster", "namespace", pk.namespace, "pod", pk.podName)
			continue
		}

		// Walk owner refs to find the root owner UID, then match
		gw := s.findGpuWorkloadForPod(ctx, pod, gwByOwner)
		if gw == nil {
			logger.V(1).Info("pod has GPU metrics but no matching GpuWorkload", "namespace", pk.namespace, "pod", pk.podName)
			continue
		}
		matched++
		if s.updatePodUtilizations(ctx, gw, pk.podName, podSamples) {
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

type gwLookupKey struct {
	namespace string
	uid       string
}

func (s *GpuMetricsScraper) findGpuWorkloadForPod(ctx context.Context, pod *corev1.Pod, gwByOwner map[gwLookupKey]*kaiwo.GpuWorkload) *kaiwo.GpuWorkload {
	rootResult, err := ResolveRootOwner(ctx, s.client, pod.Namespace, pod.Name, "Pod", "v1", pod.UID)
	if err != nil {
		return nil
	}
	key := gwLookupKey{namespace: pod.Namespace, uid: string(rootResult.Ref.UID)}
	return gwByOwner[key]
}

func (s *GpuMetricsScraper) applyUtilizationSamples(gw *kaiwo.GpuWorkload, podName string, samples []gpuSample, now metav1.Time) bool {
	changed := false
	for _, sample := range samples {
		rounded := math.Round(sample.utilization*utilizationPrecision) / utilizationPrecision
		found := false
		for i := range gw.Status.PodUtilizations {
			pu := &gw.Status.PodUtilizations[i]
			if pu.PodName == podName && pu.GpuID == sample.gpuID {
				if math.Abs(pu.Utilization-rounded) > utilizationEpsilon {
					pu.Utilization = rounded
					pu.LastUpdate = now
					changed = true
				}
				found = true
				break
			}
		}
		if !found {
			gw.Status.PodUtilizations = append(gw.Status.PodUtilizations, kaiwo.PodGpuUtilization{
				PodName:     podName,
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

func (s *GpuMetricsScraper) updatePodUtilizations(ctx context.Context, gw *kaiwo.GpuWorkload, podName string, samples []gpuSample) bool {
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

func parseGpuSamples(family *dto.MetricFamily) []gpuSample {
	var samples []gpuSample
	for _, m := range family.Metric {
		labels := make(map[string]string)
		for _, lp := range m.Label {
			if lp.Name != nil && lp.Value != nil {
				labels[*lp.Name] = *lp.Value
			}
		}

		ns := labels["namespace"]
		pod := labels["pod"]
		gpuID := labels["gpu_id"]

		if ns == "" || pod == "" {
			continue
		}

		var util float64
		if m.Gauge != nil && m.Gauge.Value != nil {
			util = *m.Gauge.Value
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
