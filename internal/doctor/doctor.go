package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/renderer"
)

type Options struct {
	Home     string
	RepoRoot string
	Python   string
}

type Report struct {
	OK            bool    `json:"ok"`
	RendererReady bool    `json:"renderer_ready"`
	Home          string  `json:"home"`
	MemoryPath    string  `json:"memory_path"`
	Checks        []Check `json:"checks"`
}

type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Detail   string `json:"detail,omitempty"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
}

func Run(ctx context.Context, opts Options) Report {
	repo := opts.RepoRoot
	if repo == "" {
		repo = "."
	}
	home := opts.Home
	if home == "" {
		home = defaultHome()
	}
	home, _ = filepath.Abs(home)
	memoryPath := filepath.Join(home, "memory", "pookiepaws.db")

	report := Report{
		OK:         true,
		Home:       home,
		MemoryPath: memoryPath,
	}

	report.add(Check{
		Name:     "go_runtime",
		OK:       strings.HasPrefix(runtime.Version(), "go1.22") || versionAtLeast(runtime.Version(), "go1.22"),
		Required: true,
		Version:  runtime.Version(),
		Detail:   "Go runtime used to build/run this command",
	})
	report.add(commandCheck(ctx, "go_toolchain", true, []string{"go"}, "version"))
	report.add(pythonCheck(ctx, opts.Python))
	report.add(ffmpegCheck(ctx))
	report.add(envCheck("RUNWARE_API_KEY", false))
	report.add(envCheck("FAL_KEY", false))
	report.add(envCheck("POOKIEPAWS_HOME", false))
	report.add(memoryWritableCheck(home))
	report.add(fileCheck("renderer_script", true, filepath.Join(repo, "scripts", "media", "render.py")))
	report.add(editPlanCheck(filepath.Join(repo, "examples", "edit_plan.json")))

	report.RendererReady = report.checkOK("python") && report.checkOK("ffmpeg") && report.checkOK("renderer_script")
	for _, check := range report.Checks {
		if check.Required && !check.OK {
			report.OK = false
			break
		}
	}
	return report
}

func (r *Report) add(check Check) {
	r.Checks = append(r.Checks, check)
}

func (r Report) checkOK(name string) bool {
	for _, check := range r.Checks {
		if check.Name == name {
			return check.OK
		}
	}
	return false
}

func commandCheck(ctx context.Context, name string, required bool, names []string, versionArg string) Check {
	var missing []string
	for _, candidate := range names {
		path, err := exec.LookPath(candidate)
		if err != nil {
			missing = append(missing, candidate)
			continue
		}
		check := Check{Name: name, OK: true, Required: required, Path: path}
		cmd := exec.CommandContext(ctx, path, versionArg)
		out, err := cmd.CombinedOutput()
		if err != nil {
			check.OK = false
			check.Detail = strings.TrimSpace(err.Error() + ": " + string(out))
			return check
		}
		check.Version = firstLine(string(out))
		return check
	}
	return Check{
		Name:     name,
		OK:       false,
		Required: required,
		Detail:   "not found on PATH: " + strings.Join(missing, ", "),
	}
}

func pythonCheck(ctx context.Context, configured string) Check {
	var candidates []string
	if configured != "" {
		candidates = append(candidates, configured)
	}
	if env := strings.TrimSpace(os.Getenv("PYTHON")); env != "" && env != configured {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, "python", "python3")
	return commandCheck(ctx, "python", true, candidates, "--version")
}

func ffmpegCheck(ctx context.Context) Check {
	var candidates []string
	if env := strings.TrimSpace(os.Getenv("FFMPEG")); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, "ffmpeg")
	return commandCheck(ctx, "ffmpeg", true, candidates, "-version")
}

func envCheck(name string, required bool) Check {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		detail := "not set"
		if !required {
			detail = "not set; optional"
		}
		return Check{Name: name, OK: !required, Required: required, Detail: detail}
	}
	return Check{Name: name, OK: true, Required: required, Detail: "set", Version: redact(value)}
}

func memoryWritableCheck(home string) Check {
	dir := filepath.Join(home, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Check{Name: "memory_writable", OK: false, Required: true, Path: dir, Detail: err.Error()}
	}
	f, err := os.CreateTemp(dir, ".doctor-*.tmp")
	if err != nil {
		return Check{Name: "memory_writable", OK: false, Required: true, Path: dir, Detail: err.Error()}
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if err := errors.Join(closeErr, removeErr); err != nil {
		return Check{Name: "memory_writable", OK: false, Required: true, Path: dir, Detail: err.Error()}
	}
	return Check{Name: "memory_writable", OK: true, Required: true, Path: dir, Detail: "can create and remove a temp file"}
}

func fileCheck(name string, required bool, path string) Check {
	if _, err := os.Stat(path); err != nil {
		return Check{Name: name, OK: false, Required: required, Path: path, Detail: err.Error()}
	}
	return Check{Name: name, OK: true, Required: required, Path: path}
}

func editPlanCheck(path string) Check {
	if _, err := renderer.LoadPlan(path); err != nil {
		return Check{Name: "example_edit_plan", OK: false, Required: false, Path: path, Detail: err.Error()}
	}
	return Check{Name: "example_edit_plan", OK: true, Required: false, Path: path, Detail: "valid JSON edit plan"}
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexAny(value, "\r\n"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func defaultHome() string {
	if custom := strings.TrimSpace(os.Getenv("POOKIEPAWS_HOME")); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".pookiepaws"
	}
	return filepath.Join(home, ".pookiepaws")
}

func versionAtLeast(actual, min string) bool {
	actual = strings.TrimPrefix(actual, "go")
	min = strings.TrimPrefix(min, "go")
	var aMaj, aMin, mMaj, mMin int
	_, _ = fmt.Sscanf(actual, "%d.%d", &aMaj, &aMin)
	_, _ = fmt.Sscanf(min, "%d.%d", &mMaj, &mMin)
	return aMaj > mMaj || (aMaj == mMaj && aMin >= mMin)
}

func redact(value string) string {
	if len(value) <= 6 {
		return "set"
	}
	return value[:3] + "..." + value[len(value)-3:]
}
