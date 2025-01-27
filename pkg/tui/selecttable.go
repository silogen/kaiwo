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

package tui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63")).
	Padding(1, 1)

type model struct {
	table       table.Model
	selectedRow *[]string
	title       string
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.table.Focused() {
				m.table.Blur()
			} else {
				m.table.Focus()
			}
		case "q", "ctrl+c":
			m.selectedRow = nil
			return m, tea.Quit
		case "enter":
			selectedRow := []string(m.table.SelectedRow())
			m.selectedRow = &selectedRow
			return m, tea.Quit
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return lipgloss.NewStyle().Bold(true).Render(m.title) + "\n" + baseStyle.Render(m.table.View()) + "\n"
}

func RunSelectTable(data [][]string, columns []string, title string, clearAfterFinish bool) (*[]string, error) {

	if len(data) == 0 {
		return nil, fmt.Errorf("no data to show")
	}

	// Check for column consistency
	numColumns := len(data[0])
	for _, row := range data {
		if len(row) != numColumns {
			return nil, fmt.Errorf("column count mismatch")
		}
	}

	// Create table columns
	tableColumns := make([]table.Column, numColumns)
	for j := 0; j < numColumns; j++ {
		maxWidth := len(columns[j])
		for i := 0; i < len(data); i++ {
			maxWidth = max(maxWidth, len(data[i][j]))
		}
		tableColumns[j] = table.Column{
			Width: maxWidth,
			Title: columns[j],
		}
	}

	// Create table rows
	rows := make([]table.Row, len(data))
	for i, row := range data {
		rows[i] = row
	}

	t := table.New(
		table.WithColumns(tableColumns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	m := model{table: t, selectedRow: nil, title: title}
	om, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, fmt.Errorf("error running tea: %v", err)
	}

	if clearAfterFinish {
		clearTable(len(strings.Split(m.View(), "\n")))
	}

	outputModel := om.(model)
	return outputModel.selectedRow, nil
}

// clearTable clears the last `height` rows from the terminal
func clearTable(height int) {
	for i := 0; i < height; i++ {
		fmt.Print("\033[F\033[2K") // Move up and clear the current line
	}
}
