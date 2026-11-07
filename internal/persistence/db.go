package persistence

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content,
    content='memories',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE OF content ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
    INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
END;
`

func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Memory{}, &Topic{}); err != nil {
		return err
	}
	if err := db.Exec(ftsSchema).Error; err != nil {
		return err
	}

	return nil
}

func DB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(`file:mem.db?_pragma=busy_timeout(5000)
		&_pragma=foreign_keys(1)
		&_pragma=journal_mode(WAL)&_txlock=immediate`), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = migrate(db)
	if err != nil {
		return nil, err
	}

	return db, nil
}
