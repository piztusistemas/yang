package main

import "testing"

// TestLinuxInstallCommand only checks the *selection* logic (which package
// manager wins, what argv gets built) - it never runs pkexec/apt, so it's
// safe to run anywhere without touching the real system.
func TestLinuxInstallCommand(t *testing.T) {
	argv, mgr, ok := linuxInstallCommand()
	if !ok {
		t.Skip("no known package manager on this machine")
	}
	if len(argv) == 0 {
		t.Fatalf("%s: empty argv", mgr)
	}
	t.Logf("detected %s -> %v", mgr, argv)
}

// TestLinuxLatexInstallCommand mirrors TestLinuxInstallCommand for the
// LaTeX toolchain's package list.
func TestLinuxLatexInstallCommand(t *testing.T) {
	argv, mgr, ok := linuxLatexInstallCommand()
	if !ok {
		t.Skip("no known package manager on this machine")
	}
	if len(argv) == 0 {
		t.Fatalf("%s: empty argv", mgr)
	}
	t.Logf("detected %s -> %v", mgr, argv)
}

// TestLinuxPandocInstallCommand mirrors TestLinuxInstallCommand for
// Pandoc's package list.
func TestLinuxPandocInstallCommand(t *testing.T) {
	argv, mgr, ok := linuxPandocInstallCommand()
	if !ok {
		t.Skip("no known package manager on this machine")
	}
	if len(argv) == 0 {
		t.Fatalf("%s: empty argv", mgr)
	}
	t.Logf("detected %s -> %v", mgr, argv)
}

// TestCheckDocDeps mirrors TestCheckLatexDeps for Pandoc: this dev/CI
// machine has it installed (docdoc_test.go's TestExportViaPandoc already
// depends on that), so PandocFound should come back true here.
func TestCheckDocDeps(t *testing.T) {
	a := &App{}
	if !commandExists("pandoc") {
		t.Skip("pandoc not found on this machine, skipping")
	}
	if !a.CheckDocDeps().PandocFound {
		t.Error("pandoc is on PATH but PandocFound is false")
	}
}

// TestCheckLatexDeps runs against whatever is actually installed on this
// machine (no mocking - commandExists shells out to exec.LookPath for
// real) - CI/dev machines running the rest of this test suite already need
// a working pdflatex + pdftoppm (see latexdoc_test.go's TestGeneratePDF),
// so this should report OK here; it's really locking in the "engine
// re-derived from settings" behaviour, not the detection itself.
func TestCheckLatexDeps(t *testing.T) {
	a := &App{}

	a.settings.LatexEngine = ""
	status := a.CheckLatexDeps()
	if status.Engine != "pdflatex" {
		t.Errorf("empty LatexEngine setting should default to pdflatex, got %q", status.Engine)
	}
	if !commandExists("pdflatex") {
		t.Skip("pdflatex not found on this machine, skipping the rest")
	}
	if !status.EngineFound {
		t.Error("pdflatex is on PATH but EngineFound is false")
	}
	if !status.PdftoppmFound && commandExists("pdftoppm") {
		t.Error("pdftoppm is on PATH but PdftoppmFound is false")
	}

	a.settings.LatexEngine = "xelatex"
	status = a.CheckLatexDeps()
	if status.Engine != "xelatex" {
		t.Errorf("LatexEngine=xelatex should be reflected as-is, got %q", status.Engine)
	}

	a.settings.LatexEngine = "pdflatex"
	if !a.CheckLatexDeps().OK() {
		t.Error("OK() should be true when both engine and pdftoppm are found")
	}
}
