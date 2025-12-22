package memiavl

import (
	"errors"
	fmt "fmt"
	"os"
	"path/filepath"

	"github.com/zbiljic/go-filelock"
)

type FileLock interface {
	Unlock() error
	Destroy() error
}

type fileLock struct {
	FileLock
	path string
}

func (l *fileLock) Destroy() error {
	errs := []error{}

	if err := l.FileLock.Destroy(); err != nil {
		errs = append(errs, err)
	}

	if err := os.Remove(l.path); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func LockFile(fname string) (FileLock, error) {
	path, err := filepath.Abs(fname)
	if err != nil {
		return nil, err
	}
	fl, err := filelock.New(path)
	if err != nil {
		return nil, err
	}
	if _, err := fl.TryLock(); err != nil {
		return nil, err
	}

	return fl, nil
}

// AcquireExporterLock creates a temporary lock file for the snapshot at the given height
// to prevent deletion while exporting.
//
// The lock is removed after export, and startup cleanup also purges all
// -tmp suffixed files, so this file is safe to leave behind on crashes.
func AcquireExporterLock(dir string, height int64) (FileLock, error) {
	path := filepath.Join(dir, fmt.Sprintf("exporter-lock-%d%s", height, TmpSuffix))
	fl, err := LockFile(path)
	if err != nil {
		return nil, err
	}

	return &fileLock{
		FileLock: fl,
		path:     path,
	}, nil
}
