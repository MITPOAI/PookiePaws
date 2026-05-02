package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunReportsReadyWithFakeTools(t *testing.T) {
	bin := t.TempDir()
	writeFakeCommand(t, bin, "go", "go version go1.22.12 test/amd64")
	writeFakeCommand(t, bin, "python", "Python 3.10.11")
	writeFakeCommand(t, bin, "ffmpeg", "ffmpeg version 7.0")
	t.Setenv("PATH", bin)
	t.Setenv("RUNWARE_API_KEY", "")
	t.Setenv("FAL_KEY", "")

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "media", "render.py"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := `{"format":"9:16","duration":1,"scenes":[{"id":"s1","start":0,"end":1,"text":"Hello"}]}`
	if err := os.WriteFile(filepath.Join(repo, "examples", "edit_plan.json"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}

	report := Run(context.Background(), Options{
		Home:     filepath.Join(t.TempDir(), "home"),
		RepoRoot: repo,
	})
	if !report.OK {
		t.Fatalf("report should be OK: %#v", report.Checks)
	}
	if !report.RendererReady {
		t.Fatalf("renderer should be ready: %#v", report.Checks)
	}
}

func TestRunReportsMissingFFmpeg(t *testing.T) {
	bin := t.TempDir()
	writeFakeCommand(t, bin, "go", "go version go1.22.12 test/amd64")
	writeFakeCommand(t, bin, "python", "Python 3.10.11")
	t.Setenv("PATH", bin)

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "media", "render.py"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := Run(context.Background(), Options{
		Home:     filepath.Join(t.TempDir(), "home"),
		RepoRoot: repo,
	})
	if report.OK {
		t.Fatalf("report should fail when ffmpeg is missing: %#v", report.Checks)
	}
	if report.RendererReady {
		t.Fatalf("renderer should not be ready without ffmpeg: %#v", report.Checks)
	}
}

func writeFakeCommand(t *testing.T, dir, name, output string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\necho '" + output + "'\n"
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		path += ".bat"
		content = "@echo off\r\necho " + output + "\r\n"
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
