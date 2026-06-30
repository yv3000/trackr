package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// removeFromUserPath strips dir from the current user's PATH environment
// variable in the registry (HKCU\Environment). No live WM_SETTINGCHANGE
// broadcast is needed for a CLI tool: the next shell session re-reads PATH.
func removeFromUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	current, valType, err := k.GetStringValue("PATH")
	if err != nil {
		return err
	}

	parts := strings.Split(current, ";")
	var kept []string
	target := strings.TrimRight(strings.ToLower(dir), `\/`)
	for _, p := range parts {
		if strings.TrimRight(strings.ToLower(strings.TrimSpace(p)), `\/`) != target {
			kept = append(kept, p)
		}
	}
	newPath := strings.Join(kept, ";")

	// Preserve the original value type. User PATH is usually REG_EXPAND_SZ so
	// entries like %USERPROFILE%\... keep expanding; writing it back as plain
	// REG_SZ would freeze those tokens as literal (broken) paths.
	if valType == registry.EXPAND_SZ {
		return k.SetExpandStringValue("PATH", newPath)
	}
	return k.SetStringValue("PATH", newPath)
}

// scheduleSelfDelete spawns a detached helper process that waits for this
// process to exit, then deletes the exe and cleans up empty parent dirs.
// Uses `cmd /c` with a ping-based delay (no extra dependency needed) and
// `rmdir`/`del` to clean up, fully detached so it survives this process exit.
func scheduleSelfDelete(exePath, binDir, trackrDir string) error {
	// ping -n 2 gives ~1 second delay, enough for this process to fully exit
	// and release the file handle on the exe.
	script := fmt.Sprintf(
		`ping -n 2 127.0.0.1 >nul & del /f /q "%s" & rmdir "%s" 2>nul & rmdir "%s" 2>nul`,
		exePath, binDir, trackrDir,
	)
	cmd := exec.Command("cmd", "/c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
	return cmd.Start()
}
