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

package tuicomponents

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63")).
	Padding(1, 1)

type model struct {
	table         table.Model
	selectedRow   *[]string
	title         string
	selectedIndex int
	result        StepResult
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", tea.KeyBackspace.String():
			m.result = StepResultPrevious
			return m, tea.Quit
		case "q", "ctrl+c":
			m.selectedRow = nil
			m.result = StepResultQuit
			return m, tea.Quit
		case "enter":
			selectedRow := []string(m.table.SelectedRow())
			m.selectedRow = &selectedRow
			m.selectedIndex = m.table.Cursor()
			m.result = StepResultOk
			return m, tea.Quit
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return lipgloss.NewStyle().Bold(true).Render(m.title) + "\n" + baseStyle.Render(m.table.View()) + "\n"
}

func RunSelectTable(data [][]string, columns []string, title string, clearAfterFinish bool) (int, StepResult, error) {
	if len(data) == 0 {
		return -1, StepResultErr, fmt.Errorf("no data to show")
	}

	// Check for column consistency
	numColumns := len(data[0])
	for _, row := range data {
		if len(row) != numColumns {
			return -1, StepResultErr, fmt.Errorf("column count mismatch")
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
		return -1, StepResultErr, fmt.Errorf("error running tea: %v", err)
	}

	if clearAfterFinish {
		clearTable(len(strings.Split(m.View(), "\n")))
	}

	outputModel := om.(model)
	return outputModel.selectedIndex, outputModel.result, nil
}

// clearTable clears the last `height` rows from the terminal
func clearTable(height int) {
	for i := 0; i < height; i++ {
		fmt.Print("\033[F\033[2K") // Move up and clear the current line
	}
}
