package desktop

import (
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// awkward has every character that has broken a message box somewhere.
const awkward = "-x & <b>\"club\"</b>\nline two"

func TestOsascriptArgs(t *testing.T) {
	for name, args := range map[string][]string{
		"alert": alertArgs(awkward, "critical"),
		"ask":   askArgs(awkward, "Não", "Sim"),
	} {
		i := slices.Index(args, "--")
		if i < 0 || args[i+1] != awkward {
			t.Errorf("%s: the message is not the first argument after --: %q", name, args)
			continue
		}
		for _, a := range args[:i] {
			if strings.Contains(a, "club") {
				t.Errorf("%s: the message is inside the script: %q", name, a)
			}
		}
		if !slices.ContainsFunc(args[:i], func(a string) bool { return strings.Contains(a, "activateIgnoringOtherApps:true") }) {
			t.Errorf("%s: the box would open behind other windows", name)
		}
	}
	if args := askArgs(awkward, "Não", "Sim"); !slices.Equal(args[len(args)-3:], []string{awkward, "Não", "Sim"}) {
		t.Errorf("ask arguments: %q", args)
	}
}

// TestOsascriptArgv runs osascript: the message reaches the script exactly,
// a leading "-" included, and "--" is not part of argv.
func TestOsascriptArgv(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is macOS only")
	}
	args := osascriptArgs([]string{"on run argv", "return (count of argv) & \"|\" & (item 1 of argv)", "end run"}, awkward)
	out, err := exec.Command("/usr/bin/osascript", args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSuffix(string(out), "\n"), "1, |, "+awkward; got != want {
		t.Errorf("osascript got %q, want %q", got, want)
	}
}

func TestZenityArgs(t *testing.T) {
	got := zenityArgs("--question", awkward, "--default-cancel")
	want := []string{"--question", "--no-markup", "--title=Hallstream", "--text=" + awkward, "--default-cancel"}
	if !slices.Equal(got, want) {
		t.Errorf("zenityArgs = %q, want %q", got, want)
	}
}

func TestZenityAnswer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	cases := []struct {
		name    string
		script  string
		yes     bool
		failure bool
	}{
		{"yes", "exit 0", true, false},
		{"no", "exit 1", false, false},
		{"timeout or error", "exit 5", false, true},
		{"crash", "kill -9 $$", false, true},
	}
	for _, c := range cases {
		yes, err := zenityAnswer(exec.Command("/bin/sh", "-c", c.script).Run())
		if yes != c.yes || (err != nil) != c.failure {
			t.Errorf("%s: yes %v, err %v", c.name, yes, err)
		}
	}
}

func TestNotifyArgs(t *testing.T) {
	got := notifyArgs(awkward)
	want := []string{"--app-name=Hallstream", "--urgency=critical", "--", "Hallstream", "-x &amp; &lt;b&gt;\"club\"&lt;/b&gt;\nline two"}
	if !slices.Equal(got, want) {
		t.Errorf("notifyArgs = %q, want %q", got, want)
	}
}
