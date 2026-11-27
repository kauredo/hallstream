package obsapp

import "path/filepath"

// startArgs are the arguments for OBS on macOS and Linux. Never
// --minimize-to-tray: on OBS's first run that leaves its browser sources,
// the overlay among them, without a renderer.
func startArgs() []string { return []string{"--disable-shutdown-check"} }

// macCandidates are the places OBS.app is looked for, in order.
func macCandidates(home string) []string {
	return []string{"/Applications/OBS.app", filepath.Join(home, "Applications", "OBS.app")}
}

// flatpakID is OBS from Flathub.
const flatpakID = "com.obsproject.Studio"

// linuxConfigPath picks native OBS's config.json when its obs-studio folder
// exists, else the Flatpak's when that folder exists, else the native path.
func linuxConfigPath(configDir, home string, exists func(string) bool) string {
	native := filepath.Join(configDir, "obs-studio")
	flatpak := filepath.Join(home, ".var", "app", flatpakID, "config", "obs-studio")
	dir := native
	if !exists(native) && exists(flatpak) {
		dir = flatpak
	}
	return filepath.Join(dir, "plugin_config", "obs-websocket", "config.json")
}
