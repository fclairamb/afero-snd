package snd

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestGc(t *testing.T) {
	fileNbMin := 3

	fileSystem, assert := GetFsConfig(t, &Config{
		Destination: afero.NewMemMapFs(),
		Behavior: &Behavior{
			CleanupPeriod: time.Millisecond,
			FileAgeMin:    time.Millisecond * 200,
			FileNbMin:     &fileNbMin,
		},
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)).With("test", t.Name()),
	})

	log := fileSystem.log

	assert.NoError(fileSystem.MkdirAll("/a/b", 0750))

	log.Info("Writing file1")
	writeFile(t, fileSystem, "/a/b/file1", "file1")

	time.Sleep(time.Millisecond * 150)

	log.Info("Writing file2")
	writeFile(t, fileSystem, "/a/b/file2", "file2")

	time.Sleep(time.Millisecond * 150)

	{
		log.Info("Checking for file1")

		_, err := fileSystem.Stat("/a/b/file1")
		assert.Error(err)
	}

	{
		log.Info("Checking for file2")

		_, err := fileSystem.Stat("/a/b/file2")
		assert.NoError(err)
	}
}
