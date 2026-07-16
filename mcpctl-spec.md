````markdown
# Specification: `mcpctl` — Go-Based Command-Line MCP Client

## 1. Objective

Build a cross-platform command-line client named `mcpctl` for interacting with Model Context Protocol (MCP) servers.

The client must support:

1. Launching and communicating with MCP servers over stdio.
2. Connecting to remote MCP servers over Streamable HTTP.
3. Initializing an MCP session and negotiating protocol capabilities.
4. Listing available tools.
5. Inspecting a tool and its input schema.
6. Calling tools with JSON arguments.
7. Producing human-readable and machine-readable output.
8. Loading reusable server definitions from configuration.
9. Handling cancellation, timeouts, server errors, and process cleanup correctly.
10. Building as a single executable for Linux, macOS, and Windows.

The implementation should use the official Go MCP SDK rather than implementing MCP or JSON-RPC directly.

---

## 2. Implementation Principles

### 2.1 Language and runtime

Use:

- Go 1.24 or newer
- Go modules
- `context.Context` for all network, process, and MCP operations
- `log/slog` for diagnostic logging
- GoReleaser for release builds

The result should be a native executable with no runtime dependency.

### 2.2 MCP implementation

Use the current stable official Go MCP SDK:

- Repository/package family: `github.com/modelcontextprotocol/go-sdk`
- Pin the dependency to a specific released version in `go.mod`
- Do not depend directly on internal SDK packages
- Do not manually implement MCP framing, initialization, capability negotiation, or Streamable HTTP session management unless an SDK defect makes it unavoidable

Before implementation, inspect the current SDK API and adapt package names and constructor calls accordingly. The SDK API is authoritative where this specification uses conceptual names.

If the SDK does not implement a required behavior, isolate the workaround behind an internal interface and document it. Do not spread SDK-specific types throughout the CLI.

### 2.3 CLI framework

Use Cobra:

- `github.com/spf13/cobra`

Avoid Viper unless configuration requirements become substantially more complicated. Prefer explicit configuration parsing and environment handling.

### 2.4 Configuration format

Use TOML for the initial implementation.

Suggested TOML library:

- `github.com/pelletier/go-toml/v2`

### 2.5 JSON Schema validation

Use a maintained JSON Schema library compatible with schemas returned by MCP servers.

Suggested library:

- `github.com/santhosh-tekuri/jsonschema/v6`

The validator must tolerate the schema dialects commonly emitted by MCP servers. If a schema cannot be compiled locally, report a warning in human-readable mode and allow the server to perform validation. Invalid or unsupported server schemas must not make a tool permanently unusable.

---

## 3. Scope

### 3.1 Version 1 requirements

Version 1 must implement:

- stdio transport
- Streamable HTTP transport
- MCP initialization
- `tools/list`, including pagination
- tool lookup and description
- `tools/call`
- configuration file support
- ephemeral server definitions from CLI flags
- static HTTP headers
- bearer tokens sourced from environment variables
- configurable timeouts
- graceful cancellation
- JSON, JSON Lines, YAML, and human-readable output where applicable
- JSON Schema argument validation
- cross-platform builds
- unit and integration tests

### 3.2 Version 1 non-goals

The following are not required for version 1:

- an interactive terminal UI
- LLM integration
- MCP server implementation
- prompts or resources commands
- sampling requests initiated by servers
- elicitation support
- a persistent background daemon
- acting as an MCP proxy or gateway
- automatic installation of stdio server packages
- shell evaluation of configured commands
- complete OAuth dynamic client registration

The architecture must not prevent later support for resources, prompts, OAuth, or server-initiated requests.

---

## 4. Command-Line Interface

## 4.1 Root command

Command name:

    mcpctl

Root help should explain that the tool connects to MCP servers and invokes their tools.

Global flags:

    --config PATH
    --output FORMAT
    --timeout DURATION
    --connect-timeout DURATION
    --log-level LEVEL
    --no-color
    --no-validate
    --protocol-version VERSION
    --help
    --version

Defaults:

- `--output human` when stdout is a terminal
- `--output json` is not automatically selected merely because stdout is redirected
- `--timeout 30s`
- `--connect-timeout 15s`
- `--log-level warn`
- color enabled only when stderr/stdout is an appropriate terminal
- argument validation enabled

Supported output formats:

- `human`
- `json`
- `jsonl`
- `yaml`

Not every command must meaningfully support JSON Lines. Commands returning collections should emit one object per line in JSON Lines mode.

All diagnostics and logs must go to stderr. Command results must go to stdout.

## 4.2 Server management commands

Implement:

    mcpctl server list
    mcpctl server show --name NAME
    mcpctl server add --name NAME [transport options]
    mcpctl server remove --name NAME

Examples:

    mcpctl server add --name local-fs \
      --stdio \
      -- npx -y @modelcontextprotocol/server-filesystem /tmp

    mcpctl server add --name remote \
      --url https://example.com/mcp

    mcpctl server remove --name local-fs

`server add` modifies the configuration file. It must not execute or connect to the server. It should validate that the server name is not empty and does not conflict with an existing name, but need not verify that the command exists or the URL is reachable.

For HTTP servers:

    mcpctl server add --name remote \
      --url https://example.com/mcp \
      --header-env Authorization=MCP_AUTHORIZATION

Secrets must not be accepted as literal configuration values by default. Environment-variable references are preferred.

A separate explicit flag may permit literal headers, but help text must warn that this writes secrets to disk:

    --header-literal 'X-API-Key=secret'

### 4.2.1 Server list output

`server list` human output must show at least:

    NAME          TRANSPORT          URL / COMMAND
    local-fs      stdio              npx -y @modelcontextprotocol/server-filesystem /tmp
    remote        streamable-http    https://example.com/mcp

For stdio servers, display the command and arguments as a single joined string. For HTTP servers, display the URL.

The command, args, URL, env variable names, and header names must be shown. Environment variable values and literal header values must be redacted.

JSON and YAML output should preserve the complete server definition from configuration, with values sourced from environment variables replaced by a placeholder such as `"<env:MCP_AUTHORIZATION>"` to avoid leaking secrets.

JSON Lines output should emit one server object per line.

### 4.2.2 Server show output

`server show` human output must display all server details:

    Name:        local-fs
    Transport:   stdio
    Command:     npx -y @modelcontextprotocol/server-filesystem /tmp
    CWD:         /tmp
    Environment:
      LOG_LEVEL=warn
    Headers:     (none)
    Bearer:      (none)

For HTTP servers with bearer tokens, show the environment variable name, never the token value. Literal headers must be flagged with a visibility warning.

JSON and YAML output should preserve the complete server definition with the same secret-placeholder behavior as `server list`.

## 4.3 Tool commands

Named servers are referenced with `--server`. Ephemeral servers are specified with `--stdio` or `--url`. These are mutually exclusive: exactly one of `--server`, `--stdio`, or `--url` must be provided for every tools command.

Implement:

    mcpctl tools list --server SERVER
    mcpctl tools describe --server SERVER TOOL
    mcpctl tools call --server SERVER TOOL [argument options]

Examples:

    mcpctl tools list --server local-fs
    mcpctl tools describe --server local-fs read_file

    mcpctl tools call --server local-fs read_file \
      --arg path=/tmp/example.txt

    mcpctl tools call --server remote search \
      --json '{"query":"MCP","limit":10}'

    mcpctl tools call --server remote search \
      --json-file arguments.json

    cat arguments.json | \
      mcpctl tools call --server remote search --json-file -

### 4.3.1 Ephemeral server definitions

A user must be able to invoke a server without adding it to configuration. Use `--stdio` or `--url` instead of `--server`:

Examples:

    mcpctl tools list --stdio -- npx -y example-mcp-server

    mcpctl tools list \
      --url https://example.com/mcp \
      --header-env Authorization=MCP_AUTHORIZATION

    mcpctl tools call \
      --url https://example.com/mcp \
      search \
      --json '{"query":"MCP"}'

These flags are mutually exclusive with `--server`. The tool name remains positional after the server flags.

Cobra argument parsing for stdio commands must preserve all arguments after `--` exactly. The stdio command must be executed directly, not through a shell.

Command arguments are never shell-evaluated. Help must include working examples for both named and ephemeral invocation.

### 4.3.2 Tool argument input

Support exactly one of:

- `--json STRING`
- `--json-file PATH`
- repeated `--arg KEY=VALUE`

These modes are mutually exclusive.

`--json` and `--json-file` must decode to a JSON object. Reject arrays, strings, numbers, booleans, and null as the top-level arguments value.

For `--arg`:

- Version 1 supports top-level properties only.
- Split on the first `=`.
- Parse the value as JSON if it is valid JSON.
- Otherwise treat it as a string.
- Reject duplicate keys.
- Empty keys are invalid.

Examples:

    --arg query=MCP
    --arg limit=10
    --arg enabled=true
    --arg tags='["go","mcp"]'
    --arg options='{"caseSensitive":false}'

To force a string that resembles JSON, users may provide a JSON string:

    --arg value='"true"'

Do not invent dotted-path or bracket notation in version 1. Complex nested arguments should use `--json` or `--json-file`.

### 4.3.3 Pagination

`tools list` must retrieve all pages by default.

Optional flags:

    --page-size N
    --max-pages N

If MCP does not expose a page-size request field, `--page-size` may be omitted. `--max-pages` should protect against invalid servers that repeat cursors indefinitely.

The client must detect repeated cursors and terminate with a protocol error rather than loop forever.

---

## 5. Configuration

## 5.1 Default paths

Use the platform-appropriate user configuration directory from `os.UserConfigDir()`.

Default file:

- Linux: typically `~/.config/mcpctl/config.toml`
- macOS: under the directory returned by `os.UserConfigDir()`
- Windows: under the directory returned by `os.UserConfigDir()`

The actual path must be determined by Go at runtime, not hard-coded by operating system.

`--config` overrides the path.

If no configuration file exists:

- commands using an ephemeral server must continue to work
- commands requiring a named server must return a clear configuration error
- read-only commands must not create a file implicitly

## 5.2 Example configuration

    version = 1

    [defaults]
    timeout = "30s"
    connect_timeout = "15s"

    [servers.local-fs]
    transport = "stdio"
    command = "npx"
    args = [
      "-y",
      "@modelcontextprotocol/server-filesystem",
      "/tmp"
    ]
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

## 5.3 Configuration data model

Conceptual Go structures:

    type Config struct {
        Version  int                     `toml:"version"`
        Defaults DefaultsConfig          `toml:"defaults"`
        Servers  map[string]ServerConfig `toml:"servers"`
    }

    type DefaultsConfig struct {
        Timeout        string `toml:"timeout"`
        ConnectTimeout string `toml:"connect_timeout"`
    }

    type ServerConfig struct {
        Transport   string            `toml:"transport"`
        Command     string            `toml:"command"`
        Args        []string          `toml:"args"`
        CWD         string            `toml:"cwd"`
        Env         map[string]string `toml:"env"`
        URL         string            `toml:"url"`
        Headers     map[string]string `toml:"headers"`
        HeaderEnv   map[string]string `toml:"header_env"`
        BearerToken *TokenSource      `toml:"bearer_token"`
    }

    type TokenSource struct {
        Env string `toml:"env"`
    }

Validation rules:

- `version` must equal `1`
- server names must be non-empty
- transport must be `stdio` or `streamable-http`
- stdio requires `command`
- stdio must not specify `url`
- Streamable HTTP requires an absolute `http` or `https` URL
- Streamable HTTP must not specify `command` or `args`
- environment-variable references must name non-empty variables
- duplicate or conflicting authorization methods must produce a clear error
- unknown fields should be rejected to catch configuration mistakes

## 5.4 Environment behavior

Configured stdio environment variables are additions or overrides to the inherited process environment.

Do not replace the entire process environment.

HTTP `header_env` means:

- configuration key: HTTP header name
- configuration value: environment variable name
- runtime header value: contents of that environment variable

Missing referenced environment variables must produce an authentication/configuration error before connecting.

Bearer-token behavior:

- read the token from the configured environment variable
- send `Authorization: Bearer <token>`
- never log the token
- reject configuration that also defines an `Authorization` header through another mechanism

---

## 6. Internal Architecture

Use a small internal interface that hides the SDK from command handlers.

Conceptual interface:

    type Client interface {
        Initialize(ctx context.Context) error
        ListTools(ctx context.Context, cursor string) (ToolPage, error)
        CallTool(
            ctx context.Context,
            name string,
            arguments map[string]any,
        ) (ToolResult, error)
        Close() error
    }

The exact interface may be adjusted to match the SDK. Required architectural properties:

- Cobra command handlers do not instantiate SDK transports directly.
- Transport selection is performed by a client factory.
- SDK types are converted to stable internal or output types at package boundaries.
- Closing a client terminates HTTP sessions and stdio child processes.
- Initialization occurs once per command invocation.
- Context cancellation propagates through initialization and calls.

Suggested package layout:

    cmd/mcpctl/
      main.go

    internal/cli/
      root.go
      server.go
      tools.go
      flags.go

    internal/client/
      client.go
      factory.go
      sdk_client.go
      stdio.go
      streamable_http.go

    internal/config/
      config.go
      load.go
      save.go
      validate.go
      paths.go

    internal/auth/
      headers.go
      redaction.go

    internal/arguments/
      parse.go
      validate.go

    internal/output/
      output.go
      human.go
      json.go
      jsonl.go
      yaml.go

    internal/apperror/
      error.go
      exitcode.go

    internal/process/
      process.go
      process_unix.go
      process_windows.go

    internal/testserver/
      stdio/
      http/

Do not create abstraction layers that provide no immediate value. In particular, avoid implementing a generic JSON-RPC framework.

---

## 7. MCP Session Behavior

For every tools command:

1. Resolve the named or ephemeral server.
2. Validate its configuration.
3. Resolve environment-based credentials.
4. Create a command-scoped context.
5. Start or connect to the transport.
6. Initialize the MCP client session.
7. Verify that the server supports tools when capability information is available.
8. Perform the requested operation.
9. Render the result.
10. Close the MCP session and transport.
11. Terminate the child process if using stdio.

The SDK should handle:

- `initialize`
- capability negotiation
- initialized notifications
- protocol version headers
- Streamable HTTP session IDs
- JSON-RPC request IDs
- response correlation
- SSE or streaming response parsing
- server notifications
- transport-level framing

If `--protocol-version` is specified, pass it through only if the SDK supports an explicit version override. Otherwise return an unsupported-option error rather than silently ignoring it.

---

## 8. Stdio Transport

## 8.1 Process startup

Start the configured command with `os/exec.Cmd`.

Requirements:

- execute the command directly
- never use `sh -c`, `cmd.exe /C`, PowerShell, or another shell
- preserve configured arguments exactly
- apply configured working directory
- inherit the current environment plus configured additions/overrides
- connect MCP protocol traffic only to stdin/stdout
- keep stderr separate from protocol traffic

## 8.2 Server stderr

Default behavior:

- forward server stderr to client stderr
- prefixing is optional in human mode
- do not alter stdout machine-readable output

When `--output json`, `jsonl`, or `yaml` is selected, server stderr must still go to stderr.

A future option may capture stderr, but version 1 does not need to include stderr in result objects.

## 8.3 Cancellation and cleanup

On Unix:

- create a process group for the stdio server
- on graceful cancellation, close the MCP client and allow a short shutdown period
- if still running, send a termination signal to the process group
- after a second short period, force kill the process group

On Windows:

- use the strongest reliable process-tree cleanup available without requiring external executables
- at minimum, ensure the direct child process is terminated
- isolate platform-specific behavior in `process_windows.go`

Suggested shutdown timing:

- graceful close: up to 2 seconds
- termination wait: up to 2 seconds
- force kill afterward

An unexpected child exit must become a transport error. Include the exit code and safe stderr context if available, but never leak secrets.

---

## 9. Streamable HTTP Transport

Use the official SDK’s Streamable HTTP client transport.

Requirements:

- support `http` and `https`
- default to normal Go certificate verification
- support static headers
- support headers sourced from environment variables
- support bearer-token authentication
- honor context cancellation and deadlines
- use a configurable connection timeout
- follow safe HTTP redirect behavior
- do not forward authorization headers to a different host during redirects
- use a descriptive `User-Agent`, for example:
  `mcpctl/<version>`

The HTTP client should use a cloned `http.Transport`, not mutate `http.DefaultTransport`.

Configure reasonable defaults:

- proxy behavior from environment
- TLS handshake timeout
- response-header timeout where compatible with streaming
- idle connection cleanup on client close

Do not impose a short response-body timeout that breaks long-running MCP calls or streaming responses. The command context is the authoritative overall timeout.

An `--insecure-skip-verify` option should not be included in version 1 unless specifically required. If later added, it must be explicit and emit a warning.

---

## 10. Tool Listing and Description

## 10.1 `tools list`

Human output should show at least:

- tool name
- description, truncated to a reasonable width

Example:

    NAME          DESCRIPTION
    read_file     Read the complete contents of a file
    write_file    Write content to a file

JSON output should preserve complete tool information, including input and output schemas when supplied by the server.

Suggested normalized output:

    {
      "server": "local-fs",
      "tools": [
        {
          "name": "read_file",
          "description": "Read the complete contents of a file",
          "inputSchema": {},
          "outputSchema": {}
        }
      ]
    }

JSON Lines output should emit one tool object per line without an enclosing array.

YAML output should correspond structurally to JSON output.

## 10.2 `tools describe`

Find the named tool after retrieving all tool pages.

If it does not exist:

- return a not-found error
- include the requested name
- optionally suggest close matches using a simple edit-distance algorithm
- do not treat this as a transport error

Human output must include:

- name
- description
- input schema, formatted as indented JSON or a readable property table
- output schema, if supplied
- annotations, if supplied

JSON and YAML output should preserve the complete tool definition.

---

## 11. Tool Invocation

Before invoking a tool:

1. Retrieve the server's tools.
2. Locate the requested tool.
3. Parse arguments.
4. Validate arguments against the tool input schema unless `--no-validate` is used.
5. Call the tool through MCP.

If the tool does not exist, do not send `tools/call`.

Future optimization: for a v1 client that creates a new session per command invocation, a full `tools/list` before every `tools call` adds latency proportional to the server's tool count. This is acceptable for v1. A future version may cache the tool list within a persistent session or add a `--tool` flag to skip the list round-trip when the tool name is already known. The architecture should not preclude this optimization.

If local validation fails:

- do not call the server
- report all useful validation failures when feasible
- identify JSON paths, expected types, and actual values without exposing secrets unnecessarily
- return the invalid-arguments exit code

If the server schema cannot be compiled:

- in human mode, print a warning to stderr
- continue with the call
- in machine-readable modes, do not contaminate stdout
- log details at debug level

## 11.1 Tool result preservation

Do not flatten an MCP tool result to a single text string internally.

Preserve:

- `content`
- text content
- image/audio/resource content metadata
- structured content
- `isError`
- any SDK-exposed result metadata

JSON output should be a faithful, stable representation of the MCP tool result.

Human output behavior:

- print text content directly
- pretty-print structured content
- show metadata for binary or embedded content
- do not print raw binary bytes to a terminal by default
- indicate when a tool result has `isError = true`

A tool result with `isError = true` is a successful MCP exchange but a failed tool execution. It must use the tool-error exit code.

---

## 12. Output and Logging

## 12.1 Output contract

Stdout:

- requested command results only

Stderr:

- logs
- warnings
- progress
- server stderr
- diagnostics

Machine-readable stdout must never contain:

- log prefixes
- color escape sequences
- progress indicators
- warnings
- server stderr

JSON output must be valid as one complete JSON document.

JSON Lines output must contain one valid JSON object per line.

YAML output must be one valid YAML document unless command documentation explicitly states otherwise.

## 12.2 Logging

Use `log/slog`.

Log levels:

- `debug`
- `info`
- `warn`
- `error`

Default: `warn`.

Debug logging may include:

- selected transport
- command lifecycle
- request method names
- timing
- status codes
- process IDs
- negotiated protocol version

Debug logging must not include:

- bearer tokens
- authorization headers
- cookies
- complete credential-bearing URLs
- secrets from environment variables
- raw tool arguments by default

Provide centralized redaction for sensitive header names:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`
- `X-API-Key`
- names containing `token`, `secret`, `password`, or `key`, case-insensitively

---

## 13. Errors and Exit Codes

Define typed application errors and map them to stable exit codes.

Required exit codes:

- `0`: success
- `1`: unspecified/internal failure
- `2`: command-line usage error
- `3`: configuration error
- `4`: authentication error
- `5`: connection or transport error
- `6`: MCP protocol or initialization error
- `7`: tool not found
- `8`: invalid tool arguments
- `9`: tool returned `isError = true`
- `10`: timeout
- `130`: interrupted by Ctrl-C where appropriate

Do not call `os.Exit` outside `main.go`.

Command packages should return errors. `main.go` should:

1. execute the root command
2. classify the returned error
3. print the error to stderr if it has not already been rendered
4. exit with the mapped code

Machine-readable error output may be added with a later `--errors-as-json` option. Version 1 errors may always be human-readable on stderr.

Error messages should include context but avoid duplicated chains such as:

    failed: failed to connect: connection failed: ...

Prefer:

    connect to server "remote": HTTP 401 Unauthorized

---

## 14. Signals and Timeouts

Handle:

- `SIGINT`
- `SIGTERM` on supported operating systems

Use `signal.NotifyContext`.

The root command context must be canceled on signal receipt.

Timeout semantics:

- `connect-timeout` applies to transport establishment and initialization
- `timeout` applies to the complete command after connection begins, or to the entire operation if separating phases is impractical
- cancellation must propagate to SDK operations
- timeout errors must map to exit code `10`
- user interruption should map to `130`

A zero timeout is not valid. The shortest allowed timeout is `1s`. This prevents the client from hanging indefinitely on a misbehaving stdio server. If the user needs an unbounded wait, a very large timeout (e.g., `24h`) is preferred over zero. The maximum allowed timeout is `24h`. Negative durations are invalid.

---

## 15. Security Requirements

1. Never execute stdio command strings through a shell.
2. Never log credentials or authorization headers.
3. Do not save literal credentials unless the user explicitly selects a clearly labeled unsafe option.
4. Use normal TLS verification.
5. Do not silently follow redirects while forwarding credentials to another origin.
6. Validate configuration file permissions where practical.
7. If a configuration file contains literal sensitive headers, warn when permissions allow access by other users on Unix.
8. Treat MCP server output, tool descriptions, schemas, and results as untrusted data.
9. Avoid terminal escape injection in human-readable tables. Strip or escape unsafe control characters.
10. Place reasonable limits on:
    - maximum pagination count
    - response sizes where the SDK permits safe limits
    - rendered terminal output
    - captured stderr
11. Do not include secrets in panic reports.
12. Avoid panics for malformed server responses; return protocol errors.

---

## 16. Testing Strategy

## 16.1 Unit tests

Provide unit tests for:

- configuration loading
- unknown configuration fields
- server validation
- default path resolution
- environment merging
- header and bearer-token resolution
- secret redaction
- argument mode exclusivity
- `--arg` parsing
- JSON object validation
- JSON Schema validation
- output formatting
- error classification
- exit-code mapping
- repeated pagination cursor detection
- terminal control-character sanitization

Use table-driven tests where appropriate.

## 16.2 Stdio integration tests

Create a deterministic test MCP server executable or test helper process.

It should support scenarios for:

- successful initialization
- multiple pages of tools
- tool invocation with structured arguments
- text results
- structured results
- tool-level errors
- malformed responses
- unexpected process exit
- delayed initialization
- delayed tool call
- stderr output
- graceful shutdown
- ignored shutdown requiring forced termination

Do not require Node.js, Python, or an external MCP server for automated tests.

## 16.3 HTTP integration tests

Use `httptest.Server`.

Cover:

- successful Streamable HTTP initialization
- required MCP session headers
- multiple tool pages
- tool invocation
- streaming/SSE responses where applicable
- static headers
- environment-derived headers
- bearer-token headers
- HTTP 401 and 403
- HTTP 404
- HTTP 500
- connection timeout
- command timeout
- cancellation
- redirect to the same origin
- redirect to another origin without credential forwarding
- malformed protocol responses

Tests should verify behavior rather than reproduce SDK internals.

## 16.4 End-to-end tests

Build the binary and run representative commands:

    mcpctl --version
    mcpctl server list
    mcpctl tools list ...
    mcpctl tools describe ...
    mcpctl tools call ...

Assert:

- stdout correctness
- stderr separation
- exit codes
- cleanup of child processes
- valid JSON/YAML output

## 16.5 Quality gates

CI must run:

    go test ./...
    go test -race ./...
    go vet ./...
    staticcheck ./...

Use `golangci-lint` only if its configuration remains small and stable. `staticcheck` is sufficient for the first version.

Aim for strong coverage of internal logic, but do not use a coverage percentage as a substitute for transport integration tests.

---

## 17. Build and Release

Use GoReleaser.

Build targets:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64
- Windows amd64
- Windows arm64 if all dependencies support it

Release artifacts should contain:

- executable
- README
- LICENSE
- shell completions where practical
- checksums

Inject version information using linker flags.

Version command output:

    mcpctl version <version>
    commit: <commit>
    built: <timestamp>
    go: <go version>

`mcpctl --version` may print a concise one-line form.

Prefer pure-Go dependencies. Avoid CGo unless there is a documented, compelling reason.

---

## 18. Documentation Requirements

Provide a README containing:

1. Installation instructions.
2. Quick-start examples.
3. Named server configuration.
4. Ephemeral stdio usage.
5. Ephemeral HTTP usage.
6. Tool listing and invocation.
7. Argument parsing rules.
8. Output format examples.
9. Authentication through environment variables.
10. Exit-code table.
11. Security warning for stdio commands.
12. Troubleshooting guidance.
13. Supported MCP transports.
14. Current limitations.

Include example configuration without real credentials.

Help output should be sufficient for ordinary use without consulting the README.

---

## 19. Delivery Phases

### Phase 1: Project skeleton

Deliver:

- Go module
- Cobra root command
- version command
- logging
- error and exit-code framework
- configuration loading
- CI
- basic release configuration

### Phase 2: Stdio MVP

Deliver:

- stdio client factory
- MCP initialization
- `tools list`
- `tools describe`
- `tools call`
- process cleanup
- stdio integration test server
- JSON and human output

### Phase 3: Streamable HTTP

Deliver:

- Streamable HTTP transport
- headers and bearer tokens
- timeout behavior
- HTTP integration tests
- redirect credential protection

### Phase 4: UX and validation

Deliver:

- TOML server management commands
- JSON Lines and YAML output
- JSON Schema validation
- pagination safeguards
- terminal sanitization
- improved human formatting

### Phase 5: Release readiness

Deliver:

- race-tested implementation
- cross-platform builds
- GoReleaser artifacts
- README and examples
- shell completions
- complete acceptance-test run

---

## 20. Acceptance Criteria

The implementation is complete when all of the following pass.

### 20.1 Stdio server

Given a valid stdio MCP server:

    mcpctl tools list --stdio -- ./test-mcp-server

must:

- start the server
- initialize MCP
- retrieve all tool pages
- print tools
- terminate the server
- exit `0`

A tool call such as:

    mcpctl tools call \
      --stdio \
      echo \
      --json '{"message":"hello"}' \
      -- ./test-mcp-server

or the final documented equivalent syntax must:

- send an object containing `message`
- preserve the MCP result
- print `hello`
- exit `0`

### 20.2 Named server

After:

    mcpctl server add --name local --stdio -- ./test-mcp-server

the command:

    mcpctl tools list --server local

must use the saved definition successfully.

### 20.3 Streamable HTTP

Given a compatible HTTP test server:

    mcpctl tools list --url http://127.0.0.1:<port>/mcp

must initialize and list tools.

The client must preserve SDK-required session and protocol-version headers.

### 20.4 Machine-readable output

The command:

    mcpctl tools list --server local --output json

must:

- emit valid JSON to stdout
- emit no logs or decorations to stdout
- place diagnostics on stderr
- exit `0`

### 20.5 Validation

Given a tool requiring:

    {
      "type": "object",
      "required": ["path"],
      "properties": {
        "path": { "type": "string" }
      }
    }

calling it with:

    --json '{}'

must:

- fail before `tools/call`
- report the missing `path`
- exit `8`

Using `--no-validate` must send the call to the server.

### 20.6 Tool errors

If a tool returns `isError = true`, the client must:

- render the returned content
- identify it as a tool error
- exit `9`

### 20.7 Cancellation

When a long-running invocation receives Ctrl-C:

- the MCP operation is canceled
- the stdio process is terminated
- no orphan child remains in the supported test environment
- the client exits with `130`

### 20.8 Authentication secrecy

Automated tests must verify that bearer tokens do not appear in:

- stdout
- normal stderr
- debug logs
- formatted errors

### 20.9 Cross-platform build

The project must compile for at least:

- `GOOS=linux GOARCH=amd64`
- `GOOS=darwin GOARCH=arm64`
- `GOOS=windows GOARCH=amd64`

---

## 21. Future Extensions

The architecture should permit, but not yet implement:

- `resources list`, `resources read`, and subscriptions
- `prompts list` and `prompts get`
- OAuth discovery and authorization-code flows
- OS keychain integration
- client-side support for server sampling requests
- elicitation
- persistent sessions
- interactive schema-driven argument prompts
- output of binary content to files
- MCP server health checks
- import of server definitions from other MCP clients
- proxy and gateway modes
- shell completion for configured servers and tool names

These features must not delay version 1.

---

## 22. Coding-Agent Instructions

When implementing this specification:

1. Begin by verifying the current official Go MCP SDK APIs for stdio and Streamable HTTP.
2. Create a short transport spike before building all CLI commands.
3. Prove initialization, `tools/list`, and `tools/call` over both transports.
4. Use the SDK rather than manually reproducing protocol behavior.
5. Keep SDK-specific code under `internal/client`.
6. Commit in small phases with tests.
7. Do not silently omit a requirement because an SDK API is inconvenient.
8. If an SDK limitation blocks a requirement:
   - document the limitation
   - add a focused adapter or workaround
   - add a regression test
   - avoid exposing the workaround to command handlers
9. Prioritize correct lifecycle handling and stdout/stderr separation over decorative CLI features.
10. Treat cross-platform process cleanup, authentication redaction, and machine-readable output as core correctness requirements.
````
