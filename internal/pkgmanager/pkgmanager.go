// Package pkgmanager discovers packages installed through pip, npm, yarn, pnpm
// and docker by shelling out to each tool and parsing its JSON output. Missing
// tools are skipped silently (returned as a note), never as a hard error.
package pkgmanager

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"trackr/internal/filesystem"
	"trackr/internal/model"
)

// commandExists reports whether a tool is resolvable on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// run executes a tool, transparently routing .cmd/.bat shims (npm, yarn, pnpm)
// through cmd.exe so CreateProcess can launch them. Returns stdout, stderr, err.
func run(name string, args ...string) (string, string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", "", err
	}
	var cmd *exec.Cmd
	lp := strings.ToLower(path)
	if strings.HasSuffix(lp, ".cmd") || strings.HasSuffix(lp, ".bat") {
		cmd = exec.Command("cmd", append([]string{"/c", name}, args...)...)
	} else {
		cmd = exec.Command(path, args...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ScanAll runs every package-manager scanner concurrently and merges results.
// The second return value is a slice of human-readable notes (skipped tools,
// docker daemon down, etc.).
func ScanAll() ([]model.Item, []string) {
	type result struct {
		items []model.Item
		notes []string
	}
	scanners := []func() ([]model.Item, []string){
		scanPip, scanNpm, scanYarn, scanPnpm, scanDocker,
	}

	results := make([]result, len(scanners))
	var wg sync.WaitGroup
	for i, fn := range scanners {
		wg.Add(1)
		go func(i int, fn func() ([]model.Item, []string)) {
			defer wg.Done()
			it, nt := fn()
			results[i] = result{it, nt}
		}(i, fn)
	}
	wg.Wait()

	var items []model.Item
	var notes []string
	for _, r := range results {
		items = append(items, r.items...)
		notes = append(notes, r.notes...)
	}
	return items, notes
}

func scanPip() ([]model.Item, []string) {
	if !commandExists("pip") {
		return nil, []string{"pip not found — skipped"}
	}
	// Try user-installed packages first (more relevant); fall back to the full
	// list when empty/erroring (e.g. running inside a virtualenv).
	out, _, err := run("pip", "list", "--user", "--format=json")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		out, _, err = run("pip", "list", "--format=json")
		if err != nil || strings.TrimSpace(out) == "" {
			return nil, []string{"pip scan failed — skipped"}
		}
	}
	var pkgs []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(out), &pkgs) != nil {
		return nil, []string{"pip output unparseable — skipped"}
	}
	// Filter out always-noisy base packages that users never install on purpose.
	noisy := map[string]bool{
		"pip": true, "setuptools": true, "wheel": true,
		"distribute": true, "pkg_resources": true, "pkg-resources": true,
	}
	var items []model.Item
	for _, p := range pkgs {
		if noisy[strings.ToLower(p.Name)] {
			continue
		}
		items = append(items, model.Item{
			Name:       p.Name,
			Tool:       model.ToolPip,
			Version:    p.Version,
			Source:     model.SourcePkg,
			Status:     model.StatusPkg,
			Command:    "pip install " + p.Name,
			RemoveArgs: []string{"pip", "uninstall", "-y", p.Name},
		})
	}

	// Calculate pip package sizes concurrently.
	// First collect locations via `pip show` for each package.
	type pipLoc struct {
		idx      int
		location string
	}
	locCh := make(chan pipLoc, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for i, it := range items {
		wg.Add(1)
		go func(idx int, pkgName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			loc := PipLocation(pkgName)
			locCh <- pipLoc{idx, loc}
		}(i, it.Name)
	}

	go func() {
		wg.Wait()
		close(locCh)
	}()

	for pl := range locCh {
		if pl.location != "" {
			sz := pipPackageSize(items[pl.idx].Name, pl.location)
			items[pl.idx].SizeBytes = sz
		}
	}

	return items, nil
}

// pipPackageSize estimates the on-disk size of a pip package given the
// site-packages location returned by `pip show`.
func pipPackageSize(name, location string) int64 {
	if location == "" {
		return 0
	}
	// pip stores packages under the normalized name (hyphens → underscores).
	normalized := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	candidates := []string{
		filepath.Join(location, normalized),
		filepath.Join(location, strings.ToLower(name)),
		filepath.Join(location, name),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if sz, err := filesystem.DirSize(p); err == nil && sz > 0 {
				return sz
			}
		}
	}
	return 0
}

func scanNpm() ([]model.Item, []string) {
	if !commandExists("npm") {
		return nil, []string{"npm not found — skipped"}
	}
	out, _, _ := run("npm", "list", "-g", "--depth=0", "--json")
	if strings.TrimSpace(out) == "" {
		return nil, []string{"npm scan failed — skipped"}
	}
	var parsed struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if json.Unmarshal([]byte(out), &parsed) != nil {
		return nil, []string{"npm output unparseable — skipped"}
	}
	var items []model.Item
	for name, d := range parsed.Dependencies {
		items = append(items, model.Item{
			Name:       name,
			Tool:       model.ToolNpm,
			Version:    d.Version,
			Source:     model.SourcePkg,
			Status:     model.StatusPkg,
			Command:    "npm install -g " + name,
			RemoveArgs: []string{"npm", "uninstall", "-g", name},
		})
	}
	return items, nil
}

var yarnPkgRe = regexp.MustCompile(`"([^"@]+)@([^"]+)"`)

func scanYarn() ([]model.Item, []string) {
	if !commandExists("yarn") {
		return nil, []string{"yarn not found — skipped"}
	}
	out, _, _ := run("yarn", "global", "list", "--json")
	if strings.TrimSpace(out) == "" {
		return nil, []string{"yarn scan failed — skipped"}
	}
	var items []model.Item
	seen := map[string]bool{}
	// yarn emits newline-delimited JSON objects; the "info" lines carry the
	// "package@version" strings we want.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		for _, m := range yarnPkgRe.FindAllStringSubmatch(obj.Data, -1) {
			name, version := m[1], m[2]
			if seen[name] {
				continue
			}
			seen[name] = true
			items = append(items, model.Item{
				Name:       name,
				Tool:       model.ToolYarn,
				Version:    version,
				Source:     model.SourcePkg,
				Status:     model.StatusPkg,
				Command:    "yarn global add " + name,
				RemoveArgs: []string{"yarn", "global", "remove", name},
			})
		}
	}
	return items, nil
}

func scanPnpm() ([]model.Item, []string) {
	if !commandExists("pnpm") {
		return nil, []string{"pnpm not found — skipped"}
	}
	out, _, _ := run("pnpm", "list", "-g", "--json")
	if strings.TrimSpace(out) == "" {
		return nil, []string{"pnpm scan failed — skipped"}
	}
	// pnpm returns an array of project objects, each with a dependencies map.
	var projects []struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if json.Unmarshal([]byte(out), &projects) != nil {
		return nil, []string{"pnpm output unparseable — skipped"}
	}
	var items []model.Item
	seen := map[string]bool{}
	for _, proj := range projects {
		for name, d := range proj.Dependencies {
			if seen[name] {
				continue
			}
			seen[name] = true
			items = append(items, model.Item{
				Name:       name,
				Tool:       model.ToolPnpm,
				Version:    d.Version,
				Source:     model.SourcePkg,
				Status:     model.StatusPkg,
				Command:    "pnpm add -g " + name,
				RemoveArgs: []string{"pnpm", "remove", "-g", name},
			})
		}
	}
	return items, nil
}

// scanDocker collects images, containers and volumes. A stopped daemon is
// reported as a friendly note rather than an error.
func scanDocker() ([]model.Item, []string) {
	if !commandExists("docker") {
		return nil, []string{"docker not found — skipped"}
	}

	var items []model.Item

	imgOut, imgErr, err := run("docker", "images", "--format", "{{json .}}")
	if err != nil && daemonDown(imgErr) {
		return nil, []string{"Docker daemon not running — skipped"}
	}
	for _, line := range nonEmptyLines(imgOut) {
		var img struct {
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			Size       string `json:"Size"`
			ID         string `json:"ID"`
		}
		if json.Unmarshal([]byte(line), &img) != nil {
			continue
		}
		ref := img.Repository
		if img.Tag != "" && img.Tag != "<none>" {
			ref = img.Repository + ":" + img.Tag
		}
		items = append(items, model.Item{
			Name:       ref,
			Tool:       model.ToolDocker,
			Source:     model.SourcePkg,
			Status:     model.StatusPkg,
			SizeBytes:  parseDockerSize(img.Size),
			Command:    "docker image " + ref,
			RemoveArgs: []string{"docker", "rmi", img.ID},
		})
	}

	ctrOut, _, _ := run("docker", "ps", "-a", "--format", "{{json .}}")
	for _, line := range nonEmptyLines(ctrOut) {
		var c struct {
			Names string `json:"Names"`
			Image string `json:"Image"`
			ID    string `json:"ID"`
			Size  string `json:"Size"`
		}
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		items = append(items, model.Item{
			Name:       "container/" + c.Names,
			Tool:       model.ToolDocker,
			Version:    c.Image,
			Source:     model.SourcePkg,
			Status:     model.StatusPkg,
			SizeBytes:  parseDockerSize(c.Size),
			Command:    "docker container " + c.Names,
			RemoveArgs: []string{"docker", "rm", "-f", c.ID},
		})
	}

	volOut, _, _ := run("docker", "volume", "ls", "--format", "{{json .}}")
	for _, line := range nonEmptyLines(volOut) {
		var v struct {
			Name string `json:"Name"`
		}
		if json.Unmarshal([]byte(line), &v) != nil {
			continue
		}
		items = append(items, model.Item{
			Name:       "volume/" + v.Name,
			Tool:       model.ToolDocker,
			Source:     model.SourcePkg,
			Status:     model.StatusPkg,
			Command:    "docker volume " + v.Name,
			RemoveArgs: []string{"docker", "volume", "rm", v.Name},
		})
	}

	return items, nil
}

func daemonDown(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "cannot connect") ||
		strings.Contains(s, "daemon") ||
		strings.Contains(s, "pipe")
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

var dockerSizeRe = regexp.MustCompile(`(?i)([0-9.]+)\s*([kmgt]?b)`)

// parseDockerSize converts docker's human size strings ("379MB", "1.1GB",
// "0B (virtual 379MB)") into a byte count, using the first match found.
func parseDockerSize(s string) int64 {
	m := dockerSizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(m[2]) {
	case "KB":
		val *= 1024
	case "MB":
		val *= 1024 * 1024
	case "GB":
		val *= 1024 * 1024 * 1024
	case "TB":
		val *= 1024 * 1024 * 1024 * 1024
	}
	return int64(val)
}

// Location helpers used by `trackr where` to locate a package on disk.

// PipLocation returns the site-packages folder reported by `pip show`.
func PipLocation(name string) string {
	if !commandExists("pip") {
		return ""
	}
	out, _, err := run("pip", "show", name)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Location:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Location:"))
		}
	}
	return ""
}

// NpmGlobalRoot returns the global node_modules directory (`npm root -g`).
func NpmGlobalRoot() string {
	if !commandExists("npm") {
		return ""
	}
	out, _, err := run("npm", "root", "-g")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
