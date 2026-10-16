# XGo Project Driver Protocol v1

This document is the canonical contract owned by
`github.com/goplus/mod/driverprotocol`. It specifies the driver metadata,
request model, argv encoding, and provenance carried between XGo and a project
driver. Framework-specific execution, packaging, caching, and release formats
are outside its scope.

The words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative. Go
identifiers refer to exported types in `driverprotocol` or `xgomod`.

## Metadata

A project may attach one driver directive to the nearest preceding `project`
directive:

```text
project main.foo Game example.com/framework math
driver v1 example.com/framework/cmd/projectdriver
```

The grammar is:

```text
driver <protocol> <driver-import-path>
```

- `protocol` MUST match `v[1-9][0-9]*`; this document defines only `v1`.
- `driver-import-path` MUST be a valid Go import path. Relative paths,
  absolute paths, and `path@version` forms are invalid.
- A project MUST NOT declare more than one driver.
- A known but malformed directive is an error in strict and lax parsing.
- The directive does not contain a module version. The producer MUST resolve
  the declaring module and driver package from the application's effective Go
  module/workspace graph.

## Invocation frame

The driver executable receives this frame after `argv[0]`:

```text
xgo-driver-v1 <action> <options> [-- <application-args>...]
```

`<action>` is exactly `run` or `build`. Every option uses one argv element in
the exact form `--name=value`; the first `=` separates its name and value.
Separate `--name value` forms, positional arguments among options, unknown
options, repeated singular options, and any element containing NUL are invalid.
Option order is insignificant to `Parse`; `Encode` emits the canonical order
defined below. The order within each repeatable option group and within the
application arguments MUST be preserved; interleaving between option groups is
not identity.

`graph-flag` and `build-flag` are the only repeatable options. Repetition of
the same canonical flag name within either group is invalid.

The request is encoded directly in argv. The protocol does not define a JSON
request file and does not consume standard input. A driver launcher MUST
inherit standard input, standard output, and standard error from its caller.

## Canonical option order

`Encode` emits options in this order. “Required” means the option MUST be
present; it does not imply that an empty value is valid.

| Option | Cardinality | Request field |
| --- | --- | --- |
| `project-dir` | required, singular | `Project.Dir` |
| `project-file` | required, singular | `Project.File` |
| `module-root` | required, singular | `Project.ModuleRoot` |
| `driver-package` | required, singular | `DriverPackage` |
| `selected-path` | required, singular | `DriverOrigin.Selected.Path` |
| `selected-version` | required, singular | `DriverOrigin.Selected.Version` |
| `origin-main` | required, singular | `DriverOrigin.Main` |
| `selected-dir` | source-dependent | `DriverOrigin.Selected.Dir` |
| `selected-gomod` | source-dependent | `DriverOrigin.Selected.GoMod` |
| `replace-path` | source-dependent | `DriverOrigin.Replace.Path` |
| `replace-version` | source-dependent | `DriverOrigin.Replace.Version` |
| `replace-dir` | source-dependent | `DriverOrigin.Replace.Dir` |
| `replace-gomod` | source-dependent | `DriverOrigin.Replace.GoMod` |
| `project-ext` | required, singular | `Project.Extension` |
| `project-full-ext` | required, singular | `Project.FullExtension` |
| `pack-dir` | optional group | `Project.Pack.Directory` |
| `pack-index` | optional group | `Project.Pack.IndexFile` |
| `declaration-file` | required, singular | `Declaration.Path` |
| `declaration-sha256` | required, singular | `Declaration.SHA256` |
| `target-modfile` | required, singular | `TargetModFile.Path` |
| `target-modfile-sha256` | required, singular | `TargetModFile.SHA256` |
| `go-command` | required, singular | `Graph.GoCommand` |
| `graph-work-dir` | required, singular | `Graph.WorkDir` |
| `go-work` | required, singular | `Graph.GoWork` |
| `graph-flag` | zero or more | `Graph.Flags` |
| `build-flag` | zero or more | `BuildFlags` |
| `output` | build only, required group | `Output.Staging` |
| `final-output` | build only, required group | `Output.Final` |

The selected source and replacement source forms are mutually exclusive:

- without a replacement, `selected-dir` and `selected-gomod` MUST both be
  present;
- with a replacement, all four `replace-*` options MUST be present, including
  `replace-version=` for a local replacement, and both selected-source options
  MUST be absent.

`pack-dir` and `pack-index` MUST either both be present or both be absent.
`origin-main` is exactly `true` or `false`.

For `run`, options MUST be followed by `--`; every later argv element is an
application argument and is preserved verbatim, including empty strings and
additional `--` elements. `run` MUST NOT contain output options.

For `build`, `output` and `final-output` MUST both be present. `build` MUST NOT
contain `--` or application arguments. The two output path spellings MUST be
lexically different; structural validation does not prove file identity.

## Structural validation

`Request.Validate` and `Parse` validate structure without reading the
filesystem.

### Paths and project shape

The following fields MUST be absolute, clean, non-empty host paths without
NUL: project directory and file, module root, declaration file, target
modfile, Go command, graph work directory, build staging output, and build
final output. `Graph.GoWork` is either exactly `off` or an absolute clean path.

The project file MUST be a top-level file in the project directory, and the
project directory MUST be inside the module root.

Pack paths are portable protocol paths rather than host paths:

- `pack-dir` is `.` or a clean, non-empty, relative slash path that does not
  escape the project;
- `pack-index` is one plain file name;
- elements reject characters below U+0020, Windows-forbidden characters and
  device names, trailing dots or spaces, backslashes, and path traversal.

Project extension strings MUST be non-empty and contain no NUL. Their semantic
relationship to framework metadata is verified by the consumer.

### Module provenance

`DriverOrigin` separates logical module selection from the physical source:

- `Selected` is the module path/version chosen by the effective Go graph;
- `Replace`, when present, is the source that supplies its files;
- `Effective()` returns the source-bearing identity.

`ResolvedModule.Equal` compares `Main`, every `Selected` field, replacement
presence, and every `Replace` field when a replacement is present.

A main module has an empty selected version and no replacement. `Selected.Path`
MUST be a valid Go module path. A non-main module has a canonical selected
version. Without a replacement, `Selected.Dir` and `Selected.GoMod` are
non-empty absolute clean path spellings. With a replacement, those selected
source fields are empty and `Replace.Dir` and `Replace.GoMod` are non-empty
absolute clean path spellings. A local replacement has an empty version and an
absolute clean `Path` equal to `Dir`; a versioned replacement has a canonical
module path and version.

`Request.Validate` calls `ResolvedModule.ValidateSyntax` and does not access
the filesystem. `ResolvedModule.Validate` is the separate operation that
checks live source files and the split Go module-cache layout.

The driver package MUST be a valid Go import path within the selected module
path. The declaration file MUST be `gox.mod` or `gop.mod` directly inside the
effective source directory.

`Declaration` and `TargetModFile` each carry an absolute path and a lowercase,
64-character SHA-256 digest. The latter binds the exact modfile bytes used to
derive the class graph. It may identify the project's `go.mod`, an explicit
`-modfile`, or a download-cache `.mod`; it need not be below the project or
module root or equal the active modfile. No adjacent sidecar file is implicitly
part of this identity.

### Flags

Every flag uses canonical `-name=value` syntax with a non-empty name and value.
The v1 codec accepts only:

| Group | Accepted values |
| --- | --- |
| graph | `-mod=mod`, `-mod=readonly`, `-mod=vendor`; absolute `-modfile=<path>` and `-overlay=<path>` |
| build | `-v=true`, `-x=true`, `-work=true`, `-trimpath=true`, `-buildvcs=false` |

This table defines the wire vocabulary, not a promise that every driver or
dispatch mode implements every listed value. A producer MUST reject a value
that its selected driver path cannot honor before execution.

## Producer obligations

A conforming producer MUST:

1. resolve metadata, the target modfile, and the driver package from one
   effective Go graph;
2. populate selected and replacement provenance without collapsing their
   identities;
3. hash the exact declaration and target-modfile bytes used during discovery;
4. use `Encode`, or emit element-for-element equivalent canonical argv;
5. preserve application argument boundaries and ordering;
6. enforce the transport budget before starting the executable. For Unix bytes,
   `len(executable)+1 + Σ(len(arg)+1) + Σ(len(env)+1) +
   8*(len(args)+len(env)+3)` MUST be at most 128 KiB. For Windows UTF-16 code
   units, `len16(executable)+1 + Σ(2*len16(arg)+3)` MUST be at most 30,000,
   and `1 + Σ(len16(env)+1)` MUST be at most 32,767; and
7. treat any encode or launch failure as an error rather than silently
   changing the request.

The package intentionally does not define a second graph resolver, a driver
lifecycle SDK, or framework-specific policy.

## Consumer obligations

Successful `Parse` is not authentication. Before consuming graph, project, or
framework inputs, a conforming driver MUST:

1. validate that identity-bearing paths still name the expected regular files
   and directories rather than relying on strings alone;
2. re-read and hash the declaration and target modfile, rejecting a digest or
   file-identity change;
3. verify that the driver package and effective source still match
   `DriverOrigin` in the supplied graph;
4. validate framework-owned extension, pack, and action semantics; and
5. reject fields or modes it cannot honor without reinterpreting the request
   through another graph.

These checks constrain a conforming driver and protect against accidental
graph drift. A selected driver is executable code running with the caller's OS
privileges; this protocol is not an access-control sandbox against a malicious
driver.

## Evolution

v1 is a closed schema. Unknown options, incomplete option groups, duplicate
singular options, duplicate canonical flags, and action-inapplicable fields
MUST be rejected. Adding or removing a field, action, or accepted flag, or
changing meaning, cardinality, empty-value rules, digest semantics, or the
accepted invocation frame requires a new protocol version and preamble. Only
an implementation repair that preserves the observable contract may remain
v1.

The deterministic encoding is frozen by the package's golden and round-trip
tests. A protocol-contract change MUST update every affected document, model,
codec, test, and downstream producer or consumer together.

## Minimal examples

A run request ends with the application delimiter:

```text
xgo-driver-v1
run
--project-dir=/work/game
--project-file=/work/game/main.foo
--module-root=/work
--driver-package=example.com/framework/cmd/projectdriver
--selected-path=example.com/framework
--selected-version=v1.2.3
--origin-main=false
--selected-dir=/go/pkg/mod/example.com/framework@v1.2.3
--selected-gomod=/go/pkg/mod/cache/download/example.com/framework/@v/v1.2.3.mod
--project-ext=.foo
--project-full-ext=*.foo
--pack-dir=assets
--pack-index=index.json
--declaration-file=/go/pkg/mod/example.com/framework@v1.2.3/gox.mod
--declaration-sha256=<64 lowercase hexadecimal characters>
--target-modfile=/work/go.mod
--target-modfile-sha256=<64 lowercase hexadecimal characters>
--go-command=/usr/local/go/bin/go
--graph-work-dir=/work
--go-work=off
--graph-flag=-mod=readonly
--build-flag=-buildvcs=false
--
--headless
```

The line breaks above show argv elements; they are not a shell script. A build
request replaces the delimiter and application arguments with `--output=` and
`--final-output=` options.
