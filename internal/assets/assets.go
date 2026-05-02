package assets

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type ProjectDirs struct {
	Root    string `json:"root"`
	Assets  string `json:"assets"`
	Outputs string `json:"outputs"`
	Reports string `json:"reports"`
}

func NewProjectID(product string, now time.Time) string {
	slug := Slugify(product)
	if slug == "" {
		slug = "ad"
	}
	return slug + "-" + now.Format("20060102-150405") + "-" + randomSuffix()
}

func EnsureProjectDirs(baseDir, projectID string) (ProjectDirs, error) {
	root := filepath.Join(baseDir, projectID)
	dirs := ProjectDirs{
		Root:    root,
		Assets:  filepath.Join(root, "assets"),
		Outputs: filepath.Join(root, "outputs"),
		Reports: filepath.Join(root, "reports"),
	}
	for _, dir := range []string{dirs.Root, dirs.Assets, dirs.Outputs, dirs.Reports} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ProjectDirs{}, err
		}
	}
	return dirs, nil
}

func Slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func randomSuffix() string {
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "local"
	}
	return hex.EncodeToString(buf[:])
}
