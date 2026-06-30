//go:build windows

package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScheduleSelfDeleteSpacedPath exercises the real scheduleSelfDelete:
// detached helper, ping delay, and the cmd /s /c quoting fix, against a path
// that contains spaces. Proves the exe is deleted and the empty bin/.trackr
// dirs are rmdir'd. (Uses an unlocked dummy file; the "delete a running exe"
// part relies on Windows releasing the handle on process exit.)
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

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if _, err := os.Stat(exePath); !os.IsNotExist(err) {
		t.Fatalf("exe was NOT deleted by detached helper: %s still exists", exePath)
	}
	t.Logf("OK: detached helper deleted spaced-path exe: %s", exePath)
	if _, err := os.Stat(binDir); !os.IsNotExist(err) {
		t.Errorf("bin dir not removed by rmdir: %s", binDir)
	}
	if _, err := os.Stat(trackrDir); !os.IsNotExist(err) {
		t.Errorf("trackr dir not removed by rmdir: %s", trackrDir)
	}
}
