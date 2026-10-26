package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

func TestActivationInstallsAfterRegistrationAndPublishesReadiness(t *testing.T) {
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
	if err != nil || !contributions.ActionService || len(contributions.Actions) != 0 || len(contributions.DebugAdapters) != 1 || len(contributions.LanguageServers) != 1 {
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
		if err != nil || frame.Kind != peb.NotificationUpdate {
			t.Fatal("status update", frame, err)
		}
		notification, err := peb.DecodeNotification(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if notification.Error {
			t.Fatal(notification.Message)
		}
		if notification.Message == "gopls 和 Delve 已就绪" {
			break
		}
	}
	for _, tool := range []string{"gopls", "dlv"} {
		if !executable(filepath.Join(tools.directory, toolFilename(tool))) {
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

func TestFailedActivationNotifiesAndRetriesWithoutStatusBarAction(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "tools")
	t.Setenv("PENCODE_GO_TOOLS_DIR", directory)
	t.Setenv("PENCODE_GO_EXTENSION_HELPER", "1")
	t.Setenv("PATH", t.TempDir())
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
		t.Fatal(frame, err)
	}
	contributions, err := peb.DecodeContributions(frame.Payload)
	if err != nil || !contributions.ActionService || len(contributions.Actions) != 0 {
		t.Fatal(contributions, err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("registration performed tool installation")
	}
	if err := w.Write(peb.ActionActivate, 0, nil); err != nil {
		t.Fatal(err)
	}
	readNotification := func() peb.Notification {
		t.Helper()
		frame, err := r.Read()
		if err != nil || frame.Kind != peb.NotificationUpdate {
			t.Fatal("expected notification", frame, err)
		}
		n, err := peb.DecodeNotification(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	readNotification() // Checking tools.
	failure := readNotification()
	if !failure.Error || failure.Action != "tools" || failure.ActionText == "" {
		t.Fatal("failure has no retry", failure)
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"gopls", "dlv"} {
		if err := os.WriteFile(filepath.Join(directory, toolFilename(tool)), nil, 0755); err != nil {
			t.Fatal(err)
		}
	}
	request, _ := peb.EncodeActionContext(nil, peb.ActionContext{ID: failure.Action})
	if err := w.Write(peb.ActionInvoke, 2, request); err != nil {
		t.Fatal(err)
	}
	ready, response := false, false
	for !ready || !response {
		frame, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		switch frame.Kind {
		case peb.NotificationUpdate:
			n, err := peb.DecodeNotification(frame.Payload)
			if err != nil || n.Error {
				t.Fatal(n, err)
			}
			ready = n.Message == "gopls 和 Delve 已就绪"
		case peb.ActionResponse:
			response = frame.ID == 2
		default:
			t.Fatal("unexpected frame", frame)
		}
	}
	if err := w.Write(peb.Shutdown, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
