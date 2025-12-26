# Claude Code Context for afero-snd

This file provides context for AI assistants (like Claude Code) working on this project.

## Project Overview

**afero-snd** is a Sync & Delete filesystem implementation built on top of the [Afero](https://github.com/spf13/afero) filesystem abstraction library. It provides asynchronous file synchronization between a fast temporary filesystem and a slower destination filesystem, with automatic garbage collection.

## Architecture

### Core Components

1. **Fs (fs.go)**: Main filesystem implementation
   - Manages dual filesystems (temporary + destination)
   - Asynchronous operation queue for syncing changes
   - Periodic garbage collection
   - Uses Go's standard `log/slog` for logging

2. **File (file.go)**: File wrapper
   - Wraps temporary file operations
   - Queues copy operations to destination on write/close
   - Logs file operations using slog

3. **Garbage Collector (gc.go)**: Cleanup logic
   - Removes old files from temporary filesystem
   - Configurable retention policies (age, count)
   - Recursive directory traversal

### Key Patterns

- **Embedded filesystem**: `Fs` embeds `afero.Fs` for delegation
- **Operation queue**: Asynchronous channel-based task processing
- **Structured logging**: Uses `*slog.Logger` throughout
- **Error wrapping**: Consistent error wrapping with context

## Development

### Build & Test Commands

```bash
# Build
go build ./...

# Run tests
go test -v ./...

# Run tests with race detection and coverage
go test -parallel 20 -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run linter
golangci-lint run

# Update dependencies
go get -u ./...
go mod tidy
```

### CI/CD

- **Platform**: GitHub Actions (ubuntu-24.04)
- **Go versions tested**: 1.25 (with linting), 1.24
- **Linter**: golangci-lint v2.7.2+ (v1.63.4 in CI)
- **Coverage**: Codecov integration

### Project Structure

```
.
├── file.go          # File operations wrapper
├── fs.go            # Main filesystem implementation
├── gc.go            # Garbage collector
├── file_test.go     # File operation tests
├── fs_test.go       # Filesystem tests
├── gc_test.go       # Garbage collection tests
├── go.mod           # Go module definition
├── .golangci.yml    # Linter configuration
└── .github/
    └── workflows/
        └── build.yml  # CI workflow
```

## Dependencies

### Direct Dependencies

- `github.com/spf13/afero` - Filesystem abstraction library
- `github.com/stretchr/testify` - Testing utilities

### Standard Library Usage

- `log/slog` - Structured logging (replaced `github.com/fclairamb/go-log` in recent upgrade)
- `io`, `os`, `time`, `errors`, `fmt` - Standard utilities

## Code Conventions

### Logging

- All components use `*slog.Logger` for structured logging
- Logger is passed via `Config` struct
- Default logger discards all logs if not provided
- Use `.With()` for contextual loggers (e.g., component name, file name)

### Error Handling

- Wrap filesystem errors with `wrapFsError()` for context
- Use standard `fmt.Errorf()` with `%w` for error chains
- Return errors; don't panic (except in tests)

### Testing

- Use `testify/require` for assertions
- Helper function `GetFsAuto()` creates test filesystem
- Tests clean up resources using `t.Cleanup()`
- Test both in-memory and OS-backed filesystems

## Recent Changes (Go 1.24 Upgrade)

1. **Logging migration**: Replaced `github.com/fclairamb/go-log` with standard `log/slog`
   - Changed `log.Logger` → `*slog.Logger`
   - Removed dependency on external logging library
   - Updated test loggers from `gokit.New()` to `slog.New()`

2. **Go version**: Upgraded from 1.23 to 1.24
   - Updated CI to test Go 1.25 and 1.24
   - Updated all dependencies to latest versions

3. **Linter updates**: Upgraded to golangci-lint v2.x
   - Updated `.golangci.yml` to version 2 format
   - Removed deprecated linters (typecheck, megacheck, etc.)
   - Added formatters section for gci, gofmt, goimports

## Configuration Options

The `Config` struct accepts:

- `Destination` (required): Target filesystem for synchronized files
- `Temporary`: Fast temporary filesystem (defaults to OS temp dir)
- `Logger`: slog.Logger instance (defaults to discard handler)
- `Behavior`: Cleanup and sync behavior settings
  - `FileNbMin`: Minimum files to keep (default: 10)
  - `FileAgeMin`: Minimum file age before deletion (default: 20 minutes)
  - `OperationsQueueSize`: Sync queue size (default: 1000)
  - `CleanupPeriod`: GC interval (default: half of FileAgeMin)

## Common Tasks for AI Assistants

### Adding a new filesystem operation

1. Add method to `Fs` struct in `fs.go`
2. Implement operation on temporary filesystem
3. Queue corresponding operation for destination using `queueOperation()`
4. Add error wrapping with `wrapFsError()`
5. Add tests in `fs_test.go`

### Debugging logging issues

- Check that `Logger` is passed in `Config`
- Use `.With()` for contextual information
- Log levels: Debug, Info, Warn, Error (no Panic in slog)
- Test logger: `slog.New(slog.NewTextHandler(os.Stdout, nil))`

### Running CI locally

```bash
# Lint
golangci-lint run

# Test with race detection
go test -race ./...

# Check Go version compatibility
go build -v ./...
```

## Related Projects

- [afero](https://github.com/spf13/afero) - Filesystem abstraction
- [ftpserver](https://github.com/fclairamb/ftpserver) - FTP server using afero
- [afero-s3](https://github.com/fclairamb/afero-s3) - S3 backend for afero
- [afero-gdrive](https://github.com/fclairamb/afero-gdrive) - Google Drive backend
- [afero-dropbox](https://github.com/fclairamb/afero-dropbox) - Dropbox backend
