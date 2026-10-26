package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type toolManager struct {
	mu        sync.Mutex
	directory string
	paths     []string
	probed    bool
	log       io.Writer
}

func newTools() (*toolManager, error) {
	directory := os.Getenv("PENCODE_GO_TOOLS_DIR")
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		directory = filepath.Join(cache, "pencode", "go", "bin")
	}
	directory, err := filepath.Abs(directory)
	return &toolManager{directory: directory, log: os.Stderr}, err
}

func toolModule(name string) string {
	switch name {
	case "gopls":
		return "golang.org/x/tools/gopls@latest"
	case "dlv":
		return "github.com/go-delve/delve/cmd/dlv@latest"
	}
	return ""
}

func toolFilename(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func executable(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0)
}

func (t *toolManager) find(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, directory := range append([]string{t.directory}, t.paths...) {
		path := filepath.Join(directory, toolFilename(name))
		if executable(path) {
			return path
		}
	}
	return ""
}

func (t *toolManager) ensure(ctx context.Context, name string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	module := toolModule(name)
	if module == "" {
		return "", fmt.Errorf("unknown Go tool %q", name)
	}
	if path := t.find(name); path != "" {
		return path, nil
	}
	if !t.probed {
		probe, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(probe, "go", "env", "-json", "GOBIN", "GOPATH")
		hideWindow(cmd)
		data, err := cmd.Output()
		cancel()
		if err != nil {
			return "", fmt.Errorf("Go toolchain unavailable; install Go on the workspace machine: %w", err)
		}
		var env struct{ GOBIN, GOPATH string }
		if err := json.Unmarshal(data, &env); err != nil {
			return "", fmt.Errorf("go env: %w", err)
		}
		t.probed = true
		if env.GOBIN != "" {
			t.paths = append(t.paths, env.GOBIN)
		}
		for _, directory := range filepath.SplitList(env.GOPATH) {
			if directory != "" {
				t.paths = append(t.paths, filepath.Join(directory, "bin"))
			}
		}
		if path := t.find(name); path != "" {
			return path, nil
		}
	}
	if err := os.MkdirAll(t.directory, 0o755); err != nil {
		return "", err
	}
	// Build in a private directory and publish only the completed executable.
	// Activation and simultaneous LSP/DAP launches cannot see a partial install.
	stage, err := os.MkdirTemp(t.directory, ".install-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	fmt.Fprintf(t.log, "Go: installing %s…\n", name)
	cmd := exec.CommandContext(ctx, "go", "install", module)
	hideWindow(cmd)
	cmd.Dir, cmd.WaitDelay = stage, time.Second
	cmd.Env = installEnvironment(stage)
	cmd.Stdout, cmd.Stderr = t.log, t.log
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install %s: %w (see Extension: go output)", name, err)
	}
	from, destination := filepath.Join(stage, toolFilename(name)), filepath.Join(t.directory, toolFilename(name))
	if !executable(from) {
		return "", fmt.Errorf("go install did not produce %s", from)
	}
	if err := os.Rename(from, destination); err != nil && !executable(destination) {
		return "", err
	}
	fmt.Fprintf(t.log, "Go: %s ready at %s\n", name, destination)
	return destination, nil
}

func installEnvironment(bin string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GOBIN", "GOOS", "GOARCH", "GO111MODULE":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "GOBIN="+bin, "GO111MODULE=on")
}
