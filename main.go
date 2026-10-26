// The Go extension supplies gopls, Delve and their workspace-side installers.
// Registration is immediate; tool downloads never block the PEB handshake.
package main

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pencode-labs/pencode/peb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := ownProcesses(); err != nil {
		return err
	}
	tools, err := newTools()
	if err != nil {
		return err
	}
	if len(os.Args) >= 3 && os.Args[1] == "--tool" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		command, err := tools.ensure(ctx, os.Args[2])
		if err != nil {
			return err
		}
		cmd := exec.Command(command, os.Args[3:]...)
		hideWindow(cmd)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	var active atomic.Bool
	var activate context.Context
	var publisher *peb.ActionPublisher
	install := func() {
		if !active.CompareAndSwap(false, true) {
			return
		}
		go func() {
			_ = publisher.Show(toolNotification("Go", "正在检查并安装 gopls 和 Delve…", false))
			ctx, cancel := context.WithTimeout(activate, 10*time.Minute)
			defer cancel()
			var failures string
			for _, tool := range []string{"gopls", "dlv"} {
				if _, err := tools.ensure(ctx, tool); err != nil {
					fmt.Fprintln(os.Stderr, err)
					failures += err.Error() + "\n"
				}
			}
			active.Store(false)
			if failures != "" {
				_ = publisher.Show(toolNotification("Go：工具安装失败", failures+"详细日志见 Output → Extension: go。", true))
			} else {
				ui := toolNotification("Go", "gopls 和 Delve 已就绪", false)
				ui.AutoCloseMS = 5000
				_ = publisher.Show(ui)
			}
		}()
	}
	return peb.ServeExtension(os.Stdin, os.Stdout, goContributions(filepath.Base(self)), peb.Handlers{
		ActivateActions: func(ctx context.Context, p *peb.ActionPublisher) {
			activate, publisher = ctx, p
			install()
		},
		UIEvent: func(e peb.UIEvent) error {
			if e.ID == "tools" && e.Event.Kind == "click" && e.Event.Ref == "retry" {
				install()
			}
			return nil
		},
	})
}

func toolNotification(title, message string, retry bool) peb.UI {
	view := &peb.EditorView{HTML: "<p>" + html.EscapeString(message) + "</p>", CSS: "p { white-space: pre-wrap; }"}
	if retry {
		view.HTML += `<button data-ref="retry" type="button">重试</button>`
		view.Events = []peb.EditorBinding{{Ref: "retry", Kind: "click"}}
	}
	return peb.UI{ID: "tools", Title: title, Kind: peb.Notification, View: view}
}

func goContributions(executable string) peb.Contributions {
	command := "bin/" + executable
	return peb.Contributions{
		LanguageServers: []peb.LanguageServer{{
			ID: "gopls", Label: "Go (gopls)", LanguageID: "go", Command: command, Args: []string{"--tool", "gopls"},
			Selectors:     []peb.DocumentSelector{{Match: peb.FileSuffix, Pattern: ".go"}, {Match: peb.FileName, Pattern: "go.mod", LanguageID: "go.mod"}, {Match: peb.FileName, Pattern: "go.work", LanguageID: "go.work"}},
			Configuration: `{"gopls":{"analyses":{"unusedparams":true},"staticcheck":true,"gofumpt":true}}`,
		}},
		DebugAdapters: []peb.DebugAdapter{{ID: "Delve", Label: "Go (Delve)", Command: command, Args: []string{"--tool", "dlv"}}},
	}
}
