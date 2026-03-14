CREATE TABLE IF NOT EXISTS "tracked_usages" (
    "id"               INTEGER PRIMARY KEY AUTOINCREMENT,
    "account_id"       TEXT    NOT NULL,
    "rule_index"       INTEGER NOT NULL,
    "source_type"      INTEGER NOT NULL DEFAULT 0,
    "time_granularity" TEXT    NOT NULL DEFAULT '',
    "window_size"      INTEGER NOT NULL DEFAULT 0,
    "rule_total"       REAL    NOT NULL DEFAULT 0,
    "local_used"       REAL    NOT NULL DEFAULT 0,
    "remote_used"      REAL    NOT NULL DEFAULT 0,
    "remote_remain"    REAL    NOT NULL DEFAULT 0,
    "window_start"     TEXT             DEFAULT NULL,
    "window_end"       TEXT             DEFAULT NULL,
    "last_sync_at"     TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_tracked_usages_account_rule" ON "tracked_usages" ("account_id", "rule_index");
CREATE INDEX IF NOT EXISTS "idx_tracked_usages_account_id" ON "tracked_usages" ("account_id");
