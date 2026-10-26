package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pencode-labs/pencode/peb"
)

func TestGoExtensionProcess(t *testing.T) {
	if os.Getenv("PENCODE_GO_EXTENSION_HELPER") != "1" {
		return
	}
	os.Args = os.Args[:1]
	if err := run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestActivationInstallsAfterRegistrationAndRemovesStatus(t *testing.T) {
	tools := fakeGo(t)
	t.Setenv("PENCODE_GO_TOOLS_DIR", tools.directory)
	t.Setenv("PENCODE_GO_EXTENSION_HELPER", "1")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestGoExtensionProcess$")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { in.Close(); cmd.Process.Kill(); cmd.Wait() })
	w, r := peb.Writer{Target: in}, peb.Reader{Source: out}
	hello, _ := peb.EncodeHello(nil, peb.Hello{Host: "test", Capabilities: peb.Capabilities})
	if err := w.Write(peb.Initialize, 1, hello); err != nil {
		t.Fatal(err)
	}
	frame, err := r.Read()
	if err != nil || frame.Kind != peb.Register {
		t.Fatal("registration", frame, err)
	}
	contributions, err := peb.DecodeContributions(frame.Payload)
	if err != nil || !contributions.ActionService || len(contributions.DebugAdapters) != 1 || len(contributions.LanguageServers) != 1 {
		t.Fatal("contributions", contributions, err)
	}
	if _, err := os.Stat(tools.directory); !os.IsNotExist(err) {
		t.Fatal("registration performed tool installation")
	}
	if err := w.Write(peb.ActionActivate, 0, nil); err != nil {
		t.Fatal(err)
	}
	for {
		frame, err := r.Read()
		if err != nil || frame.Kind != peb.ActionUpdate {
			t.Fatal("status update", frame, err)
		}
		change, err := peb.DecodeActionChange(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(change.Action.Text, "失败") {
			t.Fatal(change.Action.Tooltip)
		}
		if change.Remove && change.Action.ID == "tools" {
			break
		}
	}
	for _, tool := range []string{"gopls", "dlv"} {
		if !executable(filepath.Join(tools.directory, tool)) {
			t.Fatal("activation did not install", tool)
		}
	}
	if err := w.Write(peb.Shutdown, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
