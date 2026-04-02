package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/cli"
)

// cmdInit runs the interactive first-run setup wizard. It collects API keys
// and provider URLs, explains how they are stored, and writes
// ~/.pookiepaws/.security.json with mode 0600.
func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()
	p.Plain("Welcome to PookiePaws — your local marketing operations runtime.")
	p.Blank()
	p.Dim("Your API keys stay on this machine only.")
	p.Dim("They are written to ~/.pookiepaws/.security.json with permissions 0600.")
	p.Dim("PookiePaws never transmits credentials to any remote service.")
	p.Dim("You can rerun this wizard later and leave prompts blank to keep current values.")
	p.Blank()

	runtimeRoot, _, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	secretsPath := filepath.Join(runtimeRoot, ".security.json")

	// Load existing values so the wizard can show "currently set".
	existing := map[string]string{}
	if data, readErr := os.ReadFile(secretsPath); readErr == nil {
		// Strip UTF-8 BOM if present.
		data = trimBOM(data)
		_ = json.Unmarshal(data, &existing)
		p.Warning("Existing configuration found at %s", secretsPath)
		p.Dim("Leave any prompt blank to keep the current value.")
		p.Blank()
	}

	next := make(map[string]string, len(existing))
	for k, v := range existing {
		next[k] = v
	}

	reader := bufio.NewReader(os.Stdin)

	// ask prompts the user for a single value, optionally hiding input.
	ask := func(label, key, hint string, secret bool) {
		cur := next[key]
		if cur != "" {
			if secret {
				p.Plain("%s  (currently set — blank to keep):", label)
			} else {
				p.Plain("%s  (current: %s — blank to keep):", label, cur)
			}
		} else {
			p.Plain("%s%s:", label, hint)
		}
		fmt.Fprint(os.Stdout, "  > ")

		var val string
		var inputErr error
		if secret {
			val, inputErr = cli.ReadSecret()
			fmt.Fprintln(os.Stdout) // restore newline after hidden input
		} else {
			val, inputErr = reader.ReadString('\n')
		}
		if inputErr != nil {
			return
		}
		val = strings.TrimSpace(val)
		if val == "" {
			return // keep existing
		}
		next[key] = val
	}

	// ── LLM Provider ─────────────────────────────────────────────────────────
	p.Rule("LLM Provider")
	p.Blank()
	p.Dim("PookiePaws speaks to any OpenAI-compatible endpoint, including local models")
	p.Dim("via Ollama, LM Studio, or similar.")
	p.Blank()

	ask("LLM base URL", "llm_base_url",
		"  (e.g. http://localhost:11434/v1/chat/completions)", false)
	ask("LLM model name", "llm_model",
		"  (e.g. gpt-4o, claude-3-5-sonnet, llama3.2:latest)", false)
	ask("LLM API key", "llm_api_key",
		"  (leave blank for local/unauthenticated models)", true)

	// ── CRM Integration ───────────────────────────────────────────────────────
	p.Blank()
	p.Rule("CRM Integration  (optional — press Enter to skip)")
	p.Blank()
	p.Dim("SALESmanago credentials power the lead-routing skill.")
	p.Blank()

	ask("SALESmanago base URL", "salesmanago_base_url",
		"  (e.g. https://api.salesmanago.com/v3/keyInformation/upsert)", false)
	ask("SALESmanago API key", "salesmanago_api_key", "", true)
	ask("SALESmanago owner email", "salesmanago_owner", "", false)

	// ── SMS Integration ───────────────────────────────────────────────────────
	p.Blank()
	p.Rule("SMS Integration  (optional — press Enter to skip)")
	p.Blank()
	p.Dim("Mitto credentials power the SMS-drafter skill.")
	p.Blank()

	ask("Mitto API key", "mitto_api_key", "", true)
	ask("Mitto base URL", "mitto_base_url", "  (default: https://rest.mittoapi.net)", false)
	ask("Mitto sender ID", "mitto_from", "  (the From name shown on messages)", false)

	// —— WhatsApp Integration —————————————————————————————————————————————
	p.Blank()
	p.Rule("WhatsApp Integration  (optional — press Enter to skip)")
	p.Blank()
	p.Dim("WhatsApp runs as an approval-gated outbound channel.")
	p.Dim("The default provider mode expects Meta Cloud API-compatible endpoints.")
	p.Blank()

	ask("WhatsApp provider", "whatsapp_provider", "  (default: meta_cloud)", false)
	ask("WhatsApp access token", "whatsapp_access_token", "", true)
	ask("WhatsApp phone number ID", "whatsapp_phone_number_id", "", false)
	ask("WhatsApp business account ID", "whatsapp_business_account_id", "", false)
	ask("WhatsApp webhook verify token", "whatsapp_webhook_verify_token", "", true)
	ask("WhatsApp base URL", "whatsapp_base_url", "  (default: https://graph.facebook.com/v23.0)", false)

	// ── Write ─────────────────────────────────────────────────────────────────
	p.Blank()

	// Remove keys whose values are empty so the file stays clean.
	for k, v := range next {
		if strings.TrimSpace(v) == "" {
			delete(next, k)
		}
	}

	spin := p.NewSpinner("Saving configuration…")
	spin.Start()

	if mkErr := os.MkdirAll(runtimeRoot, 0o755); mkErr != nil {
		spin.Stop(false, "Failed to create runtime directory")
		p.Error("%v", mkErr)
		os.Exit(1)
	}

	data, marshalErr := json.MarshalIndent(next, "", "  ")
	if marshalErr != nil {
		spin.Stop(false, "Failed to serialise configuration")
		os.Exit(1)
	}

	// Write atomically via a temp file + rename.
	tmp := secretsPath + ".tmp"
	if writeErr := os.WriteFile(tmp, data, 0o600); writeErr != nil {
		spin.Stop(false, "Failed to write configuration")
		p.Error("%v", writeErr)
		os.Exit(1)
	}
	if renameErr := os.Rename(tmp, secretsPath); renameErr != nil {
		spin.Stop(false, "Failed to save configuration")
		p.Error("%v", renameErr)
		os.Exit(1)
	}

	spin.Stop(true, fmt.Sprintf("Configuration saved to %s", secretsPath))

	p.Blank()
	p.Plain("Start your workspace with:")
	p.Blank()
	p.Accent("  pookie start")
	p.Blank()
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
