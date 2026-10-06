# mcpgen

Licensed under [Apache-2.0](LICENSE). Copyright 2026 Klim Krivoguzov.

`mcpgen` generates a complete Go MCP server from `mcp.yaml`. Implement the
generated `Handler` interface, then call `NewServer(handler).Run(ctx)`.
The generated package owns SDK setup, tool metadata, schemas, registration,
decoding, validation, dispatch, encoding, and stdio transport.

Requires Go 1.25 or newer. Generated packages use the
[official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk), pinned
here to v1.8.0. They have no runtime dependency on `mcpgen`.

## Run the example

```sh
go generate ./...
go test ./...
go run ./example/cmd/server
```

The server communicates with an MCP client over stdin and stdout. The example
implements `echo`, `create_pet`, `get_pet`, and `search` for cats and dogs.
Each pet has a name and a required `kind` enum (`cat` or `dog`). Pets live in
memory for the lifetime of the process.

The specification is `example/mcp.yaml`. Generated files live in `example/internal/mcpapi`.
Handwritten handlers live in `example/internal/mcpserver`; application storage lives in
`example/internal/service`. Only `generate.go` is handwritten in the generated package.

## Generate your own server

Build or install the CLI from this checkout:

```sh
go install .
mcpgen -spec mcp.yaml -out internal/mcpapi -package mcpapi
```

For a published release, use `go install github.com/phaseant/mcpgen@latest`.

The defaults are `-spec mcp.yaml`, `-out internal/mcpapi`, and `-package mcpapi`.
In another application, add the SDK dependency:

```sh
go get github.com/modelcontextprotocol/go-sdk@v1.8.0
```

Put a generation directive in `internal/mcpapi/generate.go`:

```go
package mcpapi

//go:generate mcpgen -spec ../../mcp.yaml -out . -package mcpapi
```

A minimal specification:

```yaml
mcp: "1"
info:
  name: echo
  version: 1.0.0
tools:
  echo:
    description: Echo a message.
    input:
      type: object
      required: [message]
      properties:
        message:
          type: string
          minLength: 1
    output:
      type: object
      required: [message]
      properties:
        message:
          type: string
```

Implement its generated interface:

```go
type Handler struct{}

var _ mcpapi.Handler = (*Handler)(nil)

func (*Handler) Echo(ctx context.Context, input mcpapi.EchoInput) (mcpapi.EchoOutput, error) {
    return mcpapi.EchoOutput{Message: input.Message}, nil
}
```

Start the generated server:

```go
server, err := mcpapi.NewServer(&Handler{})
if err != nil {
    return err
}
return server.Run(ctx)
```

`Run` and `RunStdio` both use stdio. `WithLogger(*slog.Logger)` configures the SDK
logger. `MCP()` provides SDK access for custom transports and integrations.
Ordinary handlers and startup code need no SDK imports.

## Integrate with an existing server

Mount the generated Streamable HTTP endpoint on your existing router:

```go
server, err := mcpapi.NewServer(handler, mcpapi.WithMiddleware(toolLogging))
if err != nil {
    return err
}
endpoint := server.HTTPHandler(&mcpapi.HTTPOptions{
    Stateless:    true,
    JSONResponse: true,
})
mux.Handle("/mcp", authMiddleware(endpoint))
```

`endpoint` is a standard `http.Handler`. Existing HTTP middleware can provide
authentication, tracing, rate limits, or context values. Mount it once during
startup; your existing `http.Server` owns listening and shutdown. Other routes
continue to use the same router. Frameworks that accept `http.Handler` can mount
the same endpoint.

`HTTPHandler(nil)` uses SDK defaults: stateful sessions and SSE responses.
`HTTPOptions.SessionTimeout` controls idle stateful sessions. Stateless mode
avoids retained sessions and supports ordinary request/response tools. Options
are copied when the endpoint is created. HTTP context values are available in
tool middleware and handlers according to the SDK's transport context rules.

Generated tool middleware receives the tool name and a decoded input of the
generated type:

```go
func toolLogging(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
    return func(ctx context.Context, tool string, input any) (any, error) {
        started := time.Now()
        output, err := next(ctx, tool, input)
        slog.InfoContext(ctx, "MCP tool", "tool", tool, "duration", time.Since(started), "error", err)
        return output, err
    }
}
```

`WithMiddleware(first, second)` runs `first → second → handler`, then unwinds in
reverse order. Multiple `WithMiddleware` options preserve declaration order.
Input validation and decoding happen before middleware. Output validation and
encoding happen after middleware. Middleware can short-circuit with `ToolError`
or change context with `next(updatedContext, tool, input)`. Input and successful
output values must retain their generated types. Middleware that changes input
fields is responsible for preserving their validated constraints. Middleware
functions can run concurrently and should keep request state inside the call.

If your application already owns an SDK MCP server, register the whole generated
tool set on it:

```go
if err := mcpapi.RegisterTools(existingMCPServer, handler, toolLogging); err != nil {
    return err
}
```

Call `RegisterTools` during startup. It keeps the existing server's metadata,
options, middleware, and unrelated tools. Matching tool names are replaced using
the SDK's registration behavior. Generated middleware applies only to generated
tools. The existing server continues to own transport and lifecycle.

For protocol-level middleware around initialization, tool listing, and other MCP
messages, use the SDK's native `AddReceivingMiddleware` or
`AddSendingMiddleware` on the existing SDK server or on `server.MCP()`.
`NewServer` remains the default API when you do not already have an MCP server.

## Specification and types

`mcp: "1"`, `info.name`, `info.version`, and at least one tool are required.
Each tool requires `input` and `output` schemas. Input must resolve to an object.
Output may be an object, array, or primitive.

| Schema feature | Generated behavior |
| --- | --- |
| `object`, `properties`, `required` | Structs with JSON tags; optional properties are pointers |
| `string`, `integer`, `number`, `boolean` | `string`, `int64`, `float64`, `bool` |
| `array`, `items` | Slices, including nested arrays and objects |
| `schemas`, `$ref: '#/schemas/Name'` | Named Go types; references reuse those types |
| Primitive `enum` | Named types and typed constants |
| `description` | Schema metadata and Go documentation |
| Tool `title`, `description`, `annotations` | Complete advertised MCP tool metadata |

String enum constants use the type name plus the enum name. Boolean constants
use `True` and `False`. Numeric constants use `Value1`, `Value2`, and so on.
Names preserve common initialisms: `pet_id` becomes `PetID`.
Conflicting Go names produce a generation error.

Supported validation keywords:

- Numbers: `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`.
- Strings: `minLength`, `maxLength`, `pattern`.
- Arrays: `minItems`, `maxItems`, `uniqueItems`.
- Objects: boolean `additionalProperties`, `minProperties`, `maxProperties`.
- All primitive types: `enum`.

The SDK validates input before the handler runs and validates serialized output
before returning it. Optional properties accept absence, not JSON `null`.
Required arrays must contain a non-nil slice when returned; use `[]T{}` for an
empty array. Go structs contain only declared fields. Set
`additionalProperties: false` to reject undeclared input fields; otherwise the
SDK accepts them and decoding discards them.

Annotations support `readOnlyHint`, `destructiveHint`, `idempotentHint`,
`openWorldHint`, and `title`. Explicit `false` values for the pointer-valued
destructive and open-world hints are preserved.

Tool names use 1–128 ASCII letters, digits, underscores, dots, or hyphens.
Schema names start with a letter and use letters, digits, underscores, dots, or
hyphens. Property names may also start with an underscore. Names must map to
valid, distinct Go identifiers.

This version rejects recursive or external references, union/composition
schemas, nullable types, schema-valued `additionalProperties`, and unsupported
keywords such as `format` and `default`. A `$ref` may have a `description`
sibling only. Diagnostics include the filename, schema path, and source position
where available. Add broader schema support when an application needs it.

## Errors and incomplete handlers

Return expected failures without using SDK types:

```go
return mcpapi.Pet{}, mcpapi.NewToolError("pet_not_found", "pet does not exist")
```

`ToolError` values, pointers, and wrapped errors become MCP tool errors with a
JSON text content block containing `code` and `message`. Unexpected handler
errors become JSON-RPC internal errors with a generic message.

`UnimplementedHandler` is available for incremental development. Its methods
return `ErrNotImplemented`, an expected tool error. The recommended implementation
uses the compile-time interface assertion instead of embedding the helper.

Add a tool to the spec and run `go generate ./...`. The application stops
compiling until its handler implements the new method. Adding that method is
enough to register, advertise, decode, validate, dispatch, and encode the tool.

## Generation checks

Output is sorted and formatted with `go/format`. Generation checks file ownership
before writing, preserves unchanged files, and replaces changed files atomically.
It refuses to overwrite handwritten files or symlinks.

Check drift without changing files:

```sh
go run . -spec example/mcp.yaml -out example/internal/mcpapi -check
```

CI runs generation, checks the Git diff, checks generation drift, runs race tests,
and runs `go vet`. Golden tests cover the generated echo package. Update them
after an intentional generator change:

```sh
go test ./internal/generate -run TestGolden -update
```

Protocol tests cover metadata, structured output, input and output validation,
error adaptation, and optional fields. A separate test adds a tool in a temporary
application, checks the missing-method compile error, adds only the handler
method, then tests the automatically registered tool through the SDK client.

The generator uses a strict parser and validator, a normalized generation model,
separate generators for types, handlers, schemas, tools, server, and adapters,
then `go/format`. Per-tool interfaces are omitted until an application requires
them.
