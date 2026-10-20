// Package detect inspects an existing repository and suggests catalog entries.
//
// Detection is driven by each entry's detect field (files or dependencies
// that reveal a technology), so new stacks need no code changes here. The
// package only knows how to read each kind of manifest: package.json,
// pyproject.toml, requirements.txt and go.mod at the project root.
//
// Detection only suggests. It never writes anything, and every interface
// shows what it found, and why, before the user confirms.
package detect
