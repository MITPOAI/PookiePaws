package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mitpoai/pookiepaws/internal/doctor"
)

func cmdSetup(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return errors.New("setup requires: check")
	}
	return cmdDoctor(ctx, args[1:])
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	strict := fs.Bool("strict", false, "return a non-zero exit code when required checks fail")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedHome, err := resolveAdHome(*home)
	if err != nil {
		return err
	}
	report := doctor.Run(ctx, doctor.Options{
		Home:     resolvedHome,
		RepoRoot: repoRoot(),
	})
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printDoctorReport(os.Stdout, report)
	}
	if *strict && !report.OK {
		return errors.New("doctor found missing required dependencies")
	}
	return nil
}

func printDoctorReport(w io.Writer, report doctor.Report) {
	status := "ready"
	if !report.OK {
		status = "needs setup"
	}
	fmt.Fprintf(w, "PookiePaws doctor: %s\n", status)
	fmt.Fprintf(w, "home: %s\n", report.Home)
	fmt.Fprintf(w, "memory: %s\n", report.MemoryPath)
	fmt.Fprintf(w, "renderer_ready: %t\n\n", report.RendererReady)
	for _, check := range report.Checks {
		marker := "OK"
		if !check.OK {
			marker = "MISSING"
		}
		required := "optional"
		if check.Required {
			required = "required"
		}
		fmt.Fprintf(w, "[%s] %s (%s)", marker, check.Name, required)
		if check.Version != "" {
			fmt.Fprintf(w, " - %s", check.Version)
		}
		if check.Path != "" {
			fmt.Fprintf(w, " - %s", check.Path)
		}
		if check.Detail != "" {
			fmt.Fprintf(w, " - %s", check.Detail)
		}
		fmt.Fprintln(w)
	}
	if !report.RendererReady {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Install FFmpeg and make sure it is on PATH before rendering MP4 files.")
	}
}
