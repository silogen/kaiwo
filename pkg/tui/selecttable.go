/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedib0t/go-pretty/v6/table"
	"strings"
)

type SelectTableRow[T any] struct {
	Selectable bool
	Selected   bool
	Entry      SelectTableEntry[T]
}

type SelectTableEntry[T interface{}] interface {
	GetCells() []interface{}
	IsSelectable() bool
	GetData() *T
}

type SelectTableModel[T any] struct {
	Rows    []SelectTableRow[T]
	Columns []string
	Title   string
}

func (m SelectTableModel[any]) Init() tea.Cmd {
	return nil
}

func (m SelectTableModel[any]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "down":
			moveSelection(m, 1)
		case "up":
			moveSelection(m, -1)
		case "enter":
			return m, tea.Quit
		}
	}

	return m, nil
}

func moveSelection[T any](m SelectTableModel[T], direction int) SelectTableModel[T] {
	var previouslySelectedIndex = -1

	// Find the currently selected row
	for i := range m.Rows {
		if m.Rows[i].Selected {
			previouslySelectedIndex = i
			break
		}
	}

	// If no row was selected yet or the table is empty, just return
	if previouslySelectedIndex == -1 || len(m.Rows) == 0 {
		return m
	}

	// Move in the specified direction, looking for the next selectable row
	newIndex := previouslySelectedIndex
	for {
		newIndex += direction
		if newIndex < 0 || newIndex >= len(m.Rows) {
			// If we roll off the top or bottom, break (or wrap if you want).
			break
		}
		if m.Rows[newIndex].Selectable {
			// Unselect old row, select new row, and break.
			m.Rows[previouslySelectedIndex].Selected = false
			m.Rows[newIndex].Selected = true
			break
		}
	}
	return m
}

// View returns a textual representation of our table for rendering.
func (m SelectTableModel[any]) View() string {
	tw := table.NewWriter()

	displayHeaders := make([]interface{}, len(m.Columns)+1)
	displayHeaders[0] = ""
	for i := 1; i < len(displayHeaders); i++ {
		displayHeaders[i] = m.Columns[i-1]
	}

	tw.AppendHeader(displayHeaders)
	tw.SetTitle(m.Title)

	for _, row := range m.Rows {
		var cursor interface{}
		cursor = " "
		if row.Selected {
			cursor = ">"
		}

		displayRow := append([]interface{}{cursor}, row.Entry.GetCells()...)
		tw.AppendRow(displayRow)
	}

	// You can customize table style, borders, etc. here
	return tw.Render()
}

// RunSelectTable is a helper function that spins up the Bubble Tea program
// with the given columns and rows and returns which row the user selected.
func RunSelectTable[T any](data []SelectTableEntry[T], columns []string, title string, clearAfterCompletion bool) (*T, error) {

	var rows []SelectTableRow[T]

	// Ensure that at least one selectable row is marked as Selected by default
	firstSelected := false
	for _, entry := range data {
		row := SelectTableRow[T]{
			Selectable: entry.IsSelectable(),
			Entry:      entry,
		}
		if row.Selectable && !firstSelected {
			firstSelected = true
			row.Selected = true
		}
		rows = append(rows, row)
	}

	m := SelectTableModel[T]{
		Columns: columns,
		Rows:    rows,
		Title:   title,
	}

	program := tea.NewProgram(m)
	output, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run table select: %w", err)
	}

	outputModel := output.(SelectTableModel[T])

	if clearAfterCompletion {
		tableView := outputModel.View()
		lines := strings.Count(tableView, "\n")

		// Move the cursor up by the number of lines and clear the content
		for i := 0; i < lines; i++ {
			fmt.Print("\033[F\033[2K") // Move up and clear the current line
		}
	}

	for _, row := range outputModel.Rows {
		if row.Selected {
			return row.Entry.GetData(), nil
		}
	}

	return nil, fmt.Errorf("failed to run table select: no rows selected")

}
