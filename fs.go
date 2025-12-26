// Package snd is an afero sync & delete file system
package snd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
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
}

// Behavior defines the GC logic
type Behavior struct {
	FileNbMin           *int          // Minimum number of files to keep
	FileAgeMin          time.Duration // Minimum age of a file to be deleted
	OperationsQueueSize int           // Queue of operations to perform
	CleanupPeriod       time.Duration // Period of time between cleanups
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

	if config.Temporary == nil {
		var tempDir string
		tempDir, err := os.MkdirTemp("", "afero-snd")

		if err != nil {
			return nil, wrapFsError(err)
		}

		if errMkDir := os.MkdirAll(tempDir, 0750); errMkDir != nil {
			return nil, wrapFsError(errMkDir)
		}

		config.Temporary = afero.NewBasePathFs(afero.NewOsFs(), tempDir)
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
	}

	sndFs.start()

	return sndFs, nil
}

func (fs *Fs) processOperations() {
	for {
		select {
		case op := <-fs.operations:
			if op == nil {
				return
			}

			if err := op.fn(); err != nil {
				fs.log.Error("Couldn't run operation", "err", err)
			}

		case <-fs.cleanupTimer.C:
			if len(fs.operations) == 0 {
				fs.queueOperation(func() error { return fs.gc.run() })
			}
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

// Close the file system
func (fs *Fs) Close() error {
	fs.cleanupTimer.Stop()
	fs.operations <- nil

	return nil
}

func (fs *Fs) queueOperation(fn process) {
	fs.operations <- &operation{
		fn:       fn,
		queuedAt: time.Now(),
	}
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

// MkDir creates a new directory with the specified name and permission bits.
func (fs *Fs) MkDir(name string, perm os.FileMode) error {
	fs.queueOperation(func() error { return wrapFsError(fs.destination.Mkdir(name, perm)) })

	return wrapFsError(fs.Fs.Mkdir(name, perm))
}

// MkdirAll creates a directory andall its parents
func (fs *Fs) MkdirAll(path string, perm os.FileMode) error {
	fs.queueOperation(func() error { return wrapFsError(fs.destination.MkdirAll(path, perm)) })

	return wrapFsError(fs.Fs.MkdirAll(path, perm))
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func (fs *Fs) Remove(name string) error {
	err := fs.Fs.Remove(name)
	if err == nil {
		fs.queueOperation(func() error { return wrapFsError(fs.destination.Remove(name)) })
	}

	return wrapFsError(err)
}

// RemoveAll removes a path and any children it contains.
func (fs *Fs) RemoveAll(path string) error {
	err := fs.Fs.RemoveAll(path)
	if err == nil {
		fs.queueOperation(func() error { return wrapFsError(fs.destination.RemoveAll(path)) })
	}

	return wrapFsError(err)
}

// Rename renames (moves) a file.
func (fs *Fs) Rename(oldName, newName string) error {
	err := fs.Fs.Rename(oldName, newName)
	if err == nil {
		fs.queueOperation(func() error { return wrapFsError(fs.destination.Rename(oldName, newName)) })
	}

	return wrapFsError(err)
}

// Chmod changes the mode of the named file to mode.
func (fs *Fs) Chmod(name string, mode os.FileMode) error {
	fs.queueOperation(func() error { return wrapFsError(fs.destination.Chmod(name, mode)) })

	return wrapFsError(fs.Fs.Chmod(name, mode))
}

// Chown changes the numeric uid and gid of the named file.
func (fs *Fs) Chown(name string, uid, gid int) error {
	fs.queueOperation(func() error { return wrapFsError(fs.destination.Chown(name, uid, gid)) })

	return wrapFsError(fs.Fs.Chown(name, uid, gid))
}

// Chtimes changes the access and modification times of the named
func (fs *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	fs.queueOperation(func() error { return wrapFsError(fs.destination.Chtimes(name, atime, mtime)) })

	return wrapFsError(fs.Fs.Chtimes(name, atime, mtime))
}

// Sync syncs the file system, i.e. makes sure all the waiting operations
// have been done.
func (fs *Fs) Sync() error {
	wait := make(chan error)

	fs.queueOperation(func() error {
		wait <- nil

		return nil
	})

	return <-wait
}
