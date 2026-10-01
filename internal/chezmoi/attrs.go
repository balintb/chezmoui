package chezmoi

import "strings"

// Attribute identifies a chezmoi source-state attribute encoded in a source path.
type Attribute string

const (
	AttrPrivate    Attribute = "private"
	AttrReadOnly   Attribute = "readonly"
	AttrExecutable Attribute = "executable"
	AttrEncrypted  Attribute = "encrypted"
	AttrTemplate   Attribute = "template"
	AttrSymlink    Attribute = "symlink"
	AttrCreate     Attribute = "create"
	AttrModify     Attribute = "modify"
	AttrRemove     Attribute = "remove"
	AttrExact      Attribute = "exact"
	AttrExternal   Attribute = "external"
	AttrScript     Attribute = "script"
	AttrOnce       Attribute = "once"
	AttrOnChange   Attribute = "onchange"
	AttrBefore     Attribute = "before"
	AttrAfter      Attribute = "after"
	AttrEmpty      Attribute = "empty"
	AttrLiteral    Attribute = "literal"
)

// prefixAttrs maps source filename prefixes to attributes. Order does not matter because every prefix is stripped wherever it appears.
var prefixAttrs = []struct {
	prefix string
	attr   Attribute
}{
	{"private_", AttrPrivate},
	{"readonly_", AttrReadOnly},
	{"executable_", AttrExecutable},
	{"encrypted_", AttrEncrypted},
	{"symlink_", AttrSymlink},
	{"create_", AttrCreate},
	{"modify_", AttrModify},
	{"remove_", AttrRemove},
	{"exact_", AttrExact},
	{"external_", AttrExternal},
	{"empty_", AttrEmpty},
	{"literal_", AttrLiteral},
}

// scriptPrefixes are the ordered "run_" script qualifiers.
var scriptPrefixes = []string{"run_once_", "run_onchange_", "run_before_", "run_after_", "run_"}

// ParseAttributes derives the attribute set from a source-relative path. Every path component contributes, because chezmoi applies an attribute on a directory to everything beneath it (e.g. `private_dot_config/exact_foo.tmpl` is private, exact, and a template). A template suffix is recognised on the leaf only.
func ParseAttributes(sourceRelative string) []Attribute {
	if sourceRelative == "" {
		return nil
	}
	seen := map[Attribute]bool{}
	add := func(a Attribute) { seen[a] = true }

	parts := strings.Split(sourceRelative, "/")
	for i, part := range parts {
		leaf := i == len(parts)-1
		addComponentAttributes(part, leaf, add)
	}

	if len(seen) == 0 {
		return nil
	}
	out := make([]Attribute, 0, len(seen))
	for _, a := range attrOrder {
		if seen[a] {
			out = append(out, a)
		}
	}
	return out
}

// addComponentAttributes extracts prefix and leaf-only (script/template) attributes from a single path component.
func addComponentAttributes(component string, leaf bool, add func(Attribute)) {
	changed := true
	for changed {
		changed = false
		for _, p := range prefixAttrs {
			if strings.HasPrefix(component, p.prefix) {
				add(p.attr)
				component = component[len(p.prefix):]
				changed = true
			}
		}
	}
	if !leaf {
		return
	}
	if s, ok := matchScriptPrefix(component); ok {
		add(AttrScript)
		for _, q := range s {
			add(q)
		}
	}
	if strings.HasSuffix(component, ".tmpl") {
		add(AttrTemplate)
	}
}

// matchScriptPrefix returns the qualifier attributes for a "run_*" basename.
func matchScriptPrefix(base string) ([]Attribute, bool) {
	switch {
	case strings.HasPrefix(base, "run_once_"):
		return []Attribute{AttrOnce}, true
	case strings.HasPrefix(base, "run_onchange_"):
		return []Attribute{AttrOnChange}, true
	case strings.HasPrefix(base, "run_before_"):
		return []Attribute{AttrBefore}, true
	case strings.HasPrefix(base, "run_after_"):
		return []Attribute{AttrAfter}, true
	case strings.HasPrefix(base, "run_"):
		return nil, true
	}
	return nil, false
}

// attrOrder fixes a stable display order independent of parse order.
var attrOrder = []Attribute{
	AttrTemplate,
	AttrPrivate,
	AttrReadOnly,
	AttrExecutable,
	AttrEncrypted,
	AttrSymlink,
	AttrCreate,
	AttrModify,
	AttrRemove,
	AttrExact,
	AttrExternal,
	AttrEmpty,
	AttrLiteral,
	AttrScript,
	AttrOnce,
	AttrOnChange,
	AttrBefore,
	AttrAfter,
}

// Short returns a compact glyph for an attribute, used in list rows.
func (a Attribute) Short() string {
	switch a {
	case AttrPrivate:
		return "p"
	case AttrReadOnly:
		return "r"
	case AttrExecutable:
		return "x"
	case AttrEncrypted:
		return "🔒"
	case AttrTemplate:
		return "t"
	case AttrSymlink:
		return "s"
	case AttrCreate:
		return "c"
	case AttrModify:
		return "m"
	case AttrRemove:
		return "d"
	case AttrExact:
		return "e"
	case AttrExternal:
		return "E"
	case AttrScript:
		return "▶"
	case AttrOnce:
		return "1"
	case AttrOnChange:
		return "Δ"
	case AttrBefore:
		return "b"
	case AttrAfter:
		return "a"
	case AttrEmpty:
		return "0"
	case AttrLiteral:
		return "L"
	}
	return "?"
}
