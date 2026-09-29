package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// TestHelpPage_CoversEveryKeyBinding reflects over the keyMap so a newly added binding cannot be forgotten on the Help tab.
func TestHelpPage_CoversEveryKeyBinding(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	view := stripANSI(m.viewHelpPage())

	typ := reflect.TypeOf(keyMap{})
	val := reflect.ValueOf(keys)
	bindingType := reflect.TypeOf(key.Binding{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type != bindingType {
			continue
		}
		b := val.Field(i).Interface().(key.Binding)
		h := b.Help()
		if h.Key == "" {
			t.Errorf("binding %s has no help key", f.Name)
			continue
		}
		if !strings.Contains(view, h.Key) {
			t.Errorf("help page is missing binding %s (%q):\n%s", f.Name, h.Key, view)
		}
	}
}

func TestHelpGroups_WellFormed(t *testing.T) {
	groups := helpGroups()
	if len(groups) == 0 {
		t.Fatal("expected at least one help group")
	}
	seen := map[string]bool{}
	for _, g := range groups {
		if g.title == "" {
			t.Error("help group with empty title")
		}
		if len(g.rows) == 0 {
			t.Errorf("help group %q has no rows", g.title)
		}
		for _, r := range g.rows {
			if r[0] == "" || r[1] == "" {
				t.Errorf("help group %q has an empty row: %#v", g.title, r)
			}
			if seen[r[0]+"\x00"+r[1]] {
				t.Errorf("duplicate help row: %#v", r)
			}
			seen[r[0]+"\x00"+r[1]] = true
		}
	}
}
