// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// selectOrdinal returns the 1-based ordinal (1-9) if the key is a digit key,
// and true. Returns 0, false otherwise.
func selectOrdinal(msg tea.KeyMsg) (int, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return 0, false
	}
	r := msg.Runes[0]
	if r >= '1' && r <= '9' {
		return int(r - '0'), true
	}
	return 0, false
}

// isSelectField returns true when the currently focused form field is a
// huh.Select (any type parameter).
func isSelectField(form *huh.Form) bool {
	field := form.GetFocusedField()
	if field == nil {
		return false
	}
	return selectOptionCount(field) >= 0
}

// selectOptionCount returns the number of visible (filtered) options in a
// huh.Select field, or -1 if the field is not a Select.
func selectOptionCount(field huh.Field) int {
	v := reflect.ValueOf(field)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	fo := v.FieldByName("filteredOptions")
	if !fo.IsValid() {
		return -1
	}
	return fo.Len()
}

// jumpSelectToOrdinal navigates the focused Select to the Nth option
// (1-based). It sends synthetic GotoTop + Down key events to position the
// cursor without touching private state directly.
func (m *Model) jumpSelectToOrdinal(n int) {
	field := m.fs.form.GetFocusedField()
	if field == nil {
		return
	}
	count := selectOptionCount(field)
	if count < 0 || n > count {
		return
	}

	target := n - 1 // convert to 0-based index

	// 'g' resets the Select cursor to position 0.
	gotoTop := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	m.formUpdate(gotoTop)

	// Send target number of 'j' (down) presses.
	down := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	for range target {
		m.formUpdate(down)
	}
}

// formUpdate sends a single message to the form and captures the updated form.
func (m *Model) formUpdate(msg tea.Msg) {
	updated, _ := m.fs.form.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		m.fs.form = form
	}
}
