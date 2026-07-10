package main

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&dbUser{}, &dbBulletin{}, &dbBoard{}, &dbMessage{}, &dbAPRSSent{}, &dbAPRSSentPart{}, &dbAPRSReceived{}); err != nil {
		return nil, err
	}
	return db, nil
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
