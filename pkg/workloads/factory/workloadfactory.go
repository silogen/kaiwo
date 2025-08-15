package factory

import (
	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/api"
	job "github.com/silogen/kaiwo/pkg/workloads/job"
	svc "github.com/silogen/kaiwo/pkg/workloads/service"
	"k8s.io/apimachinery/pkg/runtime"
)

func NewJobReconciler(s *runtime.Scheme, k *kaiwo.KaiwoJob) api.WorkloadReconciler {
	if k.Spec.IsRayJob() {
		return &job.RayJobHandler{Scheme: s, KaiwoJob: k}
	}
	return &job.BatchJobHandler{Scheme: s, KaiwoJob: k}
}

func NewServiceReconciler(s *runtime.Scheme, x *kaiwo.KaiwoService) api.WorkloadReconciler {
	if x.Spec.IsRayService() {
		return &svc.RayServiceHandler{Scheme: s, KaiwoService: x}
	}
	return &svc.DeploymentHandler{Scheme: s, KaiwoService: x}
}
