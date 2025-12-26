# Afero Sync & Delete fs

[![Go Reference](https://pkg.go.dev/badge/github.com/fclairamb/afero-snd.svg)](https://pkg.go.dev/github.com/fclairamb/afero-snd)
[![Go Report Card](https://goreportcard.com/badge/github.com/fclairamb/afero-snd)](https://goreportcard.com/report/github.com/fclairamb/afero-snd)

The S&D file system relies on:
- A temporary file system to store files as fast as possible
- A permanent file system for the destination files

It allows to quickly accept new files and synchronize them on a slower file system.

The rationale is that when using some cloud-based file systems like [S3](https://github.com/fclairamb/afero-s3), [Google Drive](https://github.com/fclairamb/afero-gdrive) or [Dropbox](https://github.com/fclairamb/afero-dropbox) on an [FTP server](https://github.com/fclairamb/ftpserver), some devices consider the slowness as an error.

## Requirements

- Go 1.24 or later

## Features

- Asynchronous file synchronization from temporary to destination filesystem
- Automatic garbage collection of old files
- Built-in logging using Go's standard `log/slog` package
- Configurable cleanup behavior (file age, minimum file count, cleanup period)

## Installation

```bash
go get github.com/fclairamb/afero-snd
```

## Usage

```go
package main

import (
    "log/slog"
    "os"

    "github.com/fclairamb/afero-snd"
    "github.com/spf13/afero"
)

func main() {
    // Create a new sync & delete filesystem
    fs, err := snd.NewFs(&snd.Config{
        Destination: afero.NewOsFs(),
        Temporary:   afero.NewMemMapFs(),
        Logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
    })
    if err != nil {
        panic(err)
    }
    defer fs.Close()

    // Use the filesystem
    file, err := fs.Create("test.txt")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    // Write to file - changes are queued for sync
    file.WriteString("Hello, World!")
}
```

