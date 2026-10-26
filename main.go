// The Go extension supplies gopls, Delve and their workspace-side installers.
// Registration is immediate; tool downloads never block the PEB handshake.
package main

import (
	"context"
	"fmt"
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
			_ = publisher.Notify(peb.Notification{ID: "tools", Title: "Go", Message: "正在检查并安装 gopls 和 Delve…"})
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
				_ = publisher.Notify(peb.Notification{ID: "tools", Title: "Go：工具安装失败", Message: failures + "详细日志见 Output → Extension: go。", Error: true, Action: "tools", ActionText: "重试"})
			} else {
				_ = publisher.Notify(peb.Notification{ID: "tools", Title: "Go", Message: "gopls 和 Delve 已就绪"})
			}
		}()
	}
	return peb.ServeExtension(os.Stdin, os.Stdout, goContributions(filepath.Base(self)), peb.Handlers{
		ActivateActions: func(ctx context.Context, p *peb.ActionPublisher) {
			activate, publisher = ctx, p
			install()
		},
		Action: func(c peb.ActionContext) error {
			if c.ID == "tools" {
				install()
			}
			return nil
		},
	})
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
