package desktop

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kauredo/hallstream/internal/i18n"
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

// undeleted is exe without the " (deleted)" Linux adds to the path of a
// program replaced while it runs.
func undeleted(exe string) string { return strings.TrimSuffix(exe, " (deleted)") }

// Desktop Entry spec escapes. Exec is quoted: inside the quotes ", `, $
// and \ take a backslash, and % is %% so it is not a field code. The
// string escapes then apply to every value, Exec included, so each of
// those backslashes, and any in Path, is doubled: a \ in the path is \\\\
// in Exec.
var (
	execQuote = strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`, `%`, `%%`)
	strEscape = strings.NewReplacer(`\`, `\\`)
)

// autostartEntry is the XDG autostart .desktop file for the executable. A
// path with a control character in it can't be written there.
func autostartEntry(exe string) ([]byte, error) {
	exe = undeleted(exe)
	if strings.ContainsFunc(exe, unicode.IsControl) {
		return nil, i18n.Err("app.arranque_caminho")
	}
	return []byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Hallstream\n" +
		`Exec="` + execQuote.Replace(exe) + "\"\n" +
		"Path=" + strEscape.Replace(filepath.Dir(exe)) + "\n" +
		"X-GNOME-Autostart-enabled=true\n" +
		"Terminal=false\n"), nil
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
