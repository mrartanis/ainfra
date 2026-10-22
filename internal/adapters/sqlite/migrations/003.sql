CREATE INDEX IF NOT EXISTS records_document_entry ON records(json_extract(body,'$.document_id'),json_extract(body,'$.created_at'),id) WHERE kind='journal-entry';
INSERT OR IGNORE INTO schema_migrations VALUES(3);
