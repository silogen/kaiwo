// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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

package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/workloads/factory"
	"github.com/silogen/kaiwo/pkg/workloads/utils"
)

var (
	//defaultContainer bool
	follow bool
	//since            string
	//sinceTime        string
	tailLines int
	//timestamps       bool
	//previous         bool
	//stream           string
	namespaceLogs string
)

func BuildLogCmd() *cobra.Command {
	// If no accessor is given, the user can choose
	cmd := &cobra.Command{
		Use:   "logs <workloadType>/<workloadName>",
		Args:  cobra.ExactArgs(1),
		Short: "Display logs for a workload. You will be prompted to choose the target container if there is more than one",
		RunE: func(cmd *cobra.Command, args []string) error {

			workload, objectKey, err := factory.GetWorkloadAndObjectKey(args[0], namespaceLogs)

			if err != nil {
				return err
			}

			ctx := context.TODO()

			k8sClient, err := k8s.GetClient()
			if err != nil {
				return err
			}

			// TODO move
			kubeconfig, _ := k8s.GetKubeConfig()
			config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				panic(err)
			}

			// Create Kubernetes clientset
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				panic(err)
			}

			return utils.OutputLogs(
				workload,
				ctx,
				k8sClient,
				clientset,
				objectKey,
				int64(tailLines),
				false,
				//utils.StreamType(stream),
				follow,
			)
		},
	}
	//cmd.Flags().BoolVarP(&defaultContainer, "default", "d", defaultContainer, "Select the default container within the workflow")
	//cmd.Flags().StringVarP(&since, "since", "", "0s", "Only return logs newer than a relative duration")
	//cmd.Flags().StringVarP(&sinceTime, "since-time", "", "", "Only return logs after a specific date (RFC3339)")
	cmd.Flags().IntVarP(&tailLines, "tail", "", -1, "Number of lines to show from the end of the log")
	//cmd.Flags().BoolVarP(&timestamps, "timestamps", "", false, "Show timestamps")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	//cmd.Flags().BoolVarP(&previous, "previous", "p", false, "If true, print the logs for the previous instance of the container in a pod if it exists.")
	//cmd.Flags().StringVarP(&stream, "stream", "", string(StreamTypeAll), "Specify which container log stream to return to the client. One of All, Stdout or Stderr.")
	cmd.Flags().StringVarP(&namespaceLogs, "namespace", "n", "kaiwo", "Namespace of the workflow")
	return cmd
}
