package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeGo(t *testing.T) *toolManager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake toolchain uses a POSIX shell")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = env ]; then
  printf '%s' "$PENCODE_TEST_GO_ENV"
  exit 0
fi
printf '%s\n' "$*" >> "$PENCODE_TEST_GO_LOG"
printf '%s' "$GOBIN|${GOOS-unset}|${GOARCH-unset}|$GO111MODULE|$GOPROXY" > "$PENCODE_TEST_GO_INSTALL_ENV"
if [ "$PENCODE_TEST_GO_FAIL" = yes ]; then exit 1; fi
if [ "$PENCODE_TEST_GO_WAIT" = yes ]; then exec /bin/sleep 30; fi
case "$2" in
  golang.org/x/tools/gopls@latest) tool=gopls ;;
  github.com/go-delve/delve/cmd/dlv@latest) tool=dlv ;;
  *) exit 2 ;;
esac
printf '#!/bin/sh\nexit 0\n' > "$GOBIN/$tool"
/bin/chmod +x "$GOBIN/$tool"
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("PENCODE_TEST_GO_ENV", `{"GOBIN":"","GOPATH":""}`)
	t.Setenv("PENCODE_TEST_GO_LOG", filepath.Join(t.TempDir(), "calls"))
	t.Setenv("PENCODE_TEST_GO_INSTALL_ENV", filepath.Join(t.TempDir(), "env"))
	return &toolManager{directory: filepath.Join(t.TempDir(), "tools"), log: io.Discard}
}

func TestMissingToolsInstallAtomicallyAndReuse(t *testing.T) {
	tools := fakeGo(t)
	t.Setenv("GOOS", "unrelated-platform")
	t.Setenv("GOARCH", "unrelated-arch")
	t.Setenv("GOPROXY", "https://proxy.example.test")
	for _, name := range []string{"gopls", "dlv"} {
		command, err := tools.ensure(t.Context(), name)
		if err != nil || command != filepath.Join(tools.directory, name) || !executable(command) {
			t.Fatalf("%s: %q %v", name, command, err)
		}
		if _, err := tools.ensure(t.Context(), name); err != nil {
			t.Fatal(err)
		}
	}
	calls, _ := os.ReadFile(os.Getenv("PENCODE_TEST_GO_LOG"))
	if string(calls) != "install golang.org/x/tools/gopls@latest\ninstall github.com/go-delve/delve/cmd/dlv@latest\n" {
		t.Fatalf("unexpected installation commands: %s", calls)
	}
	env, _ := os.ReadFile(os.Getenv("PENCODE_TEST_GO_INSTALL_ENV"))
	if !strings.HasSuffix(string(env), "|unset|unset|on|https://proxy.example.test") {
		t.Fatalf("cross compilation settings leaked or proxy lost: %s", env)
	}
	entries, _ := os.ReadDir(tools.directory)
	if len(entries) != 2 {
		t.Fatalf("installation left staging files: %v", entries)
	}
}

func TestExistingGOBINToolsNeedNoDownload(t *testing.T) {
	tools := fakeGo(t)
	bin := t.TempDir()
	data, _ := json.Marshal(map[string]string{"GOBIN": bin, "GOPATH": ""})
	t.Setenv("PENCODE_TEST_GO_ENV", string(data))
	command := filepath.Join(bin, "gopls")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := tools.ensure(t.Context(), "gopls")
	if err != nil || got != command {
		t.Fatal(got, err)
	}
	if _, err := os.Stat(os.Getenv("PENCODE_TEST_GO_LOG")); !os.IsNotExist(err) {
		t.Fatal("existing GOBIN tool was reinstalled")
	}
}

func TestFailedInstallationCanRetryAndCancellationCleansStage(t *testing.T) {
	tools := fakeGo(t)
	t.Setenv("PENCODE_TEST_GO_FAIL", "yes")
	if _, err := tools.ensure(t.Context(), "dlv"); err == nil {
		t.Fatal("failed install reported success")
	}
	if executable(filepath.Join(tools.directory, "dlv")) {
		t.Fatal("failed install published a tool")
	}
	t.Setenv("PENCODE_TEST_GO_FAIL", "no")
	t.Setenv("PENCODE_TEST_GO_WAIT", "yes")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := tools.ensure(ctx, "dlv"); err == nil {
		t.Fatal("canceled install reported success")
	}
	entries, _ := os.ReadDir(tools.directory)
	if len(entries) != 0 {
		t.Fatal("failed installation left staging files", entries)
	}
	t.Setenv("PENCODE_TEST_GO_WAIT", "no")
	if _, err := tools.ensure(t.Context(), "dlv"); err != nil {
		t.Fatal("retry failed", err)
	}
	if _, err := tools.ensure(t.Context(), "unknown"); err == nil {
		t.Fatal("accepted an unknown tool")
	}
}
