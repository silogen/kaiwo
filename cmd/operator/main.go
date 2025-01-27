package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/silogen/kaiwo/pkg/api/v1"
	"github.com/silogen/kaiwo/pkg/controllers"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
}

func main() {
	log := logrus.New()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "kaiwo.example.com",
	    })
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := v1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to register KaiwoJob and KaiwoService types")
		os.Exit(1)
	}

	if err = (&controllers.KaiwoJobReconciler{
		Client: mgr.GetClient(),
		Log:    logrus.NewEntry(log),
	}).SetupWithManager(mgr); err != nil {
		log.Fatalf("unable to create controller for KaiwoJob: %v", err)
	}

	if err = (&controllers.KaiwoServiceReconciler{
		Client: mgr.GetClient(),
		Log:    logrus.NewEntry(log),
	}).SetupWithManager(mgr); err != nil {
		log.Fatalf("unable to create controller for KaiwoService: %v", err)
	}

	log.Infof("starting operator manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("problem running manager: %v", err)
	}
}
