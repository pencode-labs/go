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
			defer active.Store(false)
			_ = publisher.Update(toolStatus("Go：正在检查工具…", true, ""))
			ctx, cancel := context.WithTimeout(activate, 10*time.Minute)
			defer cancel()
			var failures string
			for _, tool := range []string{"gopls", "dlv"} {
				if _, err := tools.ensure(ctx, tool); err != nil {
					fmt.Fprintln(os.Stderr, err)
					failures += err.Error() + "\n"
				}
			}
			if failures != "" {
				_ = publisher.Update(toolStatus("Go：工具安装失败，点击重试", false, failures))
			} else {
				_ = publisher.Update(toolStatus("Go：工具已就绪", false, "gopls 和 Delve 已就绪；点击重新检查"))
			}
		}()
	}
	return peb.ServeExtension(os.Stdin, os.Stdout, goContributions(filepath.Base(self)), peb.Handlers{
		ActivateActions: func(ctx context.Context, p *peb.ActionPublisher) {
			activate, publisher = ctx, p
			install()
		},
		Action: func(peb.ActionContext) error { install(); return nil },
	})
}

func toolStatus(text string, disabled bool, tooltip string) peb.Action {
	return peb.Action{ID: "tools", Text: text, Tooltip: tooltip, Location: peb.StatusBar, Disabled: disabled}
}

func goContributions(executable string) peb.Contributions {
	command := "bin/" + executable
	return peb.Contributions{
		Actions: []peb.Action{toolStatus("Go：检查工具", true, "自动检测并安装 gopls 和 Delve")},
		LanguageServers: []peb.LanguageServer{{
			ID: "gopls", Label: "Go (gopls)", LanguageID: "go", Command: command, Args: []string{"--tool", "gopls"},
			Selectors:     []peb.DocumentSelector{{Match: peb.FileSuffix, Pattern: ".go"}, {Match: peb.FileName, Pattern: "go.mod", LanguageID: "go.mod"}, {Match: peb.FileName, Pattern: "go.work", LanguageID: "go.work"}},
			Configuration: `{"gopls":{"analyses":{"unusedparams":true},"staticcheck":true,"gofumpt":true}}`,
		}},
		DebugAdapters: []peb.DebugAdapter{{ID: "Delve", Label: "Go (Delve)", Command: command, Args: []string{"--tool", "dlv"}}},
	}
}
