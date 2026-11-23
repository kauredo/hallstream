package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marcador.log")
	l, err := Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("a", 39) + "\n" // 40 bytes
	for i := 0; i < 2; i++ {
		l.Write([]byte(line))
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rotated too early: %v", err)
	}
	l.Write([]byte("b\n")) // 82 bytes: still fits
	l.Write([]byte(line))  // 122 > 100: rotates first
	l.Close()

	old, _ := os.ReadFile(path + ".1")
	cur, _ := os.ReadFile(path)
	if string(old) != line+line+"b\n" {
		t.Errorf("old file = %q", old)
	}
	if string(cur) != line {
		t.Errorf("new file = %q", cur)
	}
}

func TestKeepsSizeAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marcador.log")
	os.WriteFile(path, []byte(strings.Repeat("x", 90)), 0o644)
	l, err := Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	l.Write([]byte("0123456789\n"))
	l.Close()
	if b, _ := os.ReadFile(path); string(b) != "0123456789\n" {
		t.Errorf("the size before the restart was not counted: %q", b)
	}
	if b, _ := os.ReadFile(path + ".1"); len(b) != 90 {
		t.Errorf("old file has %d bytes, want 90", len(b))
	}
}

func TestOneLineLargerThanTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marcador.log")
	l, _ := Open(path, 10)
	defer l.Close()
	if n, err := l.Write([]byte(strings.Repeat("z", 30))); err != nil || n != 30 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Error("an empty file was rotated")
	}
}

func TestWriteAfterClose(t *testing.T) {
	l, _ := Open(filepath.Join(t.TempDir(), "m.log"), 10)
	l.Close()
	if _, err := l.Write([]byte("x")); err == nil {
		t.Error("write after Close succeeded")
	}
}

func TestReopensAfterAFailedRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marcador.log")
	l, _ := Open(path, 10)
	defer l.Close()
	l.Write([]byte("0123456789"))
	// A folder where the log should be makes the reopen fail once.
	os.Rename(path, path+".1")
	os.Mkdir(path, 0o755)
	l.f.Close()
	l.f = nil
	if _, err := l.Write([]byte("x")); err == nil {
		t.Fatal("write into a folder succeeded")
	}
	os.Remove(path)
	if _, err := l.Write([]byte("back\n")); err != nil {
		t.Fatalf("logging never came back: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "back\n" {
		t.Errorf("log = %q", b)
	}
}
