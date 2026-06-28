package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestIsProtectedPath(t *testing.T) {
	protected := []string{
		`C:\`,
		`C:`,
		`D:\`,
		`C:\Windows`,
		`C:\Windows\System32`,
		`C:\Windows\SysWOW64`,
		`C:\Program Files\Common Files\Microsoft Shared`,
		`C:\Program Files`,
		`C:\Program Files (x86)`,
		``,
	}
	for _, p := range protected {
		if ok, _ := isProtectedPath(p); !ok {
			t.Errorf("expected %q to be protected, but it was allowed", p)
		}
	}

	// Add the live ProgramFiles env value explicitly.
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		if ok, _ := isProtectedPath(pf); !ok {
			t.Errorf("expected ProgramFiles root %q to be protected", pf)
		}
	}

	allowed := []string{
		`C:\Program Files\SomeApp`,
		`C:\Program Files (x86)\iVMS-4200 Site`,
		`D:\Tools\Thing`,
		`C:\Users\me\AppData\Local\Programs\VSCode`,
	}
	for _, p := range allowed {
		if ok, reason := isProtectedPath(p); ok {
			t.Errorf("expected %q to be deletable, but blocked: %s", p, reason)
		}
	}
}

func TestParseUninstallMSI(t *testing.T) {
	argv, isMsi := parseUninstall(`MsiExec.exe /I{CE2F96D0-63D2-4B9C-A8D6-0D1A60840BD8}`)
	if !isMsi {
		t.Fatalf("expected MSI detection")
	}
	got := strings.Join(argv, " ")
	want := `msiexec /x {CE2F96D0-63D2-4B9C-A8D6-0D1A60840BD8} /quiet /norestart`
	if got != want {
		t.Errorf("msi argv = %q, want %q", got, want)
	}
}

func TestParseUninstallQuotedExe(t *testing.T) {
	argv, isMsi := parseUninstall(`"C:\Program Files (x86)\App\uninstall.exe" /foo`)
	if isMsi {
		t.Fatalf("did not expect MSI")
	}
	if argv[0] != `C:\Program Files (x86)\App\uninstall.exe` {
		t.Errorf("first token not parsed past quotes/space: %q", argv[0])
	}
	// Inno Setup uninstallers ("unins...") should get a /VERYSILENT flag.
	if !contains(argv, "/VERYSILENT") {
		t.Errorf("expected silent flag appended for uninstaller exe, got %v", argv)
	}
}

func TestTokenizeRespectsQuotes(t *testing.T) {
	tokens := tokenize(`"C:\Program Files\a b\x.exe" arg1 "arg two"`)
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[0] != `C:\Program Files\a b\x.exe` || tokens[2] != "arg two" {
		t.Errorf("quoted tokenization wrong: %v", tokens)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
