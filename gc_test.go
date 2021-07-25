package snd

import (
	"testing"
	"time"

	"github.com/fclairamb/go-log/gokit"
	"github.com/spf13/afero"
)

func TestGc(t *testing.T) {
	v := 3

	fs, assert := GetFsConfig(t, &Config{
		Destination: afero.NewMemMapFs(),
		Behavior: &Behavior{
			CleanupPeriod: time.Millisecond,
			FileAgeMin:    time.Millisecond * 200,
			FileNbMin:     &v,
		},
		Logger: gokit.NewGKLoggerStdout().
			With("test", t.Name()).
			With("caller", gokit.GKDefaultCaller).
			With("time", gokit.GKDefaultTimestampUTC),
	})

	log := fs.log

	assert.NoError(fs.MkdirAll("/a/b", 0750))

	log.Info("Writing file1")
	writeFile(t, fs, "/a/b/file1", "file1")

	time.Sleep(time.Millisecond * 150)

	log.Info("Writing file2")
	writeFile(t, fs, "/a/b/file2", "file2")

	time.Sleep(time.Millisecond * 150)

	{
		log.Info("Checking for file1")
		_, err := fs.Stat("/a/b/file1")
		assert.Error(err)
	}

	{
		log.Info("Checking for file2")
		_, err := fs.Stat("/a/b/file2")
		assert.NoError(err)
	}

	{
		v := 0
		fs.behavior.FileNbMin = &v
	}
	time.Sleep(time.Millisecond * 500)
}
