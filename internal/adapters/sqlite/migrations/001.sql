CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS records(kind TEXT NOT NULL,id TEXT NOT NULL,body TEXT NOT NULL CHECK(json_valid(body)),PRIMARY KEY(kind,id));
CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(kind UNINDEXED,id UNINDEXED,body,tokenize='unicode61');
CREATE TRIGGER IF NOT EXISTS records_ai AFTER INSERT ON records WHEN new.kind IN ('node','document') BEGIN INSERT INTO search(kind,id,body) VALUES(new.kind,new.id,new.body); END;
CREATE TRIGGER IF NOT EXISTS records_ad AFTER DELETE ON records WHEN old.kind IN ('node','document') BEGIN DELETE FROM search WHERE kind=old.kind AND id=old.id; END;
CREATE TRIGGER IF NOT EXISTS records_au AFTER UPDATE ON records WHEN new.kind IN ('node','document') BEGIN DELETE FROM search WHERE kind=old.kind AND id=old.id; INSERT INTO search(kind,id,body) VALUES(new.kind,new.id,new.body); END;
INSERT OR IGNORE INTO schema_migrations VALUES(1);
