package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
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
	// owned holds the rules cyanotype.lock says cyanotype added the last
	// time. Those no longer generated are removed from the file; no other
	// rule is ever removed.
	owned lock.Rules
}

// empty reports whether there is no rule to write.
func (p permissionRules) empty() bool {
	return len(p.deny) == 0 && len(p.ask) == 0
}

// claudeRules returns the Claude permission rules of r's selected entries.
// An entry's ask group holds its ask rules, then the rules of each key of
// its enforce.claude.with whose entry is selected too, keys in alphabetical
// order since a map has none, each key's rules in catalog order.
func claudeRules(r *resolve.Result) permissionRules {
	selected := make(map[string]bool, len(r.Entries))
	for _, e := range r.Entries {
		selected[e.ID] = true
	}

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

		ask := slices.Clone(c.Ask)
		for _, key := range slices.Sorted(maps.Keys(c.With)) {
			if selected[key] {
				ask = append(ask, c.With[key].Ask...)
			}
		}
		if len(ask) > 0 {
			p.ask = append(p.ask, ask)
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
// and permissions.ask change. When the merge changes nothing, the existing
// bytes come back as they were. A file that cannot be read safely, such as
// invalid JSON, is an error, because rewriting it could lose what the user
// wrote.
//
// The rules that cyanotype.lock in existing says cyanotype added, and that
// are no longer generated, are removed, also when claude is no longer a
// target. The returned file records which rules cyanotype owns after the
// merge, for the new lock.
func renderSettings(r *resolve.Result, existing fs.FS) (File, bool, error) {
	prev, err := lock.Read(existing)
	if err != nil {
		return File{}, false, err
	}
	var rules permissionRules
	if slices.Contains(r.Spec.Targets, spec.TargetClaude) {
		rules = claudeRules(r)
	}
	if prev != nil {
		rules.owned = prev.Claude
	}
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
	owned, err := ownedRules(old, content, rules)
	if err != nil {
		return File{}, false, fmt.Errorf("render %s: %w", settingsPath, err)
	}
	return File{Path: settingsPath, Content: content, owned: &owned}, true, nil
}

// ownedRules returns the rules cyanotype owns once the merge has turned the
// settings content before into after: those rules.owned held that are
// still generated and still in the file, plus those the merge added. Rules
// are counted, not just named, since the merge may add a rule the file
// already has, to keep a negation after the rules it narrows.
func ownedRules(before, after []byte, rules permissionRules) (lock.Rules, error) {
	beforeDeny, beforeAsk, err := permissionLists(before)
	if err != nil {
		return lock.Rules{}, err
	}
	afterDeny, afterAsk, err := permissionLists(after)
	if err != nil {
		return lock.Rules{}, err
	}
	return lock.Rules{
		Deny: ownedList(beforeDeny, afterDeny, rules.owned.Deny, rules.deny),
		Ask:  ownedList(beforeAsk, afterAsk, rules.owned.Ask, rules.ask),
	}, nil
}

// ownedList applies ownedRules to one list. Of a generated rule, cyanotype
// owns what it owned before, at most as many as the file had, plus what
// the merge added. A rule no longer generated has been removed, and one
// never generated was never cyanotype's.
func ownedList(before, after, prevOwned []string, groups [][]string) []string {
	var out []string
	for _, rule := range slices.Compact(slices.Sorted(slices.Values(slices.Concat(groups...)))) {
		n := min(count(prevOwned, rule), count(before, rule)) + count(after, rule) - count(before, rule)
		for range n {
			out = append(out, rule)
		}
	}
	return out
}

// count returns how many times list holds s.
func count(list []string, s string) int {
	n := 0
	for _, item := range list {
		if item == s {
			n++
		}
	}
	return n
}

// permissionLists returns permissions.deny and permissions.ask of a
// settings file, or nothing for a missing or blank one.
func permissionLists(data []byte) (deny, ask []string, err error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, nil
	}
	root, err := parseJSONObject(data)
	if err != nil {
		return nil, nil, err
	}
	raw, ok := root.get("permissions")
	if !ok {
		return nil, nil, nil
	}
	perms, err := parseJSONObject(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("permissions: %w", err)
	}
	for _, list := range []struct {
		key string
		out *[]string
	}{{"deny", &deny}, {"ask", &ask}} {
		if raw, ok := perms.get(list.key); ok {
			if err := json.Unmarshal(raw, list.out); err != nil {
				return nil, nil, fmt.Errorf("permissions.%s must be a list of strings: %w", list.key, err)
			}
		}
	}
	return deny, ask, nil
}

// removeOwned removes from list one occurrence of each rule in owned that
// groups no longer generate: the last one, since cyanotype appends the
// rules it adds after the user's. A rule cyanotype does not own is never
// removed, even when it matches one cyanotype once generated.
func removeOwned(list, owned []string, groups [][]string) []string {
	generated := slices.Concat(groups...)
	out := slices.Clone(list)
	for _, rule := range owned {
		if slices.Contains(generated, rule) {
			continue
		}
		for i := len(out) - 1; i >= 0; i-- {
			if out[i] == rule {
				out = slices.Delete(out, i, i+1)
				break
			}
		}
	}
	return out
}

// mergeSettings merges rules into the settings file content old, which is
// nil for a new file. Each list is merged on its own: first the rules
// cyanotype owns and no longer generates are removed (see removeOwned),
// then the generated ones are added with mergeOrdered. A list the merge
// does not change is left as it is, or left out when absent; a list the
// removal empties is left out, and so is a permissions object left empty.
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
		owned  []string
	}{
		{"deny", rules.deny, rules.owned.Deny},
		{"ask", rules.ask, rules.owned.Ask},
	} {
		var current []string
		if raw, ok := perms.get(list.key); ok {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, fmt.Errorf("permissions.%s must be a list of strings: %w", list.key, err)
			}
		}
		merged := mergeOrdered(removeOwned(current, list.owned, list.groups), list.groups)
		if slices.Equal(merged, current) {
			continue
		}
		changed = true
		if len(merged) == 0 {
			perms = perms.without(list.key)
			continue
		}
		raw, err := marshalJSON(merged)
		if err != nil {
			return nil, err
		}
		perms = perms.set(list.key, raw)
	}
	if old != nil && !changed {
		return old, nil
	}
	if len(perms) == 0 {
		return indentJSON(root.without("permissions"))
	}

	permsJSON, err := marshalJSON(perms)
	if err != nil {
		return nil, err
	}
	return indentJSON(root.set("permissions", permsJSON))
}
