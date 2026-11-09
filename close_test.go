package snd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestCloseRunsQueuedOperations(t *testing.T) {
	req := require.New(t)
	dst := afero.NewMemMapFs()

	sndFs, err := NewFs(&Config{Destination: dst, Temporary: afero.NewMemMapFs()})
	req.NoError(err)

	// The copy is queued behind a slow operation, so it's still pending when Close is called
	sndFs.queueSleep(200 * time.Millisecond)
	writeFile(t, sndFs, "/file", "content")

	req.NoError(sndFs.Close())

	content, err := afero.ReadFile(dst, "/file")
	req.NoError(err)
	req.Equal("content", string(content))
}

func TestCloseTwice(t *testing.T) {
	req := require.New(t)

	sndFs, err := NewFs(&Config{Destination: afero.NewMemMapFs(), Temporary: afero.NewMemMapFs()})
	req.NoError(err)

	req.NoError(sndFs.Close())

	done := make(chan error)

	go func() { done <- sndFs.Close() }()

	select {
	case err := <-done:
		req.NoError(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Second Close blocked")
	}
}

func TestOperationsAfterClose(t *testing.T) {
	req := require.New(t)

	sndFs, err := NewFs(&Config{Destination: afero.NewMemMapFs(), Temporary: afero.NewMemMapFs()})
	req.NoError(err)
	req.NoError(sndFs.Close())

	req.ErrorIs(sndFs.Sync(), ErrClosed)
	req.ErrorIs(sndFs.MkdirAll("/a", 0750), ErrClosed)

	file, err := sndFs.OpenFile("/b", os.O_CREATE|os.O_WRONLY, 0600)
	req.NoError(err)
	req.ErrorIs(file.Close(), ErrClosed)
}

func TestCloseContextTimeout(t *testing.T) {
	req := require.New(t)

	sndFs, err := NewFs(&Config{Destination: afero.NewMemMapFs()})
	req.NoError(err)

	tempDir := sndFs.ownedTempDir

	sndFs.queueSleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = sndFs.CloseContext(ctx)
	req.ErrorIs(err, context.DeadlineExceeded)

	// The queue keeps draining in the background, then the directory is removed
	req.NoError(sndFs.Close())
	_, err = os.Stat(tempDir)
	req.True(os.IsNotExist(err), "temporary directory should be removed: %v", err)
}

func TestCloseRemovesOwnedTempDir(t *testing.T) {
	req := require.New(t)

	sndFs, err := NewFs(&Config{Destination: afero.NewMemMapFs()})
	req.NoError(err)

	tempDir := sndFs.ownedTempDir
	req.NotEmpty(tempDir)
	writeFile(t, sndFs, "/file", "content")

	_, err = os.Stat(filepath.Join(tempDir, "file"))
	req.NoError(err)

	req.NoError(sndFs.Close())

	_, err = os.Stat(tempDir)
	req.True(os.IsNotExist(err), "temporary directory should be removed: %v", err)
}

func TestCloseKeepsOperatorTempDir(t *testing.T) {
	req := require.New(t)
	tempDir := t.TempDir()

	sndFs, err := NewFs(&Config{
		Destination: afero.NewMemMapFs(),
		Temporary:   afero.NewBasePathFs(afero.NewOsFs(), tempDir),
	})
	req.NoError(err)
	req.Empty(sndFs.ownedTempDir)

	writeFile(t, sndFs, "/file", "content")
	req.NoError(sndFs.Close())

	_, err = os.Stat(filepath.Join(tempDir, "file"))
	req.NoError(err, "operator directory and its files should be kept")
}
