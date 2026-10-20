package detect

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// packageJSON is what detection reads from a package.json.
type packageJSON struct {
	// dependencies are the package names of every dependency list, sorted
	// and without duplicates.
	dependencies []string
	// scripts are the names of the scripts, sorted.
	scripts []string
}

// parsePackageJSON reads the dependency lists and the script names of a
// package.json. Only the keys matter, so values of any type are accepted:
// a malformed version string is npm's problem, not detection's.
func parsePackageJSON(data []byte) (packageJSON, error) {
	var raw struct {
		Dependencies         map[string]json.RawMessage `json:"dependencies"`
		DevDependencies      map[string]json.RawMessage `json:"devDependencies"`
		OptionalDependencies map[string]json.RawMessage `json:"optionalDependencies"`
		PeerDependencies     map[string]json.RawMessage `json:"peerDependencies"`
		Scripts              map[string]json.RawMessage `json:"scripts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return packageJSON{}, fmt.Errorf("invalid JSON: %w", err)
	}
	names := map[string]bool{}
	for _, list := range []map[string]json.RawMessage{raw.Dependencies, raw.DevDependencies, raw.OptionalDependencies, raw.PeerDependencies} {
		for name := range list {
			names[name] = true
		}
	}
	return packageJSON{
		dependencies: slices.Sorted(maps.Keys(names)),
		scripts:      slices.Sorted(maps.Keys(raw.Scripts)),
	}, nil
}
