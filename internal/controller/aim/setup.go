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

package aim

import (
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupWithManager registers all AIM controllers with the manager
func SetupWithManager(mgr ctrl.Manager) error {
	// Create kubernetes clientset for pod log access
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Setup AIMClusterServiceTemplate controller
	if err := (&AIMClusterServiceTemplateReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-cluster-template-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMServiceTemplate controller
	if err := (&AIMServiceTemplateReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-namespace-template-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMService controller
	if err := (&AIMServiceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aim-service-controller"),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&AIMModelCacheReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-modelcache-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMModel controller
	if err := (&AIMModelReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-image-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMClusterModel controller
	if err := (&AIMClusterModelReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-cluster-image-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMTemplateCache controller
	if err := (&AIMTemplateCacheReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-templatecache-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMClusterModelSource controller
	if err := (&ClusterModelSourceReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aim-cluster-image-source-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Setup AIMKVCache controller
	if err := (&AIMKVCacheReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("aimkvcache-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}
