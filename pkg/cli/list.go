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
