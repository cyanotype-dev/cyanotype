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

// denyGroups returns the Claude deny rules of every selected entry that has
// some, one group per entry in selection order, each group in the order the
// catalog lists it. The order inside a group matters: a negated rule such as
// Read(!.env.example) only narrows the rules listed before it.
func denyGroups(r *resolve.Result) [][]string {
	var groups [][]string
	for _, e := range r.Entries {
		if e.Enforce == nil || e.Enforce.Claude == nil || len(e.Enforce.Claude.Deny) == 0 {
			continue
		}
		group := slices.Clone(e.Enforce.Claude.Deny) // the catalog's slices are shared
		if e.ID == ruleMaintenanceID {
			group = append(group, "Edit(/"+r.Spec.Output.TopicDir+"/**)")
		}
		groups = append(groups, group)
	}
	return groups
}

// mergeDeny adds the rules of groups to existing, which it keeps unchanged
// at the start. Each group's rules end up in the result in their original
// order, as a subsequence: a rule already present is reused only when it
// comes after the previous rule of the same group, otherwise it is added
// again at the end. That keeps every negation after the rules it narrows,
// whatever order the existing list has, at the cost of an occasional
// duplicate. Merging the result again changes nothing.
func mergeDeny(existing []string, groups [][]string) []string {
	out := slices.Clone(existing)
	for _, group := range groups {
		last := -1
		for _, rule := range group {
			i := indexAfter(out, rule, last)
			if i < 0 {
				out = append(out, rule)
				i = len(out) - 1
			}
			last = i
		}
	}
	return out
}

// indexAfter returns the first index greater than after where list holds
// s, or -1.
func indexAfter(list []string, s string, after int) int {
	for i := after + 1; i < len(list); i++ {
		if list[i] == s {
			return i
		}
	}
	return -1
}

// renderSettings returns .claude/settings.json with the deny rules of the
// selected entries, and false when there is nothing to write: no rules and
// no existing file.
//
// An existing file is merged, never replaced: every key the user wrote
// stays, in its order and with its exact values, and only permissions.deny
// changes. When the merge adds nothing, the existing bytes come back as
// they were. A file that cannot be read safely, such as invalid JSON, is
// an error, because rewriting it could lose what the user wrote.
func renderSettings(r *resolve.Result, existing fs.FS) (File, bool, error) {
	groups := denyGroups(r)
	old, err := readExisting(existing, settingsPath)
	if err != nil {
		return File{}, false, err
	}
	if old == nil && len(groups) == 0 {
		return File{}, false, nil
	}

	content, err := mergeSettings(old, groups)
	if err != nil {
		return File{}, false, fmt.Errorf("render %s: %w", settingsPath, err)
	}
	return File{Path: settingsPath, Content: content}, true, nil
}

// mergeSettings merges groups into the settings file content old, which is
// nil for a new file.
func mergeSettings(old []byte, groups [][]string) ([]byte, error) {
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
	var deny []string
	if raw, ok := perms.get("deny"); ok {
		if err := json.Unmarshal(raw, &deny); err != nil {
			return nil, fmt.Errorf("permissions.deny must be a list of strings: %w", err)
		}
	}

	merged := mergeDeny(deny, groups)
	if old != nil && slices.Equal(merged, deny) {
		return old, nil
	}

	denyJSON, err := marshalJSON(merged)
	if err != nil {
		return nil, err
	}
	permsJSON, err := marshalJSON(perms.set("deny", denyJSON))
	if err != nil {
		return nil, err
	}
	return indentJSON(root.set("permissions", permsJSON))
}
