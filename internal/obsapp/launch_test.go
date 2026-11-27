package obsapp

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestStartArgs(t *testing.T) {
	if slices.Contains(startArgs(), "--minimize-to-tray") {
		t.Error("a minimised first run leaves the overlay without a renderer")
	}
}

func TestMacCandidates(t *testing.T) {
	want := []string{"/Applications/OBS.app", filepath.FromSlash("/Users/a/Applications/OBS.app")}
	if got := macCandidates("/Users/a"); !reflect.DeepEqual(got, want) {
		t.Errorf("%q", got)
	}
}

func TestLinuxConfigPath(t *testing.T) {
	const config, home = "/home/a/.config", "/home/a"
	native := filepath.FromSlash("/home/a/.config/obs-studio")
	flatpak := filepath.FromSlash("/home/a/.var/app/com.obsproject.Studio/config/obs-studio")
	tail := filepath.FromSlash("/plugin_config/obs-websocket/config.json")
	has := func(dirs ...string) func(string) bool {
		return func(p string) bool { return slices.Contains(dirs, p) }
	}
	cases := []struct {
		exists func(string) bool
		want   string
	}{
		{has(flatpak), flatpak + tail},
		{has(native, flatpak), native + tail},
		{has(native), native + tail},
		{has(), native + tail},
	}
	for i, c := range cases {
		if got := linuxConfigPath(config, home, c.exists); got != c.want {
			t.Errorf("case %d: %q, want %q", i, got, c.want)
		}
	}
}
