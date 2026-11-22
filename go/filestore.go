//go:build linux || darwin || freebsd || netbsd || openbsd

package eid

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// FileStore is a lease store in a directory, for processes that start and end on one host. The directory holds one
// file per system, named by its code, whose content is the stored grant second in decimal; and two kinds of lock file:
//
//   - lock serialises every read-compare-write of CompareAndSet across processes with flock;
//   - <system>.active is held with flock from Activate until Release or until the process ends, so a second
//     activation of a system on one directory is refused whichever process makes it.
//
// A record is replaced as a whole: the new value is written to a temporary file, synced, renamed over the record, and
// the directory is synced. CompareAndSet returns true only after that, so a grant is on disk before its lease is
// returned. A record that cannot be read or parsed reads as the largest second, so the allocator refuses (clock-behind)
// instead of granting again.
//
// One store holds the state of one allocator. Two hosts that share a system must not each keep a store of it.
type FileStore struct {
	dir string

	mu     sync.Mutex
	active map[Code]*os.File
	err    error

	noLock bool // set only by tests, to show what the lock prevents
}

// OpenFileStore opens the store in dir, creating the directory if it does not exist.
func OpenFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("eid: " + dir + " is not a directory")
	}
	return &FileStore{dir: dir, active: map[Code]*os.File{}}, nil
}

// Dir returns the directory of the store.
func (s *FileStore) Dir() string { return s.dir }

// Err returns the last input or output error of the store, or nil. The LeaseStore methods report only a bool, so a
// caller that sees a refusal it did not expect asks here.
func (s *FileStore) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *FileStore) fail(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *FileStore) path(system Code, suffix string) string {
	return filepath.Join(s.dir, system.String()+suffix)
}

func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			return err
		}
	}
}

// lock takes the store's lock for one read-compare-write and returns the function that gives it up.
func (s *FileStore) lock() (func(), error) {
	if s.noLock {
		return func() {}, nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flock(f, syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		flock(f, syscall.LOCK_UN)
		f.Close()
	}, nil
}

var errCorruptRecord = errors.New("eid: the stored grant second is not a decimal number")

func parseRecord(b []byte) (uint64, bool) {
	t := strings.TrimSuffix(string(b), "\n")
	if t == "" || len(t) > 20 || (len(t) > 1 && t[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(t); i++ {
		if t[i] < '0' || t[i] > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseUint(t, 10, 64)
	return v, err == nil
}

// read returns the stored grant second of system; a missing record is (0, false, nil).
func (s *FileStore) read(system Code) (uint64, bool, error) {
	b, err := os.ReadFile(s.path(system, ""))
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	v, ok := parseRecord(b)
	if !ok {
		return 0, false, errCorruptRecord
	}
	return v, true, nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	d.Close()
	if errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}

// write replaces the record of system by v, durably.
func (s *FileStore) write(system Code, v uint64) error {
	f, err := os.CreateTemp(s.dir, system.String()+".tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	_, err = f.WriteString(strconv.FormatUint(v, 10) + "\n")
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, s.path(system, ""))
	}
	if err != nil {
		os.Remove(name)
		return err
	}
	return syncDir(s.dir)
}

// LastSecond implements LeaseStore.
func (s *FileStore) LastSecond(system Code) (uint64, bool) {
	v, ok, err := s.read(system)
	if err != nil {
		s.fail(err)
		return ^uint64(0), true
	}
	return v, ok
}

// CompareAndSet implements LeaseStore.
func (s *FileStore) CompareAndSet(system Code, expected uint64, exists bool, next uint64) bool {
	unlock, err := s.lock()
	if err != nil {
		s.fail(err)
		return false
	}
	defer unlock()
	cur, has, err := s.read(system)
	if err != nil {
		s.fail(err)
		return false
	}
	if has != exists || (has && cur != expected) {
		return false
	}
	if s.noLock {
		time.Sleep(time.Millisecond) // widens the window between the compare and the write
	}
	if err := s.write(system, next); err != nil {
		s.fail(err)
		return false
	}
	return true
}

// Activate implements LeaseStore: it takes the system's activation lock without waiting.
func (s *FileStore) Activate(system Code) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[system] != nil {
		return false
	}
	f, err := os.OpenFile(s.path(system, ".active"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		s.err = err
		return false
	}
	if err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err != syscall.EWOULDBLOCK {
			s.err = err
		}
		return false
	}
	s.active[system] = f
	return true
}

// Release implements LeaseStore.
func (s *FileStore) Release(system Code) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.active[system]; f != nil {
		flock(f, syscall.LOCK_UN)
		f.Close()
		delete(s.active, system)
	}
}
