package cmd

import (
	"testing"

	"trackr/internal/model"
)

func TestFilterFolderGhosts(t *testing.T) {
	ghosts := []model.Item{
		{Name: "npm-cache"},          // excluded by name
		{Name: "npm"},                // excluded by name + detected tool
		{Name: "Programs"},           // excluded container
		{Name: "NVIDIA Corporation"}, // excluded infra
		{Name: "my-app-updater"},     // excluded by suffix
		{Name: "GenuineApp"},         // kept
		{Name: "AnotherTool"},        // kept
	}
	pkg := []model.Item{
		{Name: "express", Tool: "npm"},
		{Name: "requests", Tool: "pip"},
	}

	out := filterFolderGhosts(ghosts, pkg)

	kept := map[string]bool{}
	for _, g := range out {
		kept[g.Name] = true
	}
	if len(out) != 2 || !kept["GenuineApp"] || !kept["AnotherTool"] {
		t.Fatalf("expected only GenuineApp and AnotherTool kept, got %v", out)
	}
	for _, bad := range []string{"npm-cache", "npm", "Programs", "NVIDIA Corporation", "my-app-updater"} {
		if kept[bad] {
			t.Errorf("%q should have been filtered out", bad)
		}
	}
}

func TestShouldExcludeFromOrphanScan(t *testing.T) {
	for _, name := range []string{"Temp", "npm-cache", "go-build", "Foo-Builder", "x-cache"} {
		if !shouldExcludeFromOrphanScan(name) {
			t.Errorf("expected %q to be excluded", name)
		}
	}
	for _, name := range []string{"VisualStudioCode", "Git", "Docker"} {
		if shouldExcludeFromOrphanScan(name) {
			t.Errorf("expected %q to be kept", name)
		}
	}
}
