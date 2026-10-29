package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pencode-labs/pencode/debugger"
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
	templates, err := debugger.ParseConfigs([]byte(contributions.DebugAdapters[0].Templates), "${workspaceFolder}")
	if err != nil || len(templates) != 2 || templates[0].Adapter != "Delve" {
		t.Fatal("Go templates", templates, err)
	}
	if _, err := os.Stat(tools.directory); !os.IsNotExist(err) {
		t.Fatal("registration performed tool installation")
	}
	if err := w.Write(peb.ActionActivate, 0, nil); err != nil {
		t.Fatal(err)
	}
	for {
		frame, err := r.Read()
		if err != nil || frame.Kind != peb.UIUpdate {
			t.Fatal("status update", frame, err)
		}
		notification, err := peb.DecodeUI(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if notification.Kind != peb.Notification || strings.Contains(notification.Title, "失败") {
			t.Fatal(notification)
		}
		if notification.AutoCloseMS > 0 && strings.Contains(notification.View.HTML, "gopls 和 Delve 已就绪") {
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
	readNotification := func() peb.UI {
		t.Helper()
		frame, err := r.Read()
		if err != nil || frame.Kind != peb.UIUpdate {
			t.Fatal("expected notification", frame, err)
		}
		n, err := peb.DecodeUI(frame.Payload)
		if err != nil || n.Kind != peb.Notification || n.View == nil {
			t.Fatal(err)
		}
		return n
	}
	readNotification() // Checking tools.
	failure := readNotification()
	if !strings.Contains(failure.Title, "失败") || failure.AutoCloseMS != 0 || len(failure.View.Events) != 1 || failure.View.Events[0].Ref != "retry" {
		t.Fatal("failure has no retry", failure)
	}
	if !strings.Contains(failure.View.HTML, `data-pencode-action="show-output"`) || !strings.Contains(failure.View.HTML, "查看日志") {
		t.Fatal("failure has no output action", failure)
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"gopls", "dlv"} {
		if err := os.WriteFile(filepath.Join(directory, toolFilename(tool)), nil, 0755); err != nil {
			t.Fatal(err)
		}
	}
	request, _ := peb.EncodeUIEvent(nil, peb.UIEvent{ID: failure.ID, Event: peb.EditorEvent{Kind: "click", Ref: "retry"}})
	if err := w.Write(peb.UIRequest, 2, request); err != nil {
		t.Fatal(err)
	}
	ready, response := false, false
	for !ready || !response {
		frame, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		switch frame.Kind {
		case peb.UIUpdate:
			n, err := peb.DecodeUI(frame.Payload)
			if err != nil || n.Kind != peb.Notification || strings.Contains(n.Title, "失败") || n.View == nil {
				t.Fatal(n, err)
			}
			ready = n.AutoCloseMS > 0 && strings.Contains(n.View.HTML, "gopls 和 Delve 已就绪")
		case peb.UIResponse:
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
