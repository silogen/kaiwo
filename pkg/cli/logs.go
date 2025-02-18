// // Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// //       http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.
package cmd

//
//import (
//	"context"
//	"fmt"
//
//	"github.com/silogen/kaiwo/pkg/cli/utils"
//
//	"github.com/spf13/cobra"
//
//	"github.com/silogen/kaiwo/pkg/k8s"
//	list "github.com/silogen/kaiwo/pkg/tui/list/pod"
//	"github.com/silogen/kaiwo/pkg/workloads/factory"
//)
//
//var (
//	// defaultContainer bool
//	follow bool
//	// since            string
//	// sinceTime        string
//	tailLines int
//	// timestamps bool
//	// previous         bool
//	// stream           string
//	namespaceLogs string
//)
//
//func BuildLogCmd() *cobra.Command {
//	// If no accessor is given, the user can choose
//	cmd := &cobra.Command{
//		Use:   "logs <workloadType>/<workloadName>",
//		Args:  cobra.ExactArgs(1),
//		Short: "Display logs for a workload. You will be prompted to choose the target container if there is more than one",
//		RunE:  executeLogsCommand,
//	}
//	// cmd.Flags().BoolVarP(&defaultContainer, "default", "d", defaultContainer, "Select the default container within the workload")
//	// cmd.Flags().StringVarP(&since, "since", "", "0s", "Only return logs newer than a relative duration")
//	// cmd.Flags().StringVarP(&sinceTime, "since-time", "", "", "Only return logs after a specific date (RFC3339)")
//	cmd.Flags().IntVarP(&tailLines, "tail", "", -1, "Number of lines to show from the end of the log")
//	// cmd.Flags().BoolVarP(&timestamps, "timestamps", "", false, "Show timestamps")
//	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
//	// cmd.Flags().BoolVarP(&previous, "previous", "p", false, "If true, print the logs for the previous instance of the container in a pod if it exists.")
//	// cmd.Flags().StringVarP(&stream, "stream", "", string(StreamTypeAll), "Specify which container log stream to return to the client. One of All, Stdout or Stderr.")
//	cmd.Flags().StringVarP(&namespaceLogs, "namespace", "n", "kaiwo", "Namespace of the workload")
//	return cmd
//}
//
//func executeLogsCommand(_ *cobra.Command, args []string) error {
//	workload, objectKey, err := factory.GetWorkloadAndObjectKey(args[0], namespaceLogs)
//	if err != nil {
//		return err
//	}
//
//	ctx := context.TODO()
//
//	var clients *k8s.KubernetesClients
//	clients, err = k8s.GetKubernetesClients()
//	if err != nil {
//		return fmt.Errorf("failed to get k8s clients: %w", err)
//	}
//
//	if err := workload.LoadFromObjectKey(ctx, clients.Client, objectKey); err != nil {
//		return fmt.Errorf("failed to build workload reference: %w", err)
//	}
//
//	podName, containerName, err, cancelled := list.ChoosePodAndContainer(ctx, *clients, workload)
//
//	if err != nil {
//		return fmt.Errorf("failed to choose pod and container: %w", err)
//	}
//	if cancelled {
//		return nil
//	}
//
//	return utils.OutputLogs(
//		ctx,
//		clients.Clientset,
//		podName,
//		containerName,
//		int64(tailLines),
//		namespaceLogs,
//		follow,
//	)
//}
