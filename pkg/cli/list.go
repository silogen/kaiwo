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

package cmd

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	tui "github.com/silogen/kaiwo/pkg/tui/list"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var (
	listUser      string
	namespaceList string
	allUsers      bool
)

func BuildListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manage [workloadType] [workloadName]",
		Args:  cobra.MaximumNArgs(2),
		Short: "Manage workloads interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			workloadType := ""
			workloadName := ""

			if len(args) > 0 {
				workloadType = args[0]
			}
			if len(args) > 1 {
				workloadName = args[1]
			}

			if listUser == "" && !allUsers {
				var err error
				listUser, err = baseutils.GetCurrentUser()
				if err != nil {
					return fmt.Errorf("could not get current user: %v", err)
				}
			}
			if listUser != "" {
				logrus.Infof("Listing as user: %s", listUser)
			}

			return tui.RunList(workloadType, workloadName, namespaceList, user)
		},
	}
	cmd.Flags().StringVarP(&listUser, "user", "u", "", "Limit the workloads to one created by this user")
	cmd.Flags().StringVarP(&namespaceList, "namespace", "n", "kaiwo", "Namespace of the workloads to return")
	cmd.Flags().BoolVarP(&allUsers, "all-users", "", false, "Return workloads for all users")
	return cmd
}
