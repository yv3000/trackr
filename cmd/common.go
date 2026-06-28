package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"trackr/internal/db"
	"trackr/internal/filesystem"
	"trackr/internal/model"
	"trackr/internal/orphan"
	"trackr/internal/pkgmanager"
	"trackr/internal/registry"
	"trackr/internal/ui"
)

// runFullScan performs the complete system scan, reporting progress on status.
func runFullScan(status chan<- string) ui.ScanResult {
	status <- "Scanning package managers (pip, npm, yarn, pnpm, docker)..."
	pkg, pkgNotes := pkgmanager.ScanAll()

	status <- "Scanning registry for installed software..."
	exe, regNotes := registry.Scan()

	status <- "Scanning common install folders across all drives..."
	folders, fsNotes := filesystem.ScanFolders()

	status <- "Detecting orphaned installs..."
	regGhosts, folderGhosts := orphan.Detect(exe, folders)

	// Persist discovered package-manager items to the local history DB. We only
	// log pkg-manager items (the ones users actively install); EXE/registry
	// software predates trackr and is intentionally not recorded here.
	status <- "Saving discovered packages to trackr history..."
	if database, err := db.Open(); err == nil {
		defer database.Close()
		existing, _ := database.ListInstalls()
		seen := map[string]bool{}
		for _, e := range existing {
			seen[e.Tool+":"+e.Name] = true
		}
		for _, it := range pkg {
			key := it.Tool + ":" + it.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			database.AddInstall(db.Install{
				Name:    it.Name,
				Tool:    it.Tool,
				Command: it.Command,
			})
		}
	}

	notes := make([]string, 0, len(pkgNotes)+len(regNotes)+len(fsNotes))
	notes = append(notes, pkgNotes...)
	notes = append(notes, regNotes...)
	notes = append(notes, fsNotes...)

	return ui.ScanResult{
		Pkg:            pkg,
		Exe:            exe,
		Folders:        folders,
		RegistryGhosts: regGhosts,
		FolderGhosts:   folderGhosts,
		Notes:          notes,
	}
}

// sortItems orders items by size (desc) or name (asc).
func sortItems(items []model.Item, mode string) []model.Item {
	out := make([]model.Item, len(items))
	copy(out, items)
	switch mode {
	case "name":
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	default: // size
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].SizeBytes > out[j].SizeBytes
		})
	}
	return out
}

// combinedItems flattens a scan result into one searchable slice.
func combinedItems(res ui.ScanResult) []model.Item {
	all := make([]model.Item, 0, len(res.Pkg)+len(res.Exe)+len(res.Folders))
	all = append(all, res.Exe...)
	all = append(all, res.Pkg...)
	all = append(all, res.Folders...)
	return all
}

// matchItems returns every item whose name fuzzily contains the query.
func matchItems(items []model.Item, query string) []model.Item {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []model.Item
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Name), q) {
			out = append(out, it)
		}
	}
	return out
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func sourceLabel(it model.Item) string {
	switch it.Source {
	case model.SourceExe:
		return "exe"
	case model.SourceFolder:
		return "folder"
	default:
		return it.Tool
	}
}

// selectMatch resolves a possibly-ambiguous match list to a single item,
// prompting the user with an interactive list when there is more than one.
func selectMatch(matches []model.Item, action string) (*model.Item, error) {
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	}
	rows := make([]ui.Row, 0, len(matches))
	for i := range matches {
		it := matches[i]
		rows = append(rows, ui.Row{
			Item: &matches[i],
			Tone: ui.ToneForStatus(it.Status),
			Text: fmt.Sprintf("%-32s %-12s %-8s %10s",
				trunc(it.Name, 32), trunc(it.Version, 12), sourceLabel(it),
				model.FormatSize(it.SizeBytes)),
		})
	}
	return ui.RunList("Multiple matches — pick one to "+action, rows, true)
}

// confirm prints a prompt and returns true only for an explicit yes.
func confirm(prompt string) bool {
	fmt.Print(prompt + " ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// ---------------------------------------------------------------------------
// Safety: never delete from these locations.
// ---------------------------------------------------------------------------

var driveRootRe = regexp.MustCompile(`^[a-z]:\\?$`)

// isProtectedPath reports whether a path must never be deleted, with a reason.
func isProtectedPath(p string) (bool, string) {
	lp := strings.ToLower(strings.Trim(p, `"`))
	lp = strings.TrimRight(lp, `\`)
	if lp == "" {
		return true, "empty path"
	}
	if driveRootRe.MatchString(lp) || len(lp) <= 2 {
		return true, "drive root"
	}
	for _, sys := range []string{`\windows`, `:\windows\system32`, `:\windows\syswow64`} {
		if strings.Contains(lp, sys) {
			return true, "Windows system directory"
		}
	}
	if strings.Contains(lp, `\common files`) {
		return true, "Common Files (shared — needs manual review)"
	}
	// Refuse any exact common install root (e.g. a registry entry whose
	// InstallLocation is literally "C:\Program Files (x86)"). Deleting these
	// would wipe out every installed program, not just the target.
	for _, root := range filesystem.CommonInstallRoots() {
		nr := strings.ToLower(strings.TrimRight(root, `\`))
		if lp == nr {
			return true, "shared install root — would remove unrelated software"
		}
	}
	return false, ""
}

// ---------------------------------------------------------------------------
// Uninstall-string parsing.
// ---------------------------------------------------------------------------

var msiGUIDRe = regexp.MustCompile(`\{[0-9A-Fa-f\-]+\}`)

// parseUninstall turns a registry UninstallString into an argv slice and
// reports whether it is an MSI uninstall. A best-effort silent flag is added.
func parseUninstall(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	if strings.Contains(strings.ToLower(s), "msiexec") {
		if guid := msiGUIDRe.FindString(s); guid != "" {
			return []string{"msiexec", "/x", guid, "/quiet", "/norestart"}, true
		}
	}
	argv := tokenize(s)
	if len(argv) == 0 {
		return nil, false
	}
	hasFlag := false
	for _, a := range argv[1:] {
		switch strings.ToLower(a) {
		case "/s", "/silent", "/verysilent", "/quiet", "/qn":
			hasFlag = true
		}
	}
	if !hasFlag {
		if strings.Contains(strings.ToLower(argv[0]), "unins") {
			argv = append(argv, "/VERYSILENT", "/NORESTART")
		} else {
			argv = append(argv, "/S")
		}
	}
	return argv, false
}

// tokenize splits a command string into tokens, respecting double quotes.
func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// runArgv executes an external command and returns combined output + error.
func runArgv(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// needsElevation reports whether an error/output indicates admin rights needed.
func needsElevation(out string, err error) bool {
	s := strings.ToLower(out)
	if err != nil {
		s += " " + strings.ToLower(err.Error())
	}
	return strings.Contains(s, "access is denied") ||
		strings.Contains(s, "elevation") ||
		strings.Contains(s, "administrator") ||
		strings.Contains(s, "requested operation requires")
}
