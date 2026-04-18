package main

import (
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	in := `abc123  pookiepaws_0.5.2_darwin_arm64.tar.gz
def456  pookiepaws_0.5.2_darwin_amd64.tar.gz
789ghi  pookiepaws_0.5.2_linux_arm64.tar.gz
000jkl  pookiepaws_0.5.2_linux_amd64.tar.gz
xxxxxx  pookiepaws_0.5.2_windows_amd64.zip
`
	got, err := parseChecksums(strings.NewReader(in), "0.5.2")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := assets{
		Version:           "0.5.2",
		DarwinArm64SHA256: "abc123",
		DarwinAmd64SHA256: "def456",
		LinuxArm64SHA256:  "789ghi",
		LinuxAmd64SHA256:  "000jkl",
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := `version "{{ .Version }}" sha "{{ .DarwinArm64SHA256 }}"`
	out, err := renderFormulaText(tmpl, "0.5.2", assets{DarwinArm64SHA256: "abc"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `version "0.5.2" sha "abc"` {
		t.Fatalf("got %q", out)
	}
}

func TestParseChecksumsMissingPlatform(t *testing.T) {
	in := `abc  pookiepaws_0.5.2_darwin_arm64.tar.gz
`
	_, err := parseChecksums(strings.NewReader(in), "0.5.2")
	if err == nil {
		t.Fatal("expected error when platforms missing")
	}
}

func TestRenderUsingRealTemplate(t *testing.T) {
	// End-to-end sanity: render the actual repo template with synthetic SHAs
	// and verify all four placeholders were filled.
	const realTemplate = `class Pookie < Formula
  version "{{ .Version }}"
  on_macos do
    on_arm do
      sha256 "{{ .DarwinArm64SHA256 }}"
    end
    on_intel do
      sha256 "{{ .DarwinAmd64SHA256 }}"
    end
  end
  on_linux do
    on_arm do
      sha256 "{{ .LinuxArm64SHA256 }}"
    end
    on_intel do
      sha256 "{{ .LinuxAmd64SHA256 }}"
    end
  end
end
`
	a := assets{
		DarwinArm64SHA256: "AAAA",
		DarwinAmd64SHA256: "BBBB",
		LinuxArm64SHA256:  "CCCC",
		LinuxAmd64SHA256:  "DDDD",
	}
	out, err := renderFormulaText(realTemplate, "0.5.2", a)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{`version "0.5.2"`, `"AAAA"`, `"BBBB"`, `"CCCC"`, `"DDDD"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}
