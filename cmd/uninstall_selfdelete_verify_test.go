//go:build windows

package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScheduleSelfDeleteSpacedPath proves the detached self-delete helper:
//   - spawns under DETACHED_PROCESS and survives independently,
//   - correctly quotes a path containing spaces (the trackr repo + some
//     usernames live under spaced paths),
//   - deletes the target exe and rmdir's the now-empty bin/.trackr folders.
//
// Note: this uses an unlocked dummy file. The "delete a still-running exe"
// aspect relies on Windows releasing the file handle when the real process
// exits (OS behaviour, not our code); what this test covers is our spawn +
// delay + del + rmdir mechanism and its quoting.
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

	// Helper waits ~1s (ping -n 2) then deletes; poll up to 15s.
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
