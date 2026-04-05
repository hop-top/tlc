# hdl

Cross-platform URL scheme registration for Go. Register a custom protocol (e.g. `ctxt://`)
with the OS so clicking that link opens your app.

## Install

    go get hop.top/hdl

## Usage

```go
import "hop.top/hdl"

// Runtime registration (macOS CLI, Linux, Windows)
err := hdl.Register("ctxt", "/Applications/ctxt.app")

// Generate static snippets (iOS, app bundles)
import "hop.top/hdl/generate"
snippet, _ := generate.Snippet("ios", "ctxt", "")
```

## Platform support

| Platform | Runtime | Static snippet |
|----------|---------|---------------|
| macOS (CLI) | ✅ LSSetDefaultHandlerForURLScheme | ✅ PlistSnippet |
| iOS | ❌ must be in Info.plist | ✅ PlistSnippet |
| Linux | ✅ xdg-mime + .desktop | ✅ DesktopFile |
| Windows | ✅ HKCU registry | ✅ WindowsRegSnippet |
| Other | ❌ ErrUnsupported | ✅ Snippet("platform", ...) |

## Related

- [hop.top/uri](https://github.com/hop-top/uri) — structured identifier parsing
