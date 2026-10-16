mod - Module support for Go/XGo
=====

[![Build Status](https://github.com/goplus/mod/actions/workflows/go.yml/badge.svg)](https://github.com/goplus/mod/actions/workflows/go.yml)
[![GitHub release](https://img.shields.io/github/v/tag/goplus/mod.svg?label=release)](https://github.com/goplus/mod/releases)
[![Coverage Status](https://codecov.io/gh/goplus/mod/branch/main/graph/badge.svg)](https://codecov.io/gh/goplus/mod)
[![GoDoc](https://pkg.go.dev/badge/github.com/goplus/mod.svg)](https://pkg.go.dev/github.com/goplus/mod)
[![XGo](https://img.shields.io/badge/project-XGo-blue.svg)](https://github.com/goplus/xgo)

This repository holds packages for writing tools that work directly with Go/XGo module mechanics. That is, it is for direct manipulation of Go/XGo modules themselves.

## Project drivers

Framework metadata may attach a driver to the nearest preceding project:

```text
project main.spx Game example.com/framework math
driver v1 example.com/framework/cmd/xgodriver
```

The protocol must match `v[1-9][0-9]*`; `driver` is a single-line, non-duplicate directive and has no block form. [`driverprotocol/spec-v1.md`](driverprotocol/spec-v1.md) is the canonical v1 wire and provenance contract.

The package defines the request model and argv codec, but structural validation does not authenticate files. Consumers must reject non-canonical or symlinked declaration paths and re-hash declaration bytes before trusting the supplied SHA-256 identity. `xgomod.ImportClassesResolved` validates the target snapshot and ordered class-module provenance without rediscovering the graph.
