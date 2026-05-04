package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trydydd/detour/internal/config"
)

// helperBinary builds a small helper binary that prints its env and args,
// then returns its path. The binary is cached in t.TempDir().
func helperBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "helper.go")
	bin := filepath.Join(dir, "helper")
	code := `package main
import (
	"fmt"
	"os"
	"strings"
)
func main() {
	for _, e := range os.Environ() {
		fmt.Println("ENV:" + e)
	}
	fmt.Println("ARGS:" + strings.Join(os.Args[1:], ","))
	os.Exit(0)
}
`
	if err := os.WriteFile(src, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}

func makeCfg(modelName string, port int) *config.Config {
	return &config.Config{
		Port:      port,
		ModelName: modelName,
		ModelAPI:  "http://localhost:8001",
	}
}

func TestEnvInjected(t *testing.T) {
	bin := helperBinary(t)
	out, err := launchCapture(bin, makeCfg("red", 8888), nil)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	assertEnv(t, out, "ANTHROPIC_BASE_URL", "http://127.0.0.1:8888")
}

func TestCustomModelOptionSet(t *testing.T) {
	bin := helperBinary(t)
	out, err := launchCapture(bin, makeCfg("red", 8888), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertEnv(t, out, "ANTHROPIC_CUSTOM_MODEL_OPTION", "red")
}

func TestAPIKeyUnchanged(t *testing.T) {
	const testKey = "sk-ant-test-12345"
	t.Setenv("ANTHROPIC_API_KEY", testKey)
	bin := helperBinary(t)
	out, err := launchCapture(bin, makeCfg("red", 8888), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertEnv(t, out, "ANTHROPIC_API_KEY", testKey)
}

func TestArgsForwarded(t *testing.T) {
	bin := helperBinary(t)
	out, err := launchCapture(bin, makeCfg("red", 8888), []string{"--foo", "bar"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ARGS:--foo,bar") {
		t.Errorf("args not forwarded; output:\n%s", out)
	}
}

func TestExitCodePropagated(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "exit2.go")
	bin := filepath.Join(dir, "exit2")
	code := "package main\nimport \"os\"\nfunc main() { os.Exit(2) }\n"
	os.WriteFile(src, []byte(code), 0o600)
	exec.Command("go", "build", "-o", bin, src).Run()

	err := Launch(makeCfg("red", 8888), nil, bin)
	if err == nil {
		t.Fatal("expected non-nil error for exit code 2")
	}
}

// assertEnv checks that "KEY=value" appears in the output.
func assertEnv(t *testing.T, out, key, want string) {
	t.Helper()
	prefix := fmt.Sprintf("ENV:%s=", key)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			got := strings.TrimPrefix(line, prefix)
			if got != want {
				t.Errorf("env %s: want %q, got %q", key, want, got)
			}
			return
		}
	}
	t.Errorf("env %s not found in output", key)
}
