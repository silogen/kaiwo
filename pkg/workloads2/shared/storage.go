package workloadshared

import (
	"context"
	"fmt"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

type StorageCommand[T any] struct {
	workloadutils.CommandBase[T]
	PvcNamePostfix   string
	Amount           string
	StorageClassName string
	AccessMode       corev1.PersistentVolumeAccessMode
	StoreCallback    func(base *T, claim *corev1.PersistentVolumeClaim, target string)
	StoreInto        *corev1.PersistentVolumeClaim
}

func (cmd *StorageCommand[T]) Build() (client.Object, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), cmd.PvcNamePostfix),
			Namespace: cmd.Owner.GetNamespace(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				cmd.AccessMode,
			},
			StorageClassName: &cmd.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(cmd.Amount),
				},
			},
		},
	}

	cmd.StoreCallback(cmd.State, pvc, cmd.PvcNamePostfix)
	return pvc, nil
}

func SetStorage(state *workloadutils.CommandStateBase, pvc *corev1.PersistentVolumeClaim, target string) {
	if target == DataStoragePostfix {
		state.DataPvc = pvc
	} else if target == HfStoragePostfix {
		state.HfPvc = pvc
	} else {
		panic(fmt.Sprintf("unknown target %s", target))
	}
}

func UpdatePodSpecStorage(ctx context.Context, podSpec *corev1.PodSpec, storageSpec v1alpha1.StorageSpec, ownerName string) error {
	logger := log.FromContext(ctx)

	if !storageSpec.StorageEnabled {
		return nil
	}

	addStorageVolume := func(name string, claimName string) {
		logger.Info(fmt.Sprintf("Adding %s volume", name))
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	// Add volumes
	if storageSpec.Data.IsRequested() {
		addStorageVolume(DataStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, DataStoragePostfix))
	}

	if storageSpec.HuggingFace.IsRequested() {
		addStorageVolume(HfStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, HfStoragePostfix))
	}

	addVolumeMount := func(container *corev1.Container, name string, path string) {
		logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}

	addEnvVar := func(container *corev1.Container, name string, value string) {
		logger.Info(fmt.Sprintf("Adding %s env var to %s", name, container.Name))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	addContainerInfo := func(container *corev1.Container) {
		if storageSpec.Data.IsRequested() {
			addVolumeMount(container, DataStoragePostfix, storageSpec.Data.MountPath)
		}
		if storageSpec.HuggingFace.IsRequested() {
			addVolumeMount(container, HfStoragePostfix, storageSpec.HuggingFace.MountPath)
			addEnvVar(container, "HF_HOME", storageSpec.HuggingFace.MountPath)
		}
	}

	for i := range podSpec.Containers {
		addContainerInfo(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		addContainerInfo(&podSpec.InitContainers[i])
	}

	return nil
}
