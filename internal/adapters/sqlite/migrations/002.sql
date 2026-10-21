CREATE INDEX IF NOT EXISTS records_node ON records(kind,json_extract(body,'$.node_id'));
INSERT OR IGNORE INTO schema_migrations VALUES(2);
INSERT OR REPLACE INTO records(kind,id,body)
SELECT 'finding-summary',node_id,json_object('node_id',node_id,'buckets',json_group_array(json_object('assessment',assessment,'remediation',remediation,'workflow',workflow,'suppressed_until',suppressed_until,'count',count)))
FROM (SELECT json_extract(body,'$.node_id') node_id,json_extract(body,'$.assessment') assessment,json_extract(body,'$.remediation') remediation,json_extract(body,'$.workflow') workflow,json_extract(body,'$.suppressed_until') suppressed_until,COUNT(*) count FROM records WHERE kind='finding' GROUP BY node_id,assessment,remediation,workflow,suppressed_until)
GROUP BY node_id;
