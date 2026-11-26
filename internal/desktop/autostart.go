package desktop

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
	"strings"
)

// bundleID names the Mac app, and its LaunchAgent.
const bundleID = "com.hallstream.app"

// launchAgent is the LaunchAgent plist that opens the app bundle at sign-in.
func launchAgent(label, bundle string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	xml.EscapeText(&b, []byte(label))
	b.WriteString(`</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/open</string>
		<string>-a</string>
		<string>`)
	xml.EscapeText(&b, []byte(bundle))
	b.WriteString(`</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`)
	return b.Bytes()
}

// autostartEntry is the XDG autostart .desktop file for the executable.
func autostartEntry(exe string) []byte {
	quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return []byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Hallstream\n" +
		`Exec="` + quote.Replace(exe) + "\"\n" +
		"Path=" + filepath.Dir(exe) + "\n" +
		"X-GNOME-Autostart-enabled=true\n" +
		"Terminal=false\n")
}

// translocated reports whether macOS runs a quarantined copy from a
// read-only random folder, whose path would be gone after a restart.
func translocated(exe string) bool { return strings.Contains(exe, "/AppTranslocation/") }

// bundlePath is the .app folder around exe, or "" outside a bundle.
func bundlePath(exe string) string {
	i := strings.LastIndex(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return ""
	}
	return exe[:i+len(".app")]
}
