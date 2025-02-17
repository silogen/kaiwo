package workloadutils

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

type CommandStateBase struct {
	DataPvc *corev1.PersistentVolumeClaim
	HfPvc   *corev1.PersistentVolumeClaim

	DownloadJobConfigMap *corev1.ConfigMap
	DownloadJob          *batchv1.Job
}
