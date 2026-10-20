package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/resolve"
)

const (
	// settingsPath is Claude Code's project settings file, which is
	// committed and shared by the team.
	settingsPath = ".claude/settings.json"
	// settingsSchema gives editors autocompletion in a new settings file.
	settingsSchema = "https://json.schemastore.org/claude-code-settings.json"
	// ruleMaintenanceID is the entry whose deny rules protect generated
	// files. The topic folder is configurable, so the rule covering it
	// cannot live in the catalog and the renderer adds it.
	ruleMaintenanceID = "rule-maintenance-propose"
)

// permissionRules holds the Claude permission rules of the selected
// entries: one group per entry that has some, in selection order, each
// group in the order the catalog lists it. The order inside a group
// matters: a negated rule such as Read(!.env.example) only narrows the
// rules listed before it.
type permissionRules struct {
	// deny blocks an action, even when the user would approve it.
	deny [][]string
	// ask makes Claude Code ask the user before the action, for rules that
	// mean "ask first" rather than "never".
	ask [][]string
}

// empty reports whether there is no rule to write.
func (p permissionRules) empty() bool {
	return len(p.deny) == 0 && len(p.ask) == 0
}

// claudeRules returns the Claude permission rules of r's selected entries.
func claudeRules(r *resolve.Result) permissionRules {
	var p permissionRules
	for _, e := range r.Entries {
		if e.Enforce == nil || e.Enforce.Claude == nil {
			continue
		}
		c := e.Enforce.Claude
		deny := slices.Clone(c.Deny) // the catalog's slices are shared
		if e.ID == ruleMaintenanceID {
			deny = append(deny, "Edit(/"+r.Spec.Output.TopicDir+"/**)")
		}
		if len(deny) > 0 {
			p.deny = append(p.deny, deny)
		}
		if len(c.Ask) > 0 {
			p.ask = append(p.ask, slices.Clone(c.Ask))
		}
	}
	return p
}

// renderSettings returns .claude/settings.json with the permission rules
// of the selected entries, and false when there is nothing to write: no
// rules and no existing file.
//
// An existing file is merged, never replaced: every key the user wrote
// stays, in its order and with its exact values, and only permissions.deny
// and permissions.ask change. When the merge adds nothing, the existing
// bytes come back as they were. A file that cannot be read safely, such as
// invalid JSON, is an error, because rewriting it could lose what the user
// wrote.
func renderSettings(r *resolve.Result, existing fs.FS) (File, bool, error) {
	rules := claudeRules(r)
	old, err := readExisting(existing, settingsPath)
	if err != nil {
		return File{}, false, err
	}
	if old == nil && rules.empty() {
		return File{}, false, nil
	}

	content, err := mergeSettings(old, rules)
	if err != nil {
		return File{}, false, fmt.Errorf("render %s: %w", settingsPath, err)
	}
	return File{Path: settingsPath, Content: content}, true, nil
}

// mergeSettings merges rules into the settings file content old, which is
// nil for a new file. Each list is merged on its own with mergeOrdered, and
// a list with nothing to add is left as it is, or left out when absent.
func mergeSettings(old []byte, rules permissionRules) ([]byte, error) {
	root := jsonObject{}
	if old == nil {
		schema, err := marshalJSON(settingsSchema)
		if err != nil {
			return nil, err
		}
		root = root.set("$schema", schema)
	} else if len(bytes.TrimSpace(old)) > 0 {
		var err error
		if root, err = parseJSONObject(old); err != nil {
			return nil, err
		}
	}

	perms := jsonObject{}
	if raw, ok := root.get("permissions"); ok {
		var err error
		if perms, err = parseJSONObject(raw); err != nil {
			return nil, fmt.Errorf("permissions: %w", err)
		}
	}
	changed := false
	for _, list := range []struct {
		key    string
		groups [][]string
	}{
		{"deny", rules.deny},
		{"ask", rules.ask},
	} {
		var current []string
		if raw, ok := perms.get(list.key); ok {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, fmt.Errorf("permissions.%s must be a list of strings: %w", list.key, err)
			}
		}
		merged := mergeOrdered(current, list.groups)
		if slices.Equal(merged, current) {
			continue
		}
		raw, err := marshalJSON(merged)
		if err != nil {
			return nil, err
		}
		perms = perms.set(list.key, raw)
		changed = true
	}
	if old != nil && !changed {
		return old, nil
	}

	permsJSON, err := marshalJSON(perms)
	if err != nil {
		return nil, err
	}
	return indentJSON(root.set("permissions", permsJSON))
}
