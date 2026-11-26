//go:build darwin || linux

package obsapp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// pidsIn lists the processes in proc (Linux's /proc) whose comm is name
// and whose folder belongs to uid: another user's OBS on the same PC is
// not ours to close. Only Linux has /proc; this builds on macOS too so its
// test runs there.
func pidsIn(proc, name string, uid int) ([]int, error) {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, e := range entries {
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(proc, e.Name())
		st, err := os.Stat(dir)
		if err != nil {
			continue
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); !ok || int(sys.Uid) != uid {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err == nil && strings.TrimSpace(string(comm)) == name {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
