# Go

Registers an installed `gopls` language server with Pencode. Install it separately with `go install golang.org/x/tools/gopls@latest`.

Uses the [Pencode](https://github.com/pencode-labs/pencode) peb v6 SDK. Requires Go 1.27.0 or newer.

## Build and install

```sh
git clone https://github.com/pencode-labs/go.git
cd go
go build -o bin/go .
```

In Pencode's extension manager, choose **Install from local directory** and select this checkout. Extensions run on the workspace server, so build and install on that host for remote workspaces.

On Windows, build `bin/go.exe` and set `entry` in `resources/extension.json` to `bin/go.exe` before installing.

## Development

```sh
go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
