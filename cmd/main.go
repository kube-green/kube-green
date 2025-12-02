/*
Copyright 2021.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"

	kubegreencomv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	apiv1 "github.com/kube-green/kube-green/internal/api/v1"
	sleepinfocontroller "github.com/kube-green/kube-green/internal/controller/sleepinfo"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/metrics"
	webhookv1alpha1 "github.com/kube-green/kube-green/internal/webhook/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	managerName = "kube-green"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubegreencomv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var webhookHost string
	var webhookPort int
	var metricsAddr string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var sleepDelta int64
	var secureMetrics bool
	var enableHTTP2 bool
	var maxConcurrentReconciles int
	var apiPort int
	var enableAPI bool
	var enableAPICORS bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&webhookHost, "webhook-host", "", "The host where the server binds to. Default means all interfaces.")
	flag.IntVar(&webhookPort, "webhook-server-port", 9443, "The port where the server will listen.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metric endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.Int64Var(&sleepDelta, "sleep-delta", 60,
		"The delta in seconds between the cronjob schedule and when the job is being processed before skipping it")
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
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 20,
		"Max concurrent schedules that will be processed at the same time.")
	flag.IntVar(&apiPort, "api-port", 8080, "The port where the REST API server will listen.")
	flag.BoolVar(&enableAPI, "enable-api", false, "Enable the REST API server.")
	flag.BoolVar(&enableAPICORS, "enable-api-cors", false, "Enable CORS for the REST API server.")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
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

	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
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

	webhookServer := webhook.NewServer(webhook.Options{
		Host:    webhookHost,
		TLSOpts: webhookTLSOpts,
		Port:    webhookPort,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/server
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
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

		// If the certificate is not specified, controller-runtime will automatically
		// generate self-signed certificates for the metrics server. While convenient for development and testing,
		// this setup is not recommended for production.
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
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "2bd226ed.kube-green.com",
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&v1.Secret{}},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	customMetrics := metrics.SetupMetricsOrDie("kube_green").MustRegister(ctrlMetrics.Registry)

	if err = (&sleepinfocontroller.SleepInfoReconciler{
		Client:                  mgr.GetClient(),
		Log:                     ctrl.Log.WithName("controllers").WithName("SleepInfo"),
		Scheme:                  mgr.GetScheme(),
		Metrics:                 customMetrics,
		SleepDelta:              sleepDelta,
		ManagerName:             managerName,
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SleepInfo")
		os.Exit(1)
	}
	if err = webhookv1alpha1.SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "SleepInfo")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Start REST API server if enabled
	ctx := ctrl.SetupSignalHandler()
	if enableAPI {
		// Get namespace from environment or use default
		namespace := os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = "keos-core" // Default namespace
		}

		apiServer := apiv1.NewServer(apiv1.Config{
			Port:       apiPort,
			Client:     mgr.GetClient(),
			Logger:     ctrl.Log.WithName("api"),
			EnableCORS: enableAPICORS,
			Namespace:  namespace,
		})

		// Add API server as a runnable to the manager
		if err := mgr.Add(&runnableServer{
			server: apiServer,
			ctx:    ctx,
		}); err != nil {
			setupLog.Error(err, "unable to add REST API server to manager")
			os.Exit(1)
		}
		setupLog.Info("REST API server enabled", "port", apiPort)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// runnableServer is a wrapper to make the API server compatible with controller-runtime's Manager
type runnableServer struct {
	server *apiv1.Server
	ctx    context.Context
}

func (r *runnableServer) Start(ctx context.Context) error {
	return r.server.Start(ctx)
}
