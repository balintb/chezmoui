package chezmoi

import (
	"reflect"
	"testing"
)

func TestParseAttributes_Cases(t *testing.T) {
	cases := []struct {
		in   string
		want []Attribute
	}{
		{"", nil},
		{"dot_bashrc", nil},
		{"private_dot_bashrc", []Attribute{AttrPrivate}},
		{"dot_ssh/private_id_rsa", []Attribute{AttrPrivate}},
		{"executable_dot_local/bin/foo", []Attribute{AttrExecutable}},
		{"dot_gitconfig.tmpl", []Attribute{AttrTemplate}},
		{"private_encrypted_executable_dot_foo.tmpl", []Attribute{AttrTemplate, AttrPrivate, AttrExecutable, AttrEncrypted}},
		{"encrypted_private_dot_foo", []Attribute{AttrPrivate, AttrEncrypted}},
		{"symlink_dot_foo", []Attribute{AttrSymlink}},
		{"readonly_dot_foo", []Attribute{AttrReadOnly}},
		{"create_dot_foo", []Attribute{AttrCreate}},
		{"modify_dot_foo", []Attribute{AttrModify}},
		{"remove_dot_foo", []Attribute{AttrRemove}},
		{"exact_dot_config", []Attribute{AttrExact}},
		{"external_dot_foo", []Attribute{AttrExternal}},
		{"empty_dot_foo", []Attribute{AttrEmpty}},
		{"literal_dot_foo", []Attribute{AttrLiteral}},
		{"run_foo.sh", []Attribute{AttrScript}},
		{"run_once_foo.sh", []Attribute{AttrScript, AttrOnce}},
		{"run_onchange_foo.sh", []Attribute{AttrScript, AttrOnChange}},
		{"run_before_foo.sh", []Attribute{AttrScript, AttrBefore}},
		{"run_after_foo.sh", []Attribute{AttrScript, AttrAfter}},
		{"run_once_before_foo.sh", []Attribute{AttrScript, AttrOnce}},
		{"private_dot_config/exact_foo.tmpl", []Attribute{AttrTemplate, AttrPrivate, AttrExact}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := ParseAttributes(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ParseAttributes(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParseAttributes_StableOrder(t *testing.T) {
	// Regardless of the order prefixes appear in the filename, the result is sorted by the canonical display order.
	a := ParseAttributes("private_executable_dot_foo")
	b := ParseAttributes("executable_private_dot_foo")
	if !reflect.DeepEqual(a, b) {
		t.Errorf("attribute order should be canonical: %v vs %v", a, b)
	}
	if a[0] != AttrPrivate || a[1] != AttrExecutable {
		t.Errorf("canonical order should place private before executable, got %v", a)
	}
}

func TestParseAttributes_NoDuplicates(t *testing.T) {
	got := ParseAttributes("private_private_dot_foo")
	count := 0
	for _, a := range got {
		if a == AttrPrivate {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one private attribute, got %d (%v)", count, got)
	}
}

func TestAttribute_Short_AllDistinct(t *testing.T) {
	seen := map[string]Attribute{}
	for _, a := range attrOrder {
		s := a.Short()
		if s == "" {
			t.Errorf("attribute %q has empty glyph", a)
		}
		if prev, ok := seen[s]; ok {
			t.Errorf("glyph %q collides between %q and %q", s, prev, a)
		}
		seen[s] = a
	}
}

func TestParseAttributes_DirectoryAttributesInherited(t *testing.T) {
	// chezmoi applies an attribute on a directory to everything beneath it.
	got := ParseAttributes("private_dot_config/foo")
	if !reflect.DeepEqual(got, []Attribute{AttrPrivate}) {
		t.Errorf("expected inherited private, got %v", got)
	}
}

func TestParseAttributes_PlainBasenameHasNoAttrs(t *testing.T) {
	if got := ParseAttributes("plain_dir/plain_foo"); len(got) != 0 {
		t.Errorf("plain path should have no attributes, got %v", got)
	}
}

// FuzzParseAttributes asserts the parser never panics, returns no duplicates, and only emits attributes from the known set.
func FuzzParseAttributes(f *testing.F) {
	f.Add("private_dot_bashrc")
	f.Add("dot_config/btop/btop.conf.tmpl")
	f.Add("run_once_before_foo.sh")
	f.Add("executable_private_encrypted_dot_foo.tmpl")
	f.Add("")
	f.Add("____.tmpl")
	f.Add("\x00\x01")
	f.Fuzz(func(t *testing.T, in string) {
		got := ParseAttributes(in)
		seen := map[Attribute]bool{}
		known := map[Attribute]bool{}
		for _, a := range attrOrder {
			known[a] = true
		}
		for _, a := range got {
			if !known[a] {
				t.Fatalf("unknown attribute %q from %q", a, in)
			}
			if seen[a] {
				t.Fatalf("duplicate attribute %q from %q", a, in)
			}
			seen[a] = true
		}
	})
}
