package render

import (
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// checkLockCatalog refuses a lock written with a newer catalog than the
// one r was resolved with. Regenerating with an older catalog would undo
// the newer one's changes, and two people with different versions would
// rewrite the lock in turn.
func checkLockCatalog(prev *lock.Lock, r *resolve.Result) error {
	if prev == nil || prev.CatalogVersion == "" || r.CatalogVersion == "" {
		return nil
	}
	if catalog.CompareVersions(prev.CatalogVersion, r.CatalogVersion) > 0 {
		return fmt.Errorf("%s was generated with catalog %s, newer than this cyanotype's catalog (%s); upgrade cyanotype",
			lock.Path, prev.CatalogVersion, r.CatalogVersion)
	}
	return nil
}

// renderLock returns cyanotype.lock for files, the other files Render
// returns, and the deletions of stale files; see docs/DESIGN.md, The
// generated-state file. prev is the lock in existing, or nil when the
// project has none yet.
//
// A file prev records that files no longer hold is stale. It is deleted
// only when it still has the header, holds no protected block and has the
// hash prev recorded, so the deletion loses nothing a person wrote. A
// stale file that changed, or holds a block, stays, with a warning, and
// stays in the lock, so the warning comes back until someone decides. One
// without the header, or missing, has become someone else's business and
// leaves the lock.
func renderLock(r *resolve.Result, existing fs.FS, prev *lock.Lock, files []File) ([]File, []resolve.Warning, error) {
	next := &lock.Lock{
		CatalogVersion: r.CatalogVersion,
		Entries:        make(map[string]string, len(r.Entries)),
		Files:          make(map[string]string, len(files)),
	}
	for _, e := range r.Entries {
		h, err := lock.EntryHash(catalogEntry(r, e))
		if err != nil {
			return nil, nil, err
		}
		next.Entries[e.ID] = h
	}
	generated := make(map[string]bool, len(files))
	for _, f := range files {
		generated[f.Path] = true
		if f.owned != nil {
			next.Claude = *f.owned
		}
		h, ok, err := ownedHash(f.Path, f.Content)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			next.Files[f.Path] = h
		}
	}

	var (
		out      []File
		warnings []resolve.Warning
	)
	if prev != nil {
		for _, p := range slices.Sorted(maps.Keys(prev.Files)) {
			if generated[p] {
				continue
			}
			f, w, keep, err := staleFile(existing, p, prev.Files[p])
			if err != nil {
				return nil, nil, err
			}
			if f != nil {
				out = append(out, *f)
			}
			warnings = append(warnings, w...)
			if keep {
				next.Files[p] = prev.Files[p]
			}
		}
	}

	unrecorded, err := unrecordedFiles(r, existing, prev, generated)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, unrecorded...)
	if prev == nil {
		w, err := unownedRules(r, existing)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, w...)
	}

	return append(out, File{Path: lock.Path, Content: next.Marshal()}), warnings, nil
}

// catalogEntry returns e as the catalog writes it, placeholders unfilled,
// which is what the lock hashes. A result built without a catalog falls
// back to the resolved entry.
func catalogEntry(r *resolve.Result, e resolve.Entry) catalog.Entry {
	if r.Catalog != nil {
		if c, ok := r.Catalog.Get(e.ID); ok {
			return c
		}
	}
	return e.Entry
}

// staleFile decides what happens to the file at p, which the previous lock
// recorded with hash and which is no longer generated: a deletion, a
// warning, and whether the new lock keeps recording it.
func staleFile(existing fs.FS, p, hash string) (del *File, warnings []resolve.Warning, keep bool, err error) {
	if existing == nil {
		return nil, nil, false, nil
	}
	if info, err := fs.Lstat(existing, p); err != nil || !info.Mode().IsRegular() {
		return nil, nil, false, nil // missing, or not a file cyanotype would touch
	}
	data, err := readExisting(existing, p)
	if err != nil || data == nil {
		return nil, nil, false, err
	}
	if !HasHeader(data) {
		return nil, nil, false, nil
	}
	warn := func(why string) []resolve.Warning {
		return []resolve.Warning{{File: p, Msg: "is no longer generated, but " + why +
			", so it was not deleted; delete it, or remove its header to keep it as your own"}}
	}
	blocks, blocksErr := parseCustomBlocks(data)
	now, _, hashErr := ownedHash(p, data)
	switch {
	case blocksErr != nil || hashErr != nil:
		return nil, warn("its cyanotype:custom markers do not match"), true, nil
	case len(blocks) > 0:
		return nil, warn("it holds a cyanotype:custom block"), true, nil
	case now != hash:
		return nil, warn("it changed since cyanotype generated it"), true, nil
	}
	return &File{Path: p, Delete: true}, nil, false, nil
}

// unrecordedFiles warns about Markdown files with the cyanotype header
// where cyanotype writes instruction files, the main files and the topic
// folder, that are not generated now and that no lock records: typically
// a topic file left by a split made before v0.3. Without a recorded hash
// nobody can tell whether it was edited, so it is never deleted.
func unrecordedFiles(r *resolve.Result, existing fs.FS, prev *lock.Lock, generated map[string]bool) ([]resolve.Warning, error) {
	if existing == nil {
		return nil, nil
	}
	candidates := []string{agentsPath, claudePath}
	entries, err := fs.ReadDir(existing, r.Spec.Output.TopicDir)
	if err == nil {
		for _, e := range entries {
			if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") {
				candidates = append(candidates, path.Join(r.Spec.Output.TopicDir, e.Name()))
			}
		}
	}
	var warnings []resolve.Warning
	for _, p := range candidates {
		if generated[p] || (prev != nil && prev.Files[p] != "") {
			continue
		}
		data, err := readExisting(existing, p)
		if err != nil {
			return nil, err
		}
		if HasHeader(data) {
			warnings = append(warnings, resolve.Warning{File: p, Msg: "has the cyanotype header but is not generated, and " +
				lock.Path + " does not record it, so cyanotype cannot tell whether someone edited it; delete it if it is no longer needed"})
		}
	}
	return warnings, nil
}

// unownedRules warns, on the first generation with a lock, that the rules
// already in .claude/settings.json are treated as the user's: without a
// lock nobody can tell which ones an older cyanotype added, and guessing
// could remove a rule someone wrote. It names the rules that come from
// catalog entries that are not selected, the leftovers an older cyanotype
// most likely added, and counts those that match generated rules.
func unownedRules(r *resolve.Result, existing fs.FS) ([]resolve.Warning, error) {
	old, err := readExisting(existing, settingsPath)
	if err != nil || old == nil {
		return nil, err
	}
	deny, ask, err := permissionLists(old)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", settingsPath, err)
	}
	var gen permissionRules
	if slices.Contains(r.Spec.Targets, spec.TargetClaude) {
		gen = claudeRules(r)
	}
	known := catalogRules(r)

	matching := 0
	var leftovers []string
	for _, list := range []struct {
		rules  []string
		groups [][]string
	}{{deny, gen.deny}, {ask, gen.ask}} {
		generated := slices.Concat(list.groups...)
		for _, rule := range slices.Compact(slices.Sorted(slices.Values(list.rules))) {
			switch {
			case slices.Contains(generated, rule):
				matching++
			case known[rule] && !slices.Contains(leftovers, rule):
				leftovers = append(leftovers, rule)
			}
		}
	}
	if matching == 0 && len(leftovers) == 0 {
		return nil, nil
	}

	var found []string
	if matching > 0 {
		found = append(found, fmt.Sprintf("%d match rules it generates", matching))
	}
	if len(leftovers) > 0 {
		quoted := make([]string, len(leftovers))
		for i, l := range leftovers {
			quoted[i] = fmt.Sprintf("%q", l)
		}
		found = append(found, fmt.Sprintf("%d come from catalog entries that are not selected (%s)", len(leftovers), strings.Join(quoted, ", ")))
	}
	msg := lock.Path + " does not exist yet, so cyanotype treats the rules already in this file as yours and never removes them: " +
		strings.Join(found, ", and ") + ". If an older cyanotype added them, delete them from the file and run cyanotype generate again; " +
		"it adds back the ones still generated, and removes them once they are not"
	return []resolve.Warning{{File: settingsPath, Msg: msg}}, nil
}

// catalogRules returns every permission rule some catalog entry can
// generate, with the topic folder rule of rule-maintenance-propose for the
// spec's topic folder.
func catalogRules(r *resolve.Result) map[string]bool {
	known := map[string]bool{"Edit(/" + r.Spec.Output.TopicDir + "/**)": true}
	if r.Catalog == nil {
		return known
	}
	for _, e := range r.Catalog.Entries() {
		if e.Enforce == nil || e.Enforce.Claude == nil {
			continue
		}
		c := e.Enforce.Claude
		for _, rule := range slices.Concat(c.Deny, c.Ask) {
			known[rule] = true
		}
		for _, with := range c.With {
			for _, rule := range with.Ask {
				known[rule] = true
			}
		}
	}
	return known
}
