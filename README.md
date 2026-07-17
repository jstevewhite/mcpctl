# mcpctl

A cross-platform command-line client for [Model Context Protocol](https://modelcontextprotocol.io) (MCP) servers.

`mcpctl` launches or connects to an MCP server, initializes a session, and lets
you list, inspect, and call its tools from the shell. It speaks both transports
defined by the protocol — local **stdio** subprocesses and remote **Streamable
HTTP** endpoints — and produces either human-readable tables or machine-readable
JSON/JSON&nbsp;Lines/YAML. Server definitions can be saved to a config file or
supplied ad hoc on the command line.

It is a single static binary with no runtime dependencies, built on the official
[Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk).

This application was conceived by me, designed in collaboration with Claude Fable, and implemented primarily by Claude Sonnet and Opus. 

It exists because I got tired of installing MCP servers in a client to test them. Then I realized it would be cool to expose MCP server to scripts. And then I realized it can allow a client that doesn't support MCP to use 'em. Not that there are many of those, just... a thing I noticed.

## Features

- **Two transports** — stdio (child process) and Streamable HTTP.
- **Tool workflow** — `tools list`, `tools describe`, `tools call`, with automatic pagination.
- **Flexible arguments** — pass tool arguments as a JSON string, a JSON file, stdin, or repeated `--arg key=value` pairs.
- **Local schema validation** — arguments are checked against the tool's input schema before the call is sent (opt out with `--no-validate`).
- **Saved or ephemeral servers** — reference a named server from config, or define one inline for a one-shot invocation.
- **Env-based auth** — static headers, headers from environment variables, and bearer tokens, with secrets kept out of config files and logs.
- **Machine-readable output** — `human`, `json`, `jsonl`, and `yaml`, with a strict stdout/stderr split.
- **Correct lifecycle** — context-driven timeouts, Ctrl-C cancellation, and process-group cleanup so stdio children are never orphaned.

## Requirements

- **To install from source:** Go **1.25** or newer. The MCP SDK's module declares `go 1.25`; with the default `GOTOOLCHAIN=auto`, the toolchain is fetched automatically.
- **To run a prebuilt binary:** nothing — it is statically linked.

Running a stdio server may require its own runtime (for example `npx`/Node.js for
`@modelcontextprotocol/server-filesystem`). That is a property of the server you
launch, not of `mcpctl`.

## Installation

### With `go install`

```bash
go install github.com/jstevewhite/mcpctl/cmd/mcpctl@latest
```

This places `mcpctl` in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`); make sure
that directory is on your `PATH`.

### From source

```bash
git clone https://github.com/jstevewhite/mcpctl.git
cd mcpctl
go build -o mcpctl ./cmd/mcpctl
```

### Prebuilt binaries

Tagged releases publish archives for Linux, macOS, and Windows (amd64 and arm64)
on the [Releases page](https://github.com/jstevewhite/mcpctl/releases). Download
the archive for your platform, extract it, and put `mcpctl` on your `PATH`.
(No binary release has been cut yet — build from source in the meantime.)

## Quick start

Every `tools` command connects to exactly one server, chosen by **one** of
`--server`, `--stdio`, or `--url`. The fastest way to try it is an ephemeral
stdio server — no config needed:

```bash
# List the tools a filesystem server exposes
mcpctl tools list --stdio -- npx -y @modelcontextprotocol/server-filesystem /tmp

# Inspect one tool's schema
mcpctl tools describe read_file --stdio -- npx -y @modelcontextprotocol/server-filesystem /tmp

# Call it
mcpctl tools call read_file --arg path=/tmp/example.txt \
  --stdio -- npx -y @modelcontextprotocol/server-filesystem /tmp
```

Everything after the first `--` is the server command and its arguments,
preserved verbatim and executed directly (never through a shell). Tool name and
tool-argument flags go **before** the `--`.

## Connecting to a server

| Mode      | Flag                     | Use it for                                        |
|-----------|--------------------------|---------------------------------------------------|
| Named     | `--server NAME`          | A server saved in your config file.               |
| Ephemeral | `--stdio -- CMD [ARGS…]` | A local subprocess, defined inline.               |
| Ephemeral | `--url URL`              | A remote Streamable HTTP endpoint, defined inline.|

These are mutually exclusive — provide exactly one.

### Named servers (configuration)

Save a definition once, then reference it by name. `server add` only writes to
the config file; it does **not** connect to or execute the server.

```bash
# A local stdio server
mcpctl server add --name local-fs \
  --stdio -- npx -y @modelcontextprotocol/server-filesystem /tmp

# A remote HTTP server with a bearer token from the environment
mcpctl server add --name remote \
  --url https://example.com/mcp \
  --bearer-env MCP_BEARER_TOKEN

mcpctl server list
mcpctl server show --name remote
mcpctl tools list --server local-fs
mcpctl server remove --name local-fs
```

`server list` and `server show` redact secrets: environment-variable *names* and
header *names* are shown, but values are never printed.

### Ephemeral stdio

```bash
mcpctl tools list     --stdio -- ./my-server --flag
mcpctl tools describe read_file --stdio -- ./my-server
mcpctl tools call     echo --json '{"message":"hello"}' -- ./my-server
```

Grammar: `mcpctl tools <sub> --stdio [TOOL] [arg flags] -- <server-command> [args…]`.
If `--stdio` is given with no command after `--`, it is a usage error.

### Ephemeral HTTP

```bash
mcpctl tools list --url https://example.com/mcp \
  --header-env Authorization=MCP_AUTHORIZATION

mcpctl tools call search --json '{"query":"MCP"}' \
  --url https://example.com/mcp \
  --bearer-env MCP_BEARER_TOKEN
```

The HTTP form has no `--`, since there is no child process.

## Calling tools and passing arguments

Provide arguments in exactly **one** of these ways (they are mutually exclusive):

```bash
--json '{"query":"MCP","limit":10}'   # a JSON object literal
--json-file arguments.json            # a JSON object from a file
--json-file -                         # a JSON object from stdin
--arg query=MCP --arg limit=10        # repeated key=value pairs
```

`--json` / `--json-file` must decode to a JSON **object** — arrays, strings,
numbers, booleans, and `null` at the top level are rejected.

### `--arg` value rules

Each `--arg` is split on the first `=`. The value is parsed as JSON if it is
valid JSON, and otherwise treated as a plain string:

```bash
--arg query=MCP                  # string  "MCP"
--arg limit=10                   # number  10
--arg enabled=true               # boolean true
--arg tags='["go","mcp"]'        # array   ["go","mcp"]
--arg options='{"a":false}'      # object  {"a":false}
--arg value='"true"'             # string  "true"  (quoted -> stays a string)
```

Because values that look like JSON are decoded as JSON, numeric-looking values
become numbers — watch these footguns:

- `--arg version=1.10` → the number `1.1` (trailing zero lost).
- `--arg id=1e3` → the number `1000`.
- `--arg zip=01234` → the string `"01234"` (a leading zero is not valid JSON).

To keep a value as text, quote it as a JSON string (`--arg version='"1.10"'`) or
use `--json` / `--json-file`. Version 1 supports **top-level properties only**;
for nested arguments use `--json` or `--json-file`. Duplicate and empty keys are
rejected.

### Validation

Before sending `tools/call`, arguments are validated against the tool's input
schema. On failure the call is not sent and the process exits `8`. Pass
`--no-validate` to skip the local check and let the server validate instead. If a
server's schema cannot be compiled, `mcpctl` warns (to stderr, human mode only)
and proceeds rather than making the tool permanently unusable.

## Authentication

Secrets are referenced by environment-variable name, never stored as literals by
default. All auth flags apply to HTTP transports only.

| Flag                       | Effect                                                          |
|----------------------------|----------------------------------------------------------------|
| `--bearer-env VAR`         | Sends `Authorization: Bearer $VAR`.                            |
| `--header-env NAME=VAR`    | Sends header `NAME` with the value of `$VAR`.                  |
| `--header-literal NAME=V`  | Sends header `NAME: V` (literal; **writes a secret to disk / shell history** — avoid). |

```bash
export MCP_BEARER_TOKEN=sk-...
mcpctl tools list --url https://example.com/mcp --bearer-env MCP_BEARER_TOKEN
```

Rules and safeguards:

- A referenced environment variable that is unset is an error, reported before connecting.
- `--bearer-env` conflicts with defining `Authorization` via `--header-env`/`--header-literal`; supplying both is a configuration error.
- On a cross-origin HTTP redirect, every client-added header (bearer and custom) is stripped, so credentials never follow a redirect to another host.
- Tokens are never written to logs, errors, or machine output.

## Output formats

Select with the global `--output` flag: `human` (default), `json`, `jsonl`, or
`yaml`. Redirecting stdout does **not** change the format — only `--output` does;
redirection just disables color.

```bash
mcpctl tools list --server local-fs                  # aligned table (default)
mcpctl tools list --server local-fs --output json    # one JSON document
mcpctl tools list --server local-fs --output jsonl    # one tool object per line
mcpctl tools list --server local-fs --output yaml     # YAML, structurally equal to JSON
```

Command results go to **stdout**; logs, warnings, progress, and the server's own
stderr go to **stderr**. Machine-readable stdout is never contaminated with logs,
color codes, or warnings, so it is safe to pipe into `jq`, `yq`, etc. Tool
results are preserved faithfully — text, structured content, `isError`, and
metadata for binary/embedded content are all kept rather than flattened.

## Configuration file

### Location

The path is resolved at runtime, in order:

1. `--config PATH`, if given.
2. `$XDG_CONFIG_HOME/mcpctl/config.toml`, if `XDG_CONFIG_HOME` is set.
3. The OS user config dir from Go's `os.UserConfigDir()` — e.g. `~/.config/mcpctl/config.toml` on Linux, `~/Library/Application Support/mcpctl/config.toml` on macOS, `%AppData%\mcpctl\config.toml` on Windows.

Config files are written atomically with `0600` permissions. Read-only commands
never create a file implicitly, and ephemeral (`--stdio` / `--url`) commands work
with no config file at all.

### Example

```toml
version = 1

[defaults]
timeout = "30s"
connect_timeout = "15s"

[servers.local-fs]
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
cwd = "/tmp"

[servers.local-fs.env]
LOG_LEVEL = "warn"

[servers.remote]
transport = "streamable-http"
url = "https://example.com/mcp"

[servers.remote.headers]
Accept-Language = "en-US"

[servers.remote.header_env]
Authorization = "MCP_AUTHORIZATION"

[servers.remote.bearer_token]
env = "MCP_BEARER_TOKEN"
```

`header_env` maps an HTTP header name (key) to the environment variable (value)
whose contents become the header at runtime. `version` must be `1`; unknown
fields are rejected to catch typos.

## Global flags

| Flag                    | Default  | Purpose                                             |
|-------------------------|----------|-----------------------------------------------------|
| `--config PATH`         | —        | Config file path.                                   |
| `--output FORMAT`       | `human`  | `human`, `json`, `jsonl`, or `yaml`.                |
| `--timeout DURATION`    | `30s`    | Overall command timeout (`1s`–`24h`; zero/negative invalid). |
| `--connect-timeout DURATION` | `15s` | Connection/initialization timeout (currently applied as part of `--timeout`). |
| `--log-level LEVEL`     | `warn`   | `debug`, `info`, `warn`, or `error` (to stderr).    |
| `--no-color`            | off      | Disable colored output.                             |
| `--no-validate`         | off      | Skip local argument validation.                     |

`tools list` additionally accepts **`--max-pages N`** (default `1000`, minimum
`1`): the maximum number of `tools/list` pages to fetch before stopping. It
backstops a server that paginates without end; independently, a repeated cursor
always aborts the loop.

## Exit codes

| Code | Meaning                                    |
|------|--------------------------------------------|
| `0`  | Success                                    |
| `1`  | Unspecified / internal failure             |
| `2`  | Command-line usage error                   |
| `3`  | Configuration error                        |
| `4`  | Authentication error                       |
| `5`  | Connection or transport error              |
| `6`  | MCP protocol or initialization error       |
| `7`  | Tool not found                             |
| `8`  | Invalid tool arguments                     |
| `9`  | Tool returned `isError = true`             |
| `10` | Timeout                                    |
| `130`| Interrupted (Ctrl-C)                       |

A tool that returns `isError = true` is a successful MCP exchange but a failed
tool execution: the result is rendered and the process exits `9`.

## Security

- **stdio commands are executed directly — never through a shell.** Arguments are
  not word-split, globbed, or otherwise interpreted. You are responsible for the
  trustworthiness of any command you launch; `mcpctl` runs it with your
  environment and privileges.
- Secrets are referenced by environment-variable name; `--header-literal` is the
  only way to write a literal secret and it warns that it does so.
- Config files are written `0600` (owner read/write only).
- MCP server output — tool descriptions, schemas, and results — is treated as
  untrusted. Control characters are sanitized in human-readable output to prevent
  terminal escape injection.
- TLS uses normal certificate verification. There is no `--insecure` option.

## Troubleshooting

- **`exactly one of --server, --stdio, or --url is required`** — every `tools`
  command needs a target. Pick one.
- **stdio server exits immediately / `EOF` during `initialize` (exit `5`)** — the
  command after `--` is not a working MCP server, or it failed to start. Its
  stderr is forwarded to your terminal; add `--log-level debug` for lifecycle
  detail.
- **Missing environment variable (exit `4`)** — a `--bearer-env`/`--header-env`
  variable is unset. `export` it first.
- **`exit 8` on a call** — local validation rejected the arguments; the message
  names the offending JSON path. Fix the arguments, or use `--no-validate` to let
  the server decide.
- **Garbled or empty machine output** — make sure you passed `--output json`
  (or `jsonl`/`yaml`); redirecting stdout alone keeps the human format. Logs are
  always on stderr, so `2>/dev/null` cleans up noise if needed.
- **`exceeded the page cap` / repeated-cursor protocol error** — `tools list`
  stops rather than looping forever when a server pages past the `--max-pages`
  cap (default 1000) or repeats a cursor it has already returned. Raise
  `--max-pages` only for a server you trust to terminate.

## Supported MCP transports

- **stdio** — launches a child process and speaks MCP over its stdin/stdout.
- **Streamable HTTP** — connects to a remote `http`/`https` endpoint, including
  SSE/streaming responses and MCP session headers.

The protocol version is negotiated by the SDK (currently `2025-11-25`, via Go MCP
SDK v1.5.0).

## Limitations

Version 1 intentionally omits:

- `resources` and `prompts` commands, sampling, and elicitation.
- OAuth flows and OS keychain integration (auth is env-var based).
- Persistent sessions — each command opens and closes its own session, so a
  `tools call` re-lists tools first.
- Nested `--arg` paths — use `--json`/`--json-file` for nested arguments.
- An interactive TUI, a background daemon, and proxy/gateway modes.
- Overriding the negotiated protocol version — the SDK pins it and exposes no
  override, so `--protocol-version` is **rejected with a usage error** (exit `2`)
  rather than silently ignored. The flag remains so it can be wired through if a
  future SDK adds support.

## Development

```bash
make build         # go build ./...
make test          # go test ./...
make race          # go test -race ./...
make vet           # go vet ./...
make staticcheck   # staticcheck ./...
make check         # all of the above
```

CI runs the same gates plus a Linux/macOS/Windows cross-build on every push and
pull request. SDK-specific code is confined to `internal/client`; command
handlers work against a small internal `Client` interface and stable output
types.

```text
cmd/mcpctl/        # main entry point
internal/
  cli/             # Cobra commands (root, server, tools)
  client/          # MCP SDK adapter — stdio + Streamable HTTP transports
  config/          # TOML load/save, paths, validation
  auth/            # header/bearer resolution, secret redaction
  arguments/       # --arg/--json parsing, JSON-Schema validation
  output/          # human / json / jsonl / yaml renderers
  testserver/      # deterministic MCP test servers
```

## License

MIT — see [LICENSE](LICENSE).
