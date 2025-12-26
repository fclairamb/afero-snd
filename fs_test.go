package snd

import (
	"errors"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func GetFsConfig(t *testing.T, config *Config) (*Fs, *require.Assertions) {
	t.Helper()

	fileSystem, err := NewFs(config)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := fileSystem.Close(); err != nil {
			t.Fatal(err)
		}
	})

	return fileSystem, require.New(t)
}

func GetFsAuto(t *testing.T) (*Fs, *require.Assertions) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("test", t.Name())
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
	t.Helper()

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
	fileSystem, req := GetFsAuto(t)

	fileSystem.queueSleep(time.Second)

	req.NoError(fileSystem.MkdirAll("/a/b/c", 0755))

	req.NoError(fileSystem.MkDir("/d", 0755))

	writeFile(t, fileSystem, "/a/b/c/test1", "test1")

	{
		_, err := fileSystem.destination.Stat("/a/b/c/test1")
		req.Error(err, "File should not exist")
	}

	req.NoError(fileSystem.Sync())

	fileSystem.queueSleep(time.Second)

	{
		stat, err := fileSystem.destination.Stat("/a/b/c/test1")
		req.NoError(err)
		req.Equal(int64(5), stat.Size(), "Wrong file size")
	}

	req.NoError(fileSystem.Rename("/a/b/c/test1", "/a/b/c/test2"))

	req.NoError(fileSystem.Sync())

	{
		_, err := fileSystem.destination.Stat("/a/b/c/test2")
		req.NoError(err)
	}

	{
		file, err := fileSystem.OpenFile("/a/b/c/test2", os.O_RDONLY, 0755)
		req.NoError(err)

		bufffer := make([]byte, 20)
		n, err := file.Read(bufffer)
		req.NoError(err)
		req.Equal(5, n)

		req.NoError(file.Close())
	}

	req.NoError(fileSystem.Remove("/d"))
	req.NoError(fileSystem.RemoveAll("/a"))
}

func TestFsOps(t *testing.T) {
	fileSystem, req := GetFsAuto(t)

	writeFile(t, fileSystem, "test2", "test2")

	req.NoError(fileSystem.Chmod("test2", 0700))

	if err := fileSystem.Chown("test2", 0, 0); err == nil {
		t.Fatal("We should have an error")
	} else {
		var syserr syscall.Errno
		if !errors.As(err, &syserr) || syserr != syscall.EPERM {
			t.Fatal("This is not the error we expected")
		}
	}

	{
		time := time.Unix(0, 0)
		req.NoError(fileSystem.Chtimes("test2", time, time))
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
	fs, req := GetFsAuto(t)
	req.Equal("snd", fs.Name())
}
