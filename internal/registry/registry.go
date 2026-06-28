// Package registry scans the four Windows "Uninstall" registry locations that
// back Add/Remove Programs and exposes helpers to delete an uninstall key.
package registry

import (
	"strings"

	"golang.org/x/sys/windows/registry"

	"trackr/internal/filesystem"
	"trackr/internal/model"
)

// uninstallRoot describes one registry hive + path to enumerate.
type uninstallRoot struct {
	hive  registry.Key
	path  string
	label string
}

func roots() []uninstallRoot {
	return []uninstallRoot{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM"},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKCU"},
		{registry.CURRENT_USER, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, "HKCU"},
	}
}

// Scan enumerates every uninstall entry across all four registry locations.
// It returns the discovered items plus notes describing any access failures.
func Scan() ([]model.Item, []string) {
	var items []model.Item
	var notes []string

	for _, r := range roots() {
		k, err := registry.OpenKey(r.hive, r.path, registry.READ)
		if err != nil {
			// Key simply may not exist on this machine - that is fine.
			continue
		}
		subkeys, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			notes = append(notes, "Registry access denied: "+r.label+`\`+r.path)
			continue
		}

		for _, sk := range subkeys {
			full := r.path + `\` + sk
			sub, err := registry.OpenKey(r.hive, full, registry.READ)
			if err != nil {
				continue
			}

			name, _, _ := sub.GetStringValue("DisplayName")
			if strings.TrimSpace(name) == "" {
				sub.Close()
				continue
			}
			version, _, _ := sub.GetStringValue("DisplayVersion")
			loc, _, _ := sub.GetStringValue("InstallLocation")
			uninstall, _, _ := sub.GetStringValue("UninstallString")
			bundleCache, _, _ := sub.GetStringValue("BundleCachePath")
			publisher, _, _ := sub.GetStringValue("Publisher")
			instDate, _, _ := sub.GetStringValue("InstallDate")
			estKB, _, _ := sub.GetIntegerValue("EstimatedSize")
			sysComp, _, _ := sub.GetIntegerValue("SystemComponent")
			parentKey, _, _ := sub.GetStringValue("ParentKeyName")
			sub.Close()

			// Update entries (KB patches) register as children — always hide them.
			if parentKey != "" {
				continue
			}
			// SystemComponent is a soft filter: only hide entries that look like
			// genuine OS components (no real publisher / no install location).
			// Otherwise keep them but flag for display.
			isSystemComponent := false
			if sysComp == 1 {
				lowPub := strings.ToLower(publisher)
				if publisher == "" || strings.Contains(lowPub, "microsoft") || strings.Trim(loc, `"`) == "" {
					continue
				}
				isSystemComponent = true
			}

			item := model.Item{
				Name:            name,
				Tool:            model.ToolExe,
				Source:          model.SourceExe,
				Version:         version,
				InstallDir:      strings.Trim(loc, `"`),
				UninstallString: uninstall,
				Publisher:       publisher,
				InstallDate:     instDate,
				RegistryKey:     r.label + `\` + full,
				SystemComponent: isSystemComponent,
			}

			// Prefer actual folder size; fall back to the registry estimate.
			if item.InstallDir != "" && filesystem.Exists(item.InstallDir) {
				if sz, e := filesystem.DirSize(item.InstallDir); e == nil && sz > 0 {
					item.SizeBytes = sz
				}
			}
			if item.SizeBytes == 0 && estKB > 0 {
				item.SizeBytes = int64(estKB) * 1024
			}

			// Store / UWP apps live under WindowsApps, reference the Store via
			// their uninstall string, or carry a BundleCachePath value. None can
			// be removed normally — flag them so the UI points users to winget.
			if strings.Contains(strings.ToLower(item.InstallDir), "windowsapps") ||
				strings.Contains(strings.ToLower(item.UninstallString), "ms-windows-store:") ||
				strings.Contains(strings.ToLower(item.UninstallString), "windowsstore") ||
				bundleCache != "" {
				item.StoreApp = true
			}

			switch {
			case strings.TrimSpace(uninstall) == "" && !item.StoreApp:
				item.Status = model.StatusNoUninstall
			default:
				item.Status = model.StatusClean
			}

			items = append(items, item)
		}
	}
	return items, notes
}

// IsHKLM reports whether the trackr-formatted key lives under HKEY_LOCAL_MACHINE
// (deleting such a key requires administrator rights).
func IsHKLM(trackrKey string) bool {
	return strings.HasPrefix(trackrKey, `HKLM\`)
}

// CanWrite verifies the key can be opened with write access, which is required
// to delete it. Returns the underlying error (e.g. permission denied) on failure.
func CanWrite(trackrKey string) error {
	hive := registry.LOCAL_MACHINE
	path := trackrKey
	switch {
	case strings.HasPrefix(trackrKey, `HKLM\`):
		hive = registry.LOCAL_MACHINE
		path = strings.TrimPrefix(trackrKey, `HKLM\`)
	case strings.HasPrefix(trackrKey, `HKCU\`):
		hive = registry.CURRENT_USER
		path = strings.TrimPrefix(trackrKey, `HKCU\`)
	}
	k, err := registry.OpenKey(hive, path, registry.WRITE)
	if err != nil {
		return err
	}
	k.Close()
	return nil
}

// DeleteKey removes an uninstall registry key given its trackr-formatted path
// (e.g. "HKLM\SOFTWARE\...\Uninstall\SomeApp"). Returns an error on failure.
func DeleteKey(trackrKey string) error {
	hive := registry.LOCAL_MACHINE
	path := trackrKey
	switch {
	case strings.HasPrefix(trackrKey, `HKLM\`):
		hive = registry.LOCAL_MACHINE
		path = strings.TrimPrefix(trackrKey, `HKLM\`)
	case strings.HasPrefix(trackrKey, `HKCU\`):
		hive = registry.CURRENT_USER
		path = strings.TrimPrefix(trackrKey, `HKCU\`)
	}
	return registry.DeleteKey(hive, path)
}

// IsUninstallKey reports whether a key sits safely under an Uninstall path,
// guarding against accidental deletion of unrelated system keys.
func IsUninstallKey(trackrKey string) bool {
	return strings.Contains(strings.ToLower(trackrKey), `\uninstall\`)
}
