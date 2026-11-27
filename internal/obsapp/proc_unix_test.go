//go:build darwin || linux

package obsapp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPidsIn(t *testing.T) {
	proc := t.TempDir()
	for dir, comm := range map[string]string{
		"12":   "obs\n",
		"13":   "obs-ffmpeg-mux\n",
		"self": "obs\n", // not a pid
		"14":   "",      // no comm: the process has just gone
	} {
		if err := os.Mkdir(filepath.Join(proc, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if comm != "" {
			if err := os.WriteFile(filepath.Join(proc, dir, "comm"), []byte(comm), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if ids, err := pidsIn(proc, "obs", os.Getuid()); err != nil || !slices.Equal(ids, []int{12}) {
		t.Errorf("own processes: %v, %v", ids, err)
	}
	if ids, err := pidsIn(proc, "obs", os.Getuid()+1); err != nil || ids != nil {
		t.Errorf("another user's OBS was found: %v, %v", ids, err)
	}
	if ids, err := pidsIn(proc, "OBS", os.Getuid()); err != nil || ids != nil {
		t.Errorf("no match: %v, %v", ids, err)
	}
}
