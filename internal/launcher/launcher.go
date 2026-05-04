package launcher

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/trydydd/detour/internal/config"
)

// Launch starts the claude binary (or claudeBin override) as a subprocess
// with detour's env vars injected. It blocks until the subprocess exits.
func Launch(cfg *config.Config, claudeArgs []string, claudeBin string) error {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	env := buildEnv(os.Environ(), envOverrides(cfg))
	cmd := exec.Command(claudeBin, claudeArgs...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// launchCapture is a test helper that captures subprocess output.
func launchCapture(claudeBin string, cfg *config.Config, claudeArgs []string) (string, error) {
	env := buildEnv(os.Environ(), envOverrides(cfg))
	cmd := exec.Command(claudeBin, claudeArgs...)
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func envOverrides(cfg *config.Config) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":            proxyURL(cfg),
		"ANTHROPIC_CUSTOM_MODEL_OPTION": cfg.ModelName,
	}
}

func buildEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	// Copy base, replacing keys that appear in overrides.
	applied := make(map[string]bool)
	for _, e := range base {
		key := e[:strings.IndexByte(e, '=')]
		if val, ok := overrides[key]; ok {
			if val != "" {
				out = append(out, key+"="+val)
			}
			applied[key] = true
		} else {
			out = append(out, e)
		}
	}
	// Add override keys that weren't already in base.
	for k, v := range overrides {
		if !applied[k] && v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func proxyURL(cfg *config.Config) string {
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
}
