package workloadjob

import (
	"fmt"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	"github.com/silogen/kaiwo/pkg/workloads2/shared"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BatchJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *v1alpha1.KaiwoJob
}

func (k *BatchJobCommand) Build() (client.Object, error) {
	kaiwoJob := k.KaiwoJob

	jobSpec := kaiwoJob.Spec.JobSpec
	if jobSpec == nil {
		// TODO Add ctx logger
		// logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob")

		jobSpec = &batchv1.JobSpec{
			TTLSecondsAfterFinished: baseutils.Pointer(int32(43200)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		}

		if err := workloadshared.FillPodSpec(kaiwoJob, &jobSpec.Template.Spec); err != nil {
			// TODO Add ctx logger
			return nil, fmt.Errorf("failed to fill pod spec: %w", err)
		}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: *jobSpec,
	}

	if kaiwoJob.Spec.Storage != nil {
		if err := controllerutils.UpdatePodSpecStorage(k.Context, &job.Spec.Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return nil, err
		}
	}

	return job, nil
}
