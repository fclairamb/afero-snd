package snd

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/fclairamb/go-log/gokit"
	"github.com/spf13/afero"
	tassert "github.com/stretchr/testify/assert"
)

func GetFsConfig(t *testing.T, config *Config) (*Fs, *tassert.Assertions) {
	fs, err := NewFs(config)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := fs.Close(); err != nil {
			t.Fatal(err)
		}
	})

	return fs, tassert.New(t)
}

func GetFsAuto(t *testing.T) (*Fs, *tassert.Assertions) {
	logger := gokit.New().With("test", t.Name()).With("caller", gokit.GKDefaultCaller)
	tempDirPath, _ := os.MkdirTemp("", t.Name())

	if err := os.MkdirAll(tempDirPath, 0750); err != nil {
		panic(err)
	}

	tempFs := afero.NewBasePathFs(afero.NewOsFs(), tempDirPath)

	t.Cleanup(func() {
		if err := os.RemoveAll(tempDirPath); err != nil {
			t.Fatal(err)
		}
	})

	if err := tempFs.MkdirAll("dst", 0750); err != nil {
		t.Fatal(err)
	}

	if err := tempFs.MkdirAll("temp", 0750); err != nil {
		t.Fatal(err)
	}

	dst := afero.NewBasePathFs(tempFs, "dst")
	temp := afero.NewBasePathFs(tempFs, "temp")

	conf := &Config{
		Destination: dst,
		Temporary:   temp,
		Logger:      logger,
		Behavior:    &Behavior{},
	}

	logger.Info("Temporary path available", "tempDirPath", tempDirPath)

	return GetFsConfig(t, conf)
}

func (fs *Fs) queueSleep(dur time.Duration) {
	fs.queueOperation(func() error {
		time.Sleep(dur)

		return nil
	})
}

func writeFile(t *testing.T, fs *Fs, name string, content string) { // First we write a file
	file, err := fs.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFsCreateReadDelete(t *testing.T) {
	fs, assert := GetFsAuto(t)
	fs.queueSleep(time.Second)

	assert.NoError(fs.MkdirAll("/a/b/c", 0755))

	assert.NoError(fs.MkDir("/d", 0755))

	writeFile(t, fs, "/a/b/c/test1", "test1")

	{
		_, err := fs.destination.Stat("/a/b/c/test1")
		assert.Error(err, "File should not exist")
	}

	assert.NoError(fs.Sync())

	fs.queueSleep(time.Second)

	{
		stat, err := fs.destination.Stat("/a/b/c/test1")
		assert.NoError(err)
		assert.Equal(int64(5), stat.Size(), "Wrong file size")
	}

	assert.NoError(fs.Rename("/a/b/c/test1", "/a/b/c/test2"))

	assert.NoError(fs.Sync())

	{
		_, err := fs.destination.Stat("/a/b/c/test2")
		assert.NoError(err)
	}

	{
		file, err := fs.OpenFile("/a/b/c/test2", os.O_RDONLY, 0755)
		assert.NoError(err)

		bufffer := make([]byte, 20)
		n, err := file.Read(bufffer)
		assert.NoError(err)
		assert.Equal(5, n)

		assert.NoError(file.Close())
	}

	assert.NoError(fs.Remove("/d"))
	assert.NoError(fs.RemoveAll("/a"))
}

func TestFsOps(t *testing.T) {
	fs, assert := GetFsAuto(t)

	writeFile(t, fs, "test2", "test2")

	assert.NoError(fs.Chmod("test2", 0700))

	if err := fs.Chown("test2", 0, 0); err == nil {
		t.Fatal("We should have an error")
	} else {
		var syserr syscall.Errno
		if !errors.As(err, &syserr) || syserr != syscall.EPERM {
			t.Fatal("This is not the error we expected")
		}
	}

	{
		time := time.Unix(0, 0)
		assert.NoError(fs.Chtimes("test2", time, time))
	}
}

func TestFsInit(t *testing.T) {
	GetFsConfig(t, &Config{
		Destination: afero.NewMemMapFs(),
	})

	GetFsConfig(t, &Config{
		Destination: afero.NewMemMapFs(),
		Behavior: &Behavior{
			FileAgeMin: time.Millisecond,
		},
	})

	if _, err := NewFs(&Config{}); !errors.Is(err, ErrNoDestination) {
		t.Fatal("Error should be ErrNoDestination", err)
	}
}

func TestFsProperties(t *testing.T) {
	fs, assert := GetFsAuto(t)
	assert.Equal("snd", fs.Name())
}
