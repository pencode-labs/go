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
Tool checks, readiness and errors use Pencode’s bottom-right notifications. Failed
checks include **Retry** and **View Logs** buttons. **View Logs** opens Output with
**Extension: go** selected and clears the output filter.
Readiness notifications close automatically after five seconds.
Details appear in **Output → Extension: go** (or the language/debug output for a
first-use launch).

Managed tools live in `<UserCacheDir>/pencode/go/bin`, outside the extension
package. Override this with an absolute `PENCODE_GO_TOOLS_DIR` on the workspace
server. Completed installs are published atomically. Existing tools are not
upgraded automatically. Remote workspaces install and run everything remotely.
Delve is registered with the server by the extension, so its cache directory does
not need to be added to PATH. Explicit debugger `command` settings take priority.

## Build and install

Uses the Pencode peb v14 SDK. Requires Go 1.27.0 or newer.

```sh
go build -o bin/go .
go test -race ./...
```

In Pencode’s extension manager choose **Install from local directory** and select
this checkout. Build and install on the workspace host for remote workspaces.
On Windows build `bin/go.exe` and set `entry` in `resources/extension.json` to
`bin/go.exe` before installing.

In Debug, choose **添加启动器** and select **Go: 当前项目** or **Go: 包测试**.
This appends a template to `.pencode/debug.json` and opens the JSON for editing.
Save changes, press F9 to set a breakpoint and F5 to start. Project launch/attach configuration is documented in
[Pencode’s debugger guide](https://github.com/pencode-labs/pencode/blob/main/debugger/README.md).

## License

MIT. See [LICENSE](LICENSE).

## Releases

Push a `v*` tag (for example, `v1.1.0`) to this repository, or publish a GitHub Release,
to test and build Linux, macOS and Windows packages for amd64 and arm64. The workflow
uploads ZIP archives and `SHA256SUMS` to the matching release, creating it if needed.
Prerelease tags such as `v1.1.0-rc.1` create prereleases.

Each ZIP contains `bin/`, `resources/`, the README and license. Its manifest uses the
tag version and the correct executable suffix for Windows. Download the package
for the workspace server's OS and architecture, extract it, then install the
extracted directory through Pencode's extension manager.
