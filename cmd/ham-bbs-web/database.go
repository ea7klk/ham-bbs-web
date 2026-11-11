package main

import (
	"errors"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDatabase(path string) (*gorm.DB, error) {
	// _txlock=immediate makes every transaction take the write lock at BEGIN
	// instead of at its first write, and _busy_timeout makes a waiting writer
	// block rather than fail. The classic SSH BBS opens the same file with the
	// same settings; matching them is what serialises read-then-write
	// transactions across the two applications instead of letting them
	// interleave and lose an update.
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&dbUser{}, &dbBulletin{}, &dbBoard{}, &dbMessage{}, &dbAPRSSent{}, &dbAPRSSentPart{}, &dbAPRSReceived{},
		&dbActivation{}, &dbToken{}, &dbRegistration{}, &dbSetting{}, &dbAPRSBulletin{}); err != nil {
		return nil, err
	}
	if err := backfillActivated(db); err != nil {
		return nil, err
	}
	if err := backfillRegistered(db); err != nil {
		return nil, err
	}
	return db, nil
}

// backfillActivated marks pre-existing accounts as activated.
//
// The column defaults to false, which is right for a new registration and wrong
// for every account that existed before activation was introduced. Two things
// have to happen, and both are shared with the classic SSH BBS, which runs the
// same migration against the same table:
//
//   - Rows where the column is NULL were created by the migration that added it
//     rather than by an insert, so NULL is never a deliberate value. Correcting
//     those on every start also repairs a database where an earlier version of
//     this function recorded its marker without matching the NULL rows.
//   - Rows that are explicitly false predate activation too, but only the first
//     time: an account created later and deliberately left unactivated must stay
//     that way. The marker row bounds it to one sweep, and it is the same marker
//     the SSH BBS uses, so whichever application starts first does the sweep.
func backfillActivated(db *gorm.DB) error {
	const key = "migrated.activation"
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dbUser{}).Where("activated IS NULL").UpdateColumn("activated", true).Error; err != nil {
			return err
		}
		marker := dbSetting{}
		err := tx.Where("key = ?", key).Take(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&dbUser{}).Where("activated = ?", false).UpdateColumn("activated", true).Error; err != nil {
			return err
		}
		return tx.Create(&dbSetting{Key: key, Value: "1"}).Error
	})
}

// backfillRegistered gives accounts created before the column existed a
// registration date taken from their first-seen stamp, which was written at
// creation. It is idempotent: after the first run the WHERE clause matches
// nothing. The classic SSH BBS runs the same update on startup; whichever
// application opens the database first fills the column, and rows with no
// first_seen stay empty rather than being given a fabricated date.
func backfillRegistered(db *gorm.DB) error {
	return db.Model(&dbUser{}).
		Where("(registered IS NULL OR registered = '') AND first_seen IS NOT NULL AND first_seen <> ''").
		UpdateColumn("registered", gorm.Expr("first_seen")).Error
}

func (s *server) seedDefaultData() error {
	var count int64
	if err := s.db.Model(&dbBulletin{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		now := now()
		if err := s.db.Create(&[]dbBulletin{
			{Position: 0, Title: "Welcome", Body: "This is a small HamNet-ready BBS for radio operators.\nUse it for local notes, net announcements, and station contact info.", Updated: now},
			{Position: 1, Title: "Operating Notes", Body: "Keep traffic courteous and relevant to amateur radio.\nDo not post private keys, passwords, or third-party personal data.", Updated: now},
		}).Error; err != nil {
			return err
		}
	}
	if err := s.db.Model(&dbBoard{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return s.db.Create(&dbBoard{ID: defaultBoardID, Position: 0, Name: "General", Description: "General local messages", Created: now()}).Error
	}
	return nil
}
