package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// printJSON writes payload as indented JSON to w. Returns whatever the
// encoder returns — caller decides how to handle write errors.
func printJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// emitJSONOrExit writes JSON to stdout and exits non-zero on encode failure
// (rare — usually only on pipe-closed writes). Used by diagnostic commands
// when --json is set.
func emitJSONOrExit(payload any) {
	if err := printJSON(os.Stdout, payload); err != nil {
		fmt.Fprintf(os.Stderr, "json: %v\n", err)
		os.Exit(1)
	}
}
