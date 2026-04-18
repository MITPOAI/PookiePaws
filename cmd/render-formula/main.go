// Command render-formula fills the Homebrew formula template using SHA256s
// from a goreleaser checksums.txt file. Output is written to stdout.
//
// Usage:
//
//	render-formula --dist dist --version 0.5.2 --template packaging/homebrew/pookie.rb.tmpl
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type assets struct {
	Version           string
	DarwinArm64SHA256 string
	DarwinAmd64SHA256 string
	LinuxArm64SHA256  string
	LinuxAmd64SHA256  string
}

func main() {
	dist := flag.String("dist", "dist", "Path to goreleaser dist directory")
	version := flag.String("version", "", "Release version (e.g. 0.5.2; without v prefix)")
	tmplPath := flag.String("template", "packaging/homebrew/pookie.rb.tmpl", "Template file")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "--version required")
		os.Exit(2)
	}
	v := strings.TrimPrefix(*version, "v")

	checksumsPath := filepath.Join(*dist, "checksums.txt")
	f, err := os.Open(checksumsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", checksumsPath, err)
		os.Exit(1)
	}
	defer f.Close()

	a, err := parseChecksums(f, v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	tmplBytes, err := os.ReadFile(*tmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read template: %v\n", err)
		os.Exit(1)
	}
	out, err := renderFormulaText(string(tmplBytes), v, a)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func parseChecksums(r io.Reader, version string) (assets, error) {
	wantSuffixes := map[string]*string{
		"darwin_arm64.tar.gz": nil,
		"darwin_amd64.tar.gz": nil,
		"linux_arm64.tar.gz":  nil,
		"linux_amd64.tar.gz":  nil,
	}
	got := assets{Version: version}
	wantSuffixes["darwin_arm64.tar.gz"] = &got.DarwinArm64SHA256
	wantSuffixes["darwin_amd64.tar.gz"] = &got.DarwinAmd64SHA256
	wantSuffixes["linux_arm64.tar.gz"] = &got.LinuxArm64SHA256
	wantSuffixes["linux_amd64.tar.gz"] = &got.LinuxAmd64SHA256

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sum, file := fields[0], fields[1]
		for suffix, target := range wantSuffixes {
			expected := fmt.Sprintf("pookiepaws_%s_%s", version, suffix)
			if file == expected {
				*target = sum
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return assets{}, err
	}
	for suffix, target := range wantSuffixes {
		if *target == "" {
			return assets{}, fmt.Errorf("checksums.txt missing entry for %s", suffix)
		}
	}
	return got, nil
}

func renderFormulaText(tmpl, version string, a assets) (string, error) {
	t, err := template.New("formula").Parse(tmpl)
	if err != nil {
		return "", err
	}
	a.Version = version
	var sb strings.Builder
	if err := t.Execute(&sb, a); err != nil {
		return "", err
	}
	return sb.String(), nil
}
