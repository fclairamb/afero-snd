package snd

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/afero"
)

// File is the internal representation of a file
type File struct {
	afero.File              // Underlying temporary file
	log        *slog.Logger // Logger
	parent     *Fs          // Parent filesystem¬
	name       string       // Name of the file
	flag       int          // Flag for opening file
	perm       os.FileMode  // Perm for opening file
}

// Close the file
func (f *File) Close() error {
	if err := f.File.Close(); err != nil {
		return wrapFsError(err)
	}

	if f.flag&(os.O_APPEND|os.O_WRONLY|os.O_RDWR) != 0 {
		f.parent.queueOperation(f.copyFile)
	}

	return nil
}

// Sync triggers a sync call on the local file system and copy on the distant one
func (f *File) Sync() error {
	if err := f.File.Sync(); err != nil {
		return wrapFsError(err)
	}
	defer f.parent.queueOperation(f.copyFile)

	return nil
}

// Truncate truncates the file to size bytes
func (f *File) Truncate(size int64) error {
	f.parent.queueOperation(f.copyFile)

	return wrapFsError(f.File.Truncate(size))
}

/*
func (f *File) WriteString(s string) (int, error) {
	defer f.parent.queueOperation(f.copyFile)
	return f.File.WriteString(s)
}
*/

func (f *File) copyFile() error {
	f.log.Debug("Copying file")
	defer f.log.Debug("Copy done")
	dst, errDstOpen := f.parent.destination.OpenFile(f.name, f.flag, f.perm)

	if errDstOpen != nil {
		f.log.Error(
			"Couldn't open copy destination file",
			"fileName", f.name,
			"fileFlag", f.flag,
			"filePerm", f.perm,
			"err", errDstOpen,
		)

		return wrapFsError(errDstOpen)
	}

	defer func() {
		if err := dst.Close(); err != nil {
			f.log.Error("Error closing destination file", "err", err)
		}
	}()

	src, errSrcOpen := f.parent.Fs.OpenFile(f.name, os.O_RDONLY, f.perm)
	if errSrcOpen != nil {
		return wrapFsError(errSrcOpen)
	}

	defer func() {
		if err := src.Close(); err != nil {
			f.log.Error("Error closing source file", "err", err)
		}
	}()

	if _, errCopy := io.Copy(dst, src); errCopy != nil {
		f.log.Error("Error copying file", "err", errCopy)

		return wrapFsError(errCopy)
	}

	return nil
}
