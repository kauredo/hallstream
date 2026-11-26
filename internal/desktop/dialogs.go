package desktop

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
)

// The command lines for message boxes on macOS (osascript) and Linux
// (zenity, notify-send). They build on every system so their tests run on
// any of them. The message is always one argument of its own, never part
// of a script or an option, so no character in it changes what runs.

// osascriptArgs runs the script lines as one AppleScript. args reach its
// run handler as argv, after "--" so a leading "-" is never an option.
func osascriptArgs(script []string, args ...string) []string {
	a := make([]string, 0, 2*len(script)+1+len(args))
	for _, line := range script {
		a = append(a, "-e", line)
	}
	a = append(a, "--")
	return append(a, args...)
}

// inFront is the start of a script whose box opens in front of other apps'
// windows. AppleScript's own activate left the question behind the active
// browser after Quit in the Dock (checked on macOS 26), so the script calls
// AppKit's activateIgnoringOtherApps. Unlike telling System Events to bring
// osascript forward, that needs no Automation permission.
var inFront = []string{
	`use framework "AppKit"`,
	"use scripting additions",
	"on run argv",
	"current application's NSApplication's sharedApplication()'s activateIgnoringOtherApps:true",
}

// alertArgs shows msg in an alert of kind "critical" or "warning".
func alertArgs(msg, kind string) []string {
	return osascriptArgs(append(slices.Clone(inFront),
		`display alert "Hallstream" message (item 1 of argv) as `+kind,
		"end run",
	), msg)
}

// askArgs asks msg with the buttons no and yes, no the default.
func askArgs(msg, no, yes string) []string {
	return osascriptArgs(append(slices.Clone(inFront),
		`display dialog (item 1 of argv) with title "Hallstream" buttons {(item 2 of argv), (item 3 of argv)} default button (item 2 of argv) with icon caution`,
		"end run",
	), msg, no, yes)
}

// zenityArgs shows a zenity box of kind ("--question", "--error",
// "--warning"). --no-markup keeps zenity from reading msg as Pango markup,
// where a "<" or "&" would break the box.
func zenityArgs(kind, msg string, extra ...string) []string {
	return append([]string{kind, "--no-markup", "--title=Hallstream", "--text=" + msg}, extra...)
}

// zenityAnswer reads how zenity ended: yes on 0, no on 1 (No, Escape or
// the window closed), and an error for anything else, a crash included.
func zenityAnswer(err error) (yes bool, _ error) {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	}
	return false, err
}

// notifyArgs shows msg as a desktop notification. Notification servers
// read the body as markup, so &, < and > are escaped.
func notifyArgs(msg string) []string {
	body := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(msg)
	return []string{"--app-name=Hallstream", "--urgency=critical", "--", "Hallstream", body}
}
