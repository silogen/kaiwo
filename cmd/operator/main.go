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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	controllerutils "github.com/silogen/kaiwo/pkg/workloads/common"

	"github.com/go-logr/logr"

	"github.com/go-logr/zapr"

	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"sigs.k8s.io/controller-runtime/pkg/client"

	configapi "github.com/silogen/kaiwo/apis/config/v1alpha1"
	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/silogen/kaiwo/pkg/utils/monitoring"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/silogen/kaiwo/internal/controller"

	webhookv1 "github.com/silogen/kaiwo/internal/webhook/v1"

	// +kubebuilder:scaffold:imports

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	aimcontroller "github.com/silogen/kaiwo/internal/controller/aim"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var certDirectory = baseutils.GetEnv("WEBHOOK_CERT_DIRECTORY", "")

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kaiwo.AddToScheme(scheme))
	utilruntime.Must(configapi.AddToScheme(scheme))
	utilruntime.Must(aimv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	utilruntime.Must(kueuev1beta1.AddToScheme(scheme))
	utilruntime.Must(kueuev1alpha1.AddToScheme(scheme))
	utilruntime.Must(rayv1.AddToScheme(scheme))
	utilruntime.Must(appwrapperv1beta2.AddToScheme(scheme))
	utilruntime.Must(servingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(servingv1beta1.AddToScheme(scheme))
	if err := gatewayapiv1.Install(scheme); err != nil {
		panic(err)
	}
}

func setupFormattedLogOutput() logr.Logger {
	cfg := uberzap.NewProductionConfig()
	cfg.Encoding = "console"
	// cfg.Level = uberzap.NewAtomicLevelAt(uberzap.DebugLevel)
	cfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:  "ts",
		LevelKey: "level",
		NameKey:  "logger",
		// CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // <-- colored levels
		EncodeTime:     zapcore.ISO8601TimeEncoder,       // human-readable timestamps
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	rawLogger, err := cfg.Build(uberzap.AddCaller())
	if err != nil {
		panic(err)
	}
	defer func() { _ = rawLogger.Sync() }()

	return zapr.NewLogger(rawLogger)
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if os.Getenv("FORMATTED_LOGS") == "true" {
		ctrl.SetLogger(setupFormattedLogOutput())
	} else {
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	webhooksEnabled := os.Getenv("DISABLE_WEBHOOKS") != "true"
	if !webhooksEnabled {
		setupLog.Info("webhooks disabled")
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 && webhooksEnabled {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	managerOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "83fc2279.silogen.ai",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	if webhooksEnabled {
		if certDirectory != "" {
			managerOptions.WebhookServer = webhook.NewServer(webhook.Options{
				CertDir: certDirectory,
			})
		} else {
			managerOptions.WebhookServer = webhook.NewServer(webhook.Options{
				TLSOpts: webhookTLSOpts,
			})
		}
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.KaiwoJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KaiwoJob")
		os.Exit(1)
	}
	if err = (&controller.KaiwoServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KaiwoService")
		os.Exit(1)
	}
	if err = (&controller.KaiwoQueueConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KaiwoQueueConfig")
		os.Exit(1)
	}

	// Setup AIM controllers
	if err = aimcontroller.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup AIM controllers")
		os.Exit(1)
	}

	if webhooksEnabled {
		decoder := admission.NewDecoder(scheme)

		jobWebhook := &webhookv1.JobWebhook{}
		if err = jobWebhook.InjectDecoder(&decoder); err != nil {
			setupLog.Error(err, "unable to inject decoder to webhook")
			os.Exit(1)
		}
		if err = jobWebhook.SetupJobWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Unable to create webhook", "webhook", "Job")
			os.Exit(1)
		}

		if err = webhookv1.SetupRayJobWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "RayJob")
			os.Exit(1)
		}
		if err = webhookv1.SetupRayServiceWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "RayService")
			os.Exit(1)
		}

		if err = webhookv1.SetupDeploymentWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Deployment")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil && webhooksEnabled {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if monitoring.IsMetricsMonitoringEnabled() {
		setupLog.Info("Adding metrics watcher")
		metricsWatcher, err := monitoring.NewMetricsWatcherFromEnv(
			mgr.GetClient(),
			mgr.GetScheme(),
			mgr.GetEventRecorderFor(monitoring.MetricsComponentName),
		)
		if err != nil {
			setupLog.Error(err, "unable to create metrics watcher")
			os.Exit(1)
		}

		if err := mgr.Add(metricsWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics watcher to manager")
			os.Exit(1)
		}
	} else {
		setupLog.Info("Metrics watcher is not enabled")
	}

	// Ensure that the config exists
	if err := ensureConfigExists(mgr); err != nil {
		setupLog.Error(err, "unable to ensure config exists")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func ensureConfigExists(mgr ctrl.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := ctrl.Log.WithName("configFetcher")
	ctx = ctrl.LoggerInto(ctx, logger)

	apiReader, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	if err := controllerutils.EnsureConfig(ctx, apiReader); err != nil {
		return fmt.Errorf("unable to ensure config exists: %w", err)
	}
	return nil
}
