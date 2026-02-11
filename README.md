# navi

`navi` is like `zoxide`, but for all directories/files with multi-keyword matching to deal with ambiguous search result.

It uses gap-filling fuzzy search, so each keyword can be incomplete.

Example:

```bash
navi proj nav en
```

This can still match something like `projects/navi/search/engine.go` even when each token is only partial.

Interactive mode (TUI):

```bash
navi
```

## Requirements

- Go 1.24+
- SQLite support (used via `github.com/mattn/go-sqlite3`)

## Install

Build locally:

```bash
go build -o navi .
```

Or install with Go:

```bash
go install github.com/montrey/navi@latest
```

## Basic usage

Run interactive mode in your current directory:

```bash
navi
```

Type to fuzzy-search paths, then press:

- `Enter` to run the selected action
- `Ctrl+C` to quit
- `Tab` / `Shift+Tab` to cycle action (`explorer`, `terminal`, `editor`, `copy`)

Multi-keyword search does not require full words. You can type partial chunks (for example `doc rea md`) and still get the intended result.

## Helpful shortcuts

- `Ctrl+O` open config
- `Ctrl+T` open tag UI for the selected/current directory
- `Ctrl+D` drill into selected directory

Note: the tag system is still in progress and may change.

Inside config:

- `Enter` edit/save a field
- `Left/Right` cycle default action
- `A` add custom action
- `D` delete selected custom action

## CLI examples

Print best match path and exit:

```bash
navi "src main"
```

Start interactive mode with a specific action:

```bash
navi --action editor
```

Add current directory to a tag:

```bash
navi --add work
```

Search with tag scope in interactive mode:

```text
@work query
```

Note: tag search and workflows are currently in progress.

## Data location

`navi` stores history, tags, and settings in:

```text
~/.local/share/navi/navi.db
```
