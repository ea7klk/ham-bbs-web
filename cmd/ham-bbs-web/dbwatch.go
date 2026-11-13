package main

import (
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
)

// The database file, which is not this application's to own.
//
// The classic SSH BBS is canonical (see docs/shared-database.md). It opens the
// SQLite file per session, so it always reads whatever is on disk. This app opens
// the file once at startup and holds that handle for the life of the process,
// which makes it fragile in two ways that are both silent:
//
//  1. **The file can be replaced underneath it.** A restored backup, a
//     re-provisioned volume, an operator copying a file into place. The old inode
//     is unlinked but still open, so this app keeps reading a database that no
//     longer exists on disk while the canonical one moves on. On 2026-11-12 the
//     ham-bbs volumes were re-provisioned and a restore landed the next morning;
//     this app served a deleted database for fifteen hours, and logins failed
//     because the account was not in the copy it was reading.
//
//  2. **Opening a missing file creates one.** SQLite creates the file on open, so
//     a lost volume produced an empty database that this app then seeded and
//     served — a board with no accounts, quietly, while the real one was
//     elsewhere. That is divergence created rather than inherited: worse, because
//     it looks like a working BBS.
//
// These functions close both. The watch notices a replacement and ends the
// process so the supervisor restarts it and reopens; refusing to create means the
// restart cannot manufacture an empty board in the meantime.

const (
	// dbFilePollInterval is how often the watched file is checked. One stat per
	// minute is nothing, and a restore or volume change is noticed within it.
	dbFilePollInterval = 60 * time.Second
	// dbFileWaitTimeout bounds how long startup waits for the canonical side to
	// create the file, so a rollout where both pods start together does not
	// crash-loop before the SSH BBS has migrated.
	dbFileWaitTimeout = 90 * time.Second
	// dbFileWaitPoll is how often it looks while waiting.
	dbFileWaitPoll = 250 * time.Millisecond
)

// dbFileWatch remembers which file was opened, so a later stat can tell whether
// the path still refers to it.
type dbFileWatch struct {
	path string
	info os.FileInfo
}

// newDBFileWatch records the identity of the file that was just opened.
func newDBFileWatch(path string) (*dbFileWatch, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	return &dbFileWatch{path: path, info: info}, nil
}

// changed reports whether the file at the watched path is no longer the file that
// was opened, and says why in words an operator can act on.
//
// os.SameFile compares the device and inode (or the file index on Windows), which
// is exactly the question: is this the same file, not merely the same path.
func (w *dbFileWatch) changed() (string, bool) {
	current, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "the database file is no longer there", true
		}
		return fmt.Sprintf("the database file cannot be inspected: %v", err), true
	}
	if !os.SameFile(w.info, current) {
		return "the database file was replaced", true
	}
	return "", false
}

// watchDatabaseFile polls the watched file and calls onLost once, if the file is
// replaced or removed. It never fires spuriously: an unchanged file produces no
// callback, so this cannot restart a healthy pod.
func (s *server) watchDatabaseFile(interval time.Duration, onLost func(reason string)) {
	if interval <= 0 || onLost == nil || s.dbWatch == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if reason, lost := s.dbWatch.changed(); lost {
				onLost(reason)
				return
			}
		}
	}()
}

// openCanonicalDatabase opens the shared database, refusing to create it.
//
// A missing file means the canonical side has not created it yet, or the volume
// was lost. Either way this app must not be the one to make an empty one: it
// would seed default content and serve it, which is a plausible-looking BBS with
// none of the real data in it. It waits briefly for the SSH BBS to create the
// file, because on a fresh rollout the two pods start together, then fails with
// a message that names the fix.
func openCanonicalDatabase(path string, wait time.Duration) (*gorm.DB, error) {
	deadline := time.Now().Add(wait)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"database %s does not exist: the classic SSH BBS creates and owns it, so start that first. "+
					"This app will not create an empty one, because serving a seeded board in place of the real "+
					"database is how a lost volume turns into a silent data divergence", path)
		}
		time.Sleep(dbFileWaitPoll)
	}
	db, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	return db, nil
}
