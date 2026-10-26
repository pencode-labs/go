# Go

Go language support for Pencode: completion, diagnostics, navigation, references,
rename, formatting and code actions through gopls; launch, attach, breakpoints,
stepping, stacks, variables and evaluation through Delve and Pencode’s debugger.
Go sources, `go.mod` and `go.work` share one language server per workspace.

The extension automatically checks **gopls** and **dlv** when activated. Existing
executables in PATH, the extension cache, GOBIN or GOPATH/bin are reused. Missing
tools are downloaded and installed with the official [gopls](https://go.dev/gopls/) and
[Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) commands:

```sh
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
```

Install Go on the workspace machine first. The extension preserves GOPROXY,
GOPRIVATE and GOTOOLCHAIN settings, and installs host executables independently of
project GOOS/GOARCH. Downloads run in the background after registration; language
and debug launchers also ensure their tool is ready on first use (up to 10 minutes).
The Go status button shows readiness or an error; click it to check again or retry.
Details appear in **Output → Extension: go** (or the language/debug output for a
first-use launch).

Managed tools live in `<UserCacheDir>/pencode/go/bin`, outside the extension
package. Override this with an absolute `PENCODE_GO_TOOLS_DIR` on the workspace
server. Completed installs are published atomically. Existing tools are not
upgraded automatically. Remote workspaces install and run everything remotely.
Delve is registered with the server by the extension, so its cache directory does
not need to be added to PATH. Explicit debugger `command` settings take priority.

## Build and install

Uses the Pencode peb v8 SDK. Requires Go 1.27.0 or newer.

```sh
go build -o bin/go .
go test -race ./...
```

In Pencode’s extension manager choose **Install from local directory** and select
this checkout. Build and install on the workspace host for remote workspaces.
On Windows build `bin/go.exe` and set `entry` in `resources/extension.json` to
`bin/go.exe` before installing.

Save files, select **Go: current package** or its tests in Debug, press F9 to set a
breakpoint and F5 to start. Project launch/attach configuration is documented in
[Pencode’s debugger guide](https://github.com/pencode-labs/pencode/blob/main/debugger/README.md).

## License

MIT. See [LICENSE](LICENSE).
