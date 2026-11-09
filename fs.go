// Package snd is an afero sync & delete file system
package snd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// Fs is the SND file system
type Fs struct {
	afero.Fs
	destination  afero.Fs
	behavior     *Behavior
	operations   chan *operation
	cleanupTimer *time.Ticker
	gc           *garbageCollector
	log          *slog.Logger
	ownedTempDir string        // Temporary directory created by NewFs, removed on Close
	mu           sync.RWMutex  // Protects closed against the queuing of operations
	closed       bool          // No more operations can be queued
	closeOnce    sync.Once     // Close starts the shutdown only once
	stop         chan struct{} // Closed when no more operations can be queued
	done         chan struct{} // Closed once the queued operations have run
}

// Behavior defines the GC logic
type Behavior struct {
	FileNbMin           *int          // Minimum number of files to keep
	FileAgeMin          time.Duration // Minimum age of a file to be deleted
	OperationsQueueSize int           // Queue of operations to perform
	CleanupPeriod       time.Duration // Period of time between cleanups
	CloseTimeout        time.Duration // Maximum time Close waits for the queued operations (default: 1 minute)
}

// Config defines the SND file system configuration
type Config struct {
	Destination afero.Fs
	Temporary   afero.Fs
	Behavior    *Behavior
	Logger      *slog.Logger
}

type operation struct {
	fn       process
	queuedAt time.Time
}

// ErrNoDestination is returned when the destination is nil
var ErrNoDestination = errors.New("destination FS needs to be specified")

// ErrClosed is returned when an operation can't be synced because the file system is closed
var ErrClosed = errors.New("snd file system is closed")

type process = func() error

func wrapFsError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("temporary FS had an issue: %w", err)
}

// NewFs instantiates a new file system
func NewFs(config *Config) (*Fs, error) {
	if config.Destination == nil {
		return nil, ErrNoDestination
	}

	var ownedTempDir string

	if config.Temporary == nil {
		tempDir, err := os.MkdirTemp("", "afero-snd")

		if err != nil {
			return nil, wrapFsError(err)
		}

		if errMkDir := os.MkdirAll(tempDir, 0750); errMkDir != nil {
			return nil, wrapFsError(errMkDir)
		}

		config.Temporary = afero.NewBasePathFs(afero.NewOsFs(), tempDir)
		ownedTempDir = tempDir
	}

	if config.Behavior == nil {
		config.Behavior = &Behavior{}
	}

	beh := config.Behavior

	if beh.OperationsQueueSize == 0 {
		beh.OperationsQueueSize = 1000
	}

	if beh.FileNbMin == nil {
		v := 10
		beh.FileNbMin = &v
	}

	if beh.FileAgeMin == 0 {
		// We keep files 20 minutes by default
		beh.FileAgeMin = time.Minute * 20
	}

	if beh.CleanupPeriod == 0 {
		t := beh.FileAgeMin / 2

		beh.CleanupPeriod = t
	}

	{
		minPeriod := time.Millisecond * 100

		if beh.CleanupPeriod < minPeriod {
			beh.CleanupPeriod = minPeriod
		}
	}

	if beh.CloseTimeout == 0 {
		beh.CloseTimeout = time.Minute
	}

	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	sndFs := &Fs{
		Fs:           config.Temporary,
		destination:  config.Destination,
		behavior:     config.Behavior,
		log:          config.Logger,
		operations:   make(chan *operation, config.Behavior.OperationsQueueSize),
		gc:           &garbageCollector{fs: config.Temporary, behavior: beh, log: config.Logger.With("component", "gc")},
		cleanupTimer: time.NewTicker(beh.CleanupPeriod),
		ownedTempDir: ownedTempDir,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}

	sndFs.start()

	return sndFs, nil
}

func (fs *Fs) runOperation(op *operation) {
	if err := op.fn(); err != nil {
		fs.log.Error("Couldn't run operation", "err", err)
	}
}

func (fs *Fs) processOperations() {
	defer close(fs.done)
	defer fs.cleanup()

	for {
		select {
		case op := <-fs.operations:
			fs.runOperation(op)

		case <-fs.cleanupTimer.C:
			if len(fs.operations) == 0 {
				if err := fs.gc.run(); err != nil {
					fs.log.Error("Couldn't run garbage collection", "err", err)
				}
			}

		case <-fs.stop:
			// No operation can be queued anymore: we run the ones that are still waiting
			for {
				select {
				case op := <-fs.operations:
					fs.runOperation(op)
				default:
					return
				}
			}
		}
	}
}

func (fs *Fs) cleanup() {
	fs.cleanupTimer.Stop()

	if fs.ownedTempDir != "" {
		if err := os.RemoveAll(fs.ownedTempDir); err != nil {
			fs.log.Error("Couldn't remove temporary directory", "dir", fs.ownedTempDir, "err", err)
		}
	}
}

// Name returns the name of the file system
func (fs *Fs) Name() string {
	return "snd"
}

func (fs *Fs) start() {
	go fs.processOperations()
}

// Close the file system. It waits for the operations queued before it to run, at most
// Behavior.CloseTimeout. It can be called more than once.
func (fs *Fs) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), fs.behavior.CloseTimeout)
	defer cancel()

	return fs.CloseContext(ctx)
}

// CloseContext closes the file system. It waits for the operations queued before it to run,
// until the context is done. The operations still queued then keep running in the background.
// The temporary directory created by NewFs (never one given in the Config) is removed once they
// have all run.
func (fs *Fs) CloseContext(ctx context.Context) error {
	fs.closeOnce.Do(func() {
		// Taking the lock waits for the operations being queued, which can take time if the
		// queue is full: that's why it's done in the background.
		go func() {
			fs.mu.Lock()
			fs.closed = true
			fs.mu.Unlock()
			close(fs.stop)
		}()
	})

	select {
	case <-fs.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for the queued operations: %w", ctx.Err())
	}
}

func (fs *Fs) queueOperation(opFn process) error {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if fs.closed {
		return ErrClosed
	}

	fs.operations <- &operation{
		fn:       opFn,
		queuedAt: time.Now(),
	}

	return nil
}

// OpenFile opens a file using the given flags and the given mode.
func (fs *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&os.O_WRONLY != 0 {
		file := &File{
			parent: fs,
			name:   name,
			flag:   flag,
			perm:   perm,
			log:    fs.log.With("fileName", name),
		}

		var err error
		file.File, err = fs.Fs.OpenFile(file.name, file.flag, file.perm)

		return file, wrapFsError(err)
	}

	f, err := fs.Fs.OpenFile(name, flag, perm)

	return f, wrapFsError(err)
}

// localThenQueue returns the error of the local operation, or the one of the queuing of the
// destination operation.
func localThenQueue(errLocal, errQueue error) error {
	if errLocal != nil {
		return wrapFsError(errLocal)
	}

	return errQueue
}

// MkDir creates a new directory with the specified name and permission bits.
func (fs *Fs) MkDir(name string, perm os.FileMode) error {
	errQueue := fs.queueOperation(func() error { return wrapFsError(fs.destination.Mkdir(name, perm)) })

	return localThenQueue(fs.Fs.Mkdir(name, perm), errQueue)
}

// MkdirAll creates a directory andall its parents
func (fs *Fs) MkdirAll(path string, perm os.FileMode) error {
	errQueue := fs.queueOperation(func() error { return wrapFsError(fs.destination.MkdirAll(path, perm)) })

	return localThenQueue(fs.Fs.MkdirAll(path, perm), errQueue)
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func (fs *Fs) Remove(name string) error {
	err := fs.Fs.Remove(name)
	if err != nil {
		return wrapFsError(err)
	}

	return fs.queueOperation(func() error { return wrapFsError(fs.destination.Remove(name)) })
}

// RemoveAll removes a path and any children it contains.
func (fs *Fs) RemoveAll(path string) error {
	err := fs.Fs.RemoveAll(path)
	if err != nil {
		return wrapFsError(err)
	}

	return fs.queueOperation(func() error { return wrapFsError(fs.destination.RemoveAll(path)) })
}

// Rename renames (moves) a file.
func (fs *Fs) Rename(oldName, newName string) error {
	err := fs.Fs.Rename(oldName, newName)
	if err != nil {
		return wrapFsError(err)
	}

	return fs.queueOperation(func() error { return wrapFsError(fs.destination.Rename(oldName, newName)) })
}

// Chmod changes the mode of the named file to mode.
func (fs *Fs) Chmod(name string, mode os.FileMode) error {
	errQueue := fs.queueOperation(func() error { return wrapFsError(fs.destination.Chmod(name, mode)) })

	return localThenQueue(fs.Fs.Chmod(name, mode), errQueue)
}

// Chown changes the numeric uid and gid of the named file.
func (fs *Fs) Chown(name string, uid, gid int) error {
	errQueue := fs.queueOperation(func() error { return wrapFsError(fs.destination.Chown(name, uid, gid)) })

	return localThenQueue(fs.Fs.Chown(name, uid, gid), errQueue)
}

// Chtimes changes the access and modification times of the named
func (fs *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	errQueue := fs.queueOperation(func() error { return wrapFsError(fs.destination.Chtimes(name, atime, mtime)) })

	return localThenQueue(fs.Fs.Chtimes(name, atime, mtime), errQueue)
}

// Sync syncs the file system, i.e. makes sure all the waiting operations
// have been done.
func (fs *Fs) Sync() error {
	wait := make(chan error, 1)

	if err := fs.queueOperation(func() error {
		wait <- nil

		return nil
	}); err != nil {
		return err
	}

	return <-wait
}
