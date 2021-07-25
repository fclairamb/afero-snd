package snd

import (
	"errors"
	"io"
	"path"
	"time"

	"github.com/fclairamb/go-log"
	"github.com/spf13/afero"
)

var (
	// Checking Fs conformance
	_ afero.Fs = (*Fs)(nil)
)

// nolint:govet
type garbageCollector struct {
	fs              afero.Fs
	behavior        *Behavior
	nbFilesKept     int
	nbFilesDeleted  int
	fileMinimumDate time.Time
	log             log.Logger
}

func (gc *garbageCollector) run() error {
	gc.nbFilesKept = 0
	gc.nbFilesDeleted = 0
	gc.fileMinimumDate = time.Now().Add(-gc.behavior.FileAgeMin)

	gc.log.Info("Starting garbage collection")
	_, err := gc.explore("")
	gc.log.Info("Finished garbage collection", "nbFilesKept", gc.nbFilesKept, "nbFilesDeleted", gc.nbFilesDeleted)

	return err
}

func (gc *garbageCollector) explore(dirPath string) (int, error) {
	file, err := gc.fs.Open(dirPath)
	if err != nil {
		return 0, wrapFsError(err)
	}

	files, err := file.Readdir(1000)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil
		}

		return 0, wrapFsError(err)
	}

	for _, file := range files {
		subPath := path.Join(dirPath, file.Name())
		if file.IsDir() {
			nbSubs, err := gc.explore(subPath)
			if err != nil {
				return 0, err
			}

			// Non-empty dirs are always kept
			if nbSubs > 0 {
				gc.nbFilesKept++

				continue
			}
		}

		if file.ModTime().After(gc.fileMinimumDate) {
			if gc.behavior.FileNbMin != nil && gc.nbFilesKept < *gc.behavior.FileNbMin {
				gc.nbFilesKept++

				continue
			}
		}

		gc.log.Info("Deleting file", "path", subPath)

		if err := gc.fs.Remove(subPath); err != nil {
			gc.log.Error("Couldn't delete file", "path", subPath)
		}

		gc.nbFilesDeleted++
	}

	return len(files), nil
}
