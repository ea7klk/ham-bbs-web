package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The shared database belongs to the classic BBS, which is canonical. These tests
// pin the two ways this app must not diverge from it, both of which happened on
// 2026-11-12 and neither of which was visible from inside the process.

// A missing database must not be created. SQLite creates a file on open, and the
// old code then seeded it — a board with default bulletins and no accounts,
// served quietly while the real database was elsewhere.
func TestOpenRefusesToCreateAMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.sqlite")

	_, err := openCanonicalDatabase(path, 150*time.Millisecond)
	if err == nil {
		t.Fatal("a missing database was opened, which creates it")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("the file was created anyway")
	}
	// The message has to name the cause and the fix: this is what an operator
	// sees from a crash-looping pod.
	for _, want := range []string{"does not exist", "classic SSH BBS", "will not create"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not mention %q: %v", want, err)
		}
	}
}

// It waits for the canonical side to create the file, because on a fresh rollout
// both pods start together.
func TestOpenWaitsForTheCanonicalSide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.sqlite")

	// The canonical side builds the schema elsewhere and moves it into place, as a
	// restore does. Building it in place would race this test's own open and both
	// would run AutoMigrate against the same file — a flaw in the fixture, not in
	// the code under test.
	go func() {
		time.Sleep(200 * time.Millisecond)
		staging := filepath.Join(filepath.Dir(path), "staging.sqlite")
		if db, err := openDatabase(staging); err == nil {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		_ = os.Rename(staging, path)
	}()

	db, err := openCanonicalDatabase(path, 5*time.Second)
	if err != nil {
		t.Fatalf("did not wait for the database to appear: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// The incident, reproduced: the file is replaced while the process holds it open.
//
// On 2026-11-12 the volume was re-provisioned and a restore landed the next
// morning. The SSH BBS opened the new file per session; this app kept reading the
// unlinked inode, so logins failed with "record not found" for an account that was
// on disk the whole time.
func TestWatchNoticesTheFileBeingReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bbs.sqlite")

	if db, err := openDatabase(path); err != nil {
		t.Fatal(err)
	} else if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	watch, err := newDBFileWatch(path)
	if err != nil {
		t.Fatal(err)
	}
	if reason, lost := watch.changed(); lost {
		t.Fatalf("reported a change with nothing changed: %s", reason)
	}

	// Replace the file the way a restore does: write a new one and rename it over
	// the path, so the old inode is unlinked but still open.
	replacement := filepath.Join(dir, "replacement.sqlite")
	if err := os.WriteFile(replacement, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	reason, lost := watch.changed()
	if !lost {
		t.Fatal("a replaced database was not noticed")
	}
	if !strings.Contains(reason, "replaced") {
		t.Fatalf("reason = %q, want it to name the replacement", reason)
	}
}

// Removal is noticed too, not just replacement.
func TestWatchNoticesTheFileDisappearing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bbs.sqlite")
	if db, err := openDatabase(path); err != nil {
		t.Fatal(err)
	} else if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	watch, err := newDBFileWatch(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reason, lost := watch.changed()
	if !lost || !strings.Contains(reason, "no longer there") {
		t.Fatalf("removal = %q,%t", reason, lost)
	}
}

// Writing to the file must not be mistaken for replacing it: a healthy board
// writes constantly, and a watcher that restarted on every write would be worse
// than the bug it fixes.
func TestWatchIgnoresWritesToTheSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bbs.sqlite")
	db, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	watch, err := newDBFileWatch(path)
	if err != nil {
		t.Fatal(err)
	}
	// Real writes, through the same handle the app uses.
	for i := 0; i < 20; i++ {
		if err := db.Create(&dbBulletin{Position: i, Title: "x", Body: "y", Updated: now()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if reason, lost := watch.changed(); lost {
		t.Fatalf("a write was mistaken for a replacement: %s", reason)
	}
}

// The watcher fires once and then stops, so a replacement cannot produce a storm
// of exits or callbacks.
func TestWatchFiresOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bbs.sqlite")
	if db, err := openDatabase(path); err != nil {
		t.Fatal(err)
	} else if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	watch, err := newDBFileWatch(path)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	fired := 0
	s := &server{dbWatch: watch}
	s.watchDatabaseFile(5*time.Millisecond, func(string) {
		mu.Lock()
		fired++
		mu.Unlock()
	})

	// Let a few clean intervals pass first: nothing may fire.
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	if fired != 0 {
		mu.Unlock()
		t.Fatal("fired on a healthy file")
	}
	mu.Unlock()

	replacement := filepath.Join(dir, "r.sqlite")
	if err := os.WriteFile(replacement, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	got := fired
	mu.Unlock()
	if got != 1 {
		t.Fatalf("fired %d times, want exactly 1", got)
	}
}

// A nil watch or a zero interval is a no-op rather than a panic, so a partial
// server (as the test fixtures build) still works.
func TestWatchIsSafeWithoutAFile(t *testing.T) {
	s := &server{}
	s.watchDatabaseFile(time.Millisecond, func(string) {})
	s.watchDatabaseFile(0, func(string) {})
}
