package snd

import (
	"testing"
	"time"

	"github.com/fclairamb/go-log/gokit"
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
		Logger: gokit.New().
			With("test", t.Name()).
			With("caller", gokit.GKDefaultCaller).
			With("time", gokit.GKDefaultTimestampUTC),
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
