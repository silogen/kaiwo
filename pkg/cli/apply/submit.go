// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	workloadjob "github.com/silogen/kaiwo/pkg/workloads2/job"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/pkg/workloads2"

	"github.com/spf13/cobra"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

type KaiwoJobSubmitter struct {
	Job *v1alpha1.KaiwoJob
}

func (k *KaiwoJobSubmitter) GetObject() client.Object {
	return k.Job
}

func (k *KaiwoJobSubmitter) LoadFromPath(path string) error {
	// Load entrypoint
	entrypointPath := filepath.Join(path, "entrypoint")
	if _, err := os.Stat(entrypointPath); err == nil {
		content, err := os.ReadFile(entrypointPath)
		if err == nil {
			k.Job.Spec.EntryPoint = baseutils.Pointer(string(content))
		} else {
			return fmt.Errorf("failed to read entrypoint: %w", err)
		}

	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking if entrypoint file %s exists", entrypointPath)
	}

	return nil
}

func (k *KaiwoJobSubmitter) GetInvoker(ctx context.Context, scheme *runtime.Scheme) (workloadutils.CommandInvoker, error) {
	invoker, err := workloadjob.BuildKaiwoJobInvoker(ctx, scheme, k.Job)
	if err != nil {
		return workloadutils.CommandInvoker{}, fmt.Errorf("failed to build invoker: %w", err)
	}
	return invoker, nil
}

func (k *KaiwoJobSubmitter) FromCliFlags(flags workloads2.CLIFlags) {
	job := v1alpha1.KaiwoJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      flags.Name,
			Namespace: flags.Namespace,
		},
		Spec: v1alpha1.KaiwoJobSpec{
			Image:          flags.Image,
			User:           flags.User,
			Gpus:           flags.GPUs,
			Replicas:       flags.Replicas,
			GpusPerReplica: flags.GPUsPerReplica,
			Version:        flags.Version,
			Ray:            flags.UseRay,
			Dangerous:      flags.Dangerous,
		},
	}

	if flags.ImagePullSecret != nil {
		job.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: *flags.ImagePullSecret,
			},
		}
	}

	k.Job = &job
}

func BuildSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a job",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetCLIFlags(cmd)
			submitter := &KaiwoJobSubmitter{}
			return Apply(submitter, flags)
		},
	}

	// Common shared flags
	AddCliFlags(cmd)

	return cmd
}
