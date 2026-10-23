package mcpserver

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// layout creates, in a temporary folder:
//
//	allowed/          the allowed folder, where the server starts
//	allowed/sub/      a project inside it
//	allowed/file.txt  a file, not a folder
//	allowed-evil/     a sibling whose name starts with "allowed"
//	outside/          a folder outside it
//
// and returns the temporary folder.
func layout(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, d := range []string{"allowed/sub", "allowed-evil", "outside"} {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "allowed", "file.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return tmp
}

// sameFolder fails unless r is the folder at path.
func sameFolder(t *testing.T, r *os.Root, path string) {
	t.Helper()
	got, err := r.Stat(".")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(got, want) {
		t.Errorf("opened %s, want %s", r.Name(), path)
	}
}

func openFolders(t *testing.T, start string, dirs ...string) *Folders {
	t.Helper()
	f, err := OpenFolders(start, dirs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestOpen(t *testing.T) {
	tmp := layout(t)
	allowed := filepath.Join(tmp, "allowed")
	f := openFolders(t, allowed)

	type openCase struct {
		name string
		dir  string
		want string // the folder opened; empty when Open must fail
		err  error  // the error Open wraps, when it fails with one
		msg  string // part of the message, when it fails with no sentinel
	}
	tests := []openCase{
		{name: "default", dir: "", want: allowed},
		{name: "dot", dir: ".", want: allowed},
		{name: "relative", dir: "sub", want: filepath.Join(allowed, "sub")},
		{name: "absolute", dir: filepath.Join(allowed, "sub"), want: filepath.Join(allowed, "sub")},
		{name: "dot dot back in", dir: filepath.Join("sub", "..", "sub"), want: filepath.Join(allowed, "sub")},
		{name: "parent", dir: "..", err: ErrOutside},
		{name: "dot dot out", dir: filepath.Join("sub", "..", "..", "outside"), err: ErrOutside},
		{name: "absolute outside", dir: filepath.Join(tmp, "outside"), err: ErrOutside},
		{name: "similar prefix", dir: filepath.Join(tmp, "allowed-evil"), err: ErrOutside},
		{name: "missing outside", dir: filepath.Join(tmp, "nothing"), err: ErrOutside},
		{name: "missing inside", dir: "nothing", msg: "no such folder"},
		{name: "file", dir: "file.txt", msg: "not a folder"},
		{name: "NUL byte", dir: "sub\x00", err: ErrInvalidPath},
		{name: "device path", dir: `\\?\` + allowed, err: ErrInvalidPath},
		{name: "device namespace", dir: `\\.\` + allowed, err: ErrInvalidPath},
		{name: "network path", dir: `\\server\share\project`, err: ErrInvalidPath},
		{name: "network path with slashes", dir: `//server/share/project`, err: ErrInvalidPath},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests,
			openCase{name: "other case", dir: strings.ToUpper(filepath.Join(allowed, "sub")), want: filepath.Join(allowed, "sub")},
			openCase{name: "drive relative", dir: filepath.VolumeName(allowed) + "sub", err: ErrInvalidPath},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := f.Open(tt.dir)
			if tt.want != "" {
				if err != nil {
					t.Fatalf("Open(%q) = %v, want %s", tt.dir, err, tt.want)
				}
				defer r.Close()
				sameFolder(t, r, tt.want)
				return
			}
			if err == nil {
				_ = r.Close()
				t.Fatalf("Open(%q) opened %s, want an error", tt.dir, r.Name())
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("Open(%q) = %v, want %v", tt.dir, err, tt.err)
			}
			if tt.msg != "" && !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("Open(%q) = %v, want %q in it", tt.dir, err, tt.msg)
			}
		})
	}
}

// TestOpenLinks checks that a link inside an allowed folder cannot lead a
// tool out of it, and that a link from outside to an allowed folder works.
func TestOpenLinks(t *testing.T) {
	tmp := layout(t)
	allowed := filepath.Join(tmp, "allowed")
	outside := filepath.Join(tmp, "outside")
	f := openFolders(t, allowed)

	t.Run("symlink out", func(t *testing.T) {
		link := filepath.Join(allowed, "out")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("cannot create symbolic links here: %v", err)
		}
		for _, dir := range []string{"out", filepath.Join(allowed, "out")} {
			if r, err := f.Open(dir); !errors.Is(err, ErrOutside) {
				if err == nil {
					_ = r.Close()
				}
				t.Errorf("Open(%q) = %v, want %v", dir, err, ErrOutside)
			}
		}
	})

	t.Run("symlink in", func(t *testing.T) {
		link := filepath.Join(outside, "in")
		if err := os.Symlink(filepath.Join(allowed, "sub"), link); err != nil {
			t.Skipf("cannot create symbolic links here: %v", err)
		}
		r, err := f.Open(link)
		if err != nil {
			t.Fatalf("Open(%q) = %v, want the project it links to", link, err)
		}
		defer r.Close()
		sameFolder(t, r, filepath.Join(allowed, "sub"))
	})

	t.Run("junction out", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions exist only on Windows")
		}
		link := filepath.Join(allowed, "junction")
		// mklink /J needs no privilege, unlike a symbolic link.
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil { //nolint:gosec // fixed test paths
			t.Skipf("mklink /J: %v: %s", err, out)
		}
		for _, dir := range []string{"junction", filepath.Join("junction", "..", "junction")} {
			if r, err := f.Open(dir); !errors.Is(err, ErrOutside) {
				if err == nil {
					_ = r.Close()
				}
				t.Errorf("Open(%q) = %v, want %v", dir, err, ErrOutside)
			}
		}
	})
}

func TestOpenSeveralFolders(t *testing.T) {
	tmp := layout(t)
	allowed := filepath.Join(tmp, "allowed")
	outside := filepath.Join(tmp, "outside")
	f := openFolders(t, allowed, ".", outside)

	if got := len(f.Names()); got != 2 {
		t.Errorf("Names() = %v, want the start folder once and outside", f.Names())
	}
	if r, err := f.Open(""); !errors.Is(err, ErrDirRequired) {
		if err == nil {
			_ = r.Close()
		}
		t.Errorf(`Open("") = %v, want %v`, err, ErrDirRequired)
	}
	for _, dir := range []string{"sub", outside} {
		r, err := f.Open(dir)
		if err != nil {
			t.Errorf("Open(%q) = %v, want it opened", dir, err)
			continue
		}
		_ = r.Close()
	}
	if r, err := f.Open(filepath.Join(tmp, "allowed-evil")); !errors.Is(err, ErrOutside) {
		if err == nil {
			_ = r.Close()
		}
		t.Errorf("Open(allowed-evil) = %v, want %v", err, ErrOutside)
	}
}

func TestOpenFoldersRefuses(t *testing.T) {
	tmp := layout(t)
	allowed := filepath.Join(tmp, "allowed")
	for _, tt := range []struct {
		name  string
		start string
		dirs  []string
	}{
		{"relative start", "allowed", nil},
		{"missing", allowed, []string{"nothing"}},
		{"file", allowed, []string{"file.txt"}},
		{"network path", allowed, []string{`\\server\share`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if f, err := OpenFolders(tt.start, tt.dirs); err == nil {
				_ = f.Close()
				t.Errorf("OpenFolders(%q, %q) succeeded, want an error", tt.start, tt.dirs)
			}
		})
	}
}
