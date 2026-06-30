package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScheduleSelfDeleteSpacedPath exercises the real scheduleSelfDelete end to
// end: the detached helper, the ping delay, and the `cmd /s /c` quoting, using
// a path that contains spaces (regression guard for the bug where Go-escaped
// quotes \" made cmd.exe fail with "...syntax is incorrect" and skip deletion).
//
// It uses an unlocked dummy file; the "delete a still-running exe" aspect
// relies on Windows releasing the file handle when the real process exits,
// which is OS behaviour, not something this code controls.
func TestScheduleSelfDeleteSpacedPath(t *testing.T) {
	base := t.TempDir()
	trackrDir := filepath.Join(base, "user with space", ".trackr")
	binDir := filepath.Join(trackrDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exePath := filepath.Join(binDir, "trackr.exe")
	if err := os.WriteFile(exePath, []byte("dummy exe"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := scheduleSelfDelete(exePath, binDir, trackrDir); err != nil {
		t.Fatalf("scheduleSelfDelete returned error: %v", err)
	}

	gone := func(p string) bool { _, err := os.Stat(p); return os.IsNotExist(err) }

	// Helper waits ~1s (ping -n 2) before deleting; poll generously.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && !gone(exePath) {
		time.Sleep(200 * time.Millisecond)
	}
	if !gone(exePath) {
		t.Fatalf("exe was NOT deleted by detached helper: %s still exists", exePath)
	}

	// Empty-dir cleanup (rmdir) runs right after del in the same helper; give
	// it a brief moment so the assertion isn't racing the sequential commands.
	for time.Now().Before(deadline) && !gone(binDir) {
		time.Sleep(100 * time.Millisecond)
	}
	if !gone(binDir) {
		t.Errorf("bin dir not removed by rmdir: %s", binDir)
	}
}
