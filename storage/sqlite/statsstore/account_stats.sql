CREATE TABLE IF NOT EXISTS "account_stats" (
    "account_id"           TEXT    NOT NULL,
    "total_calls"          INTEGER NOT NULL DEFAULT 0,
    "success_calls"        INTEGER NOT NULL DEFAULT 0,
    "failed_calls"         INTEGER NOT NULL DEFAULT 0,
    "consecutive_failures" INTEGER NOT NULL DEFAULT 0,
    "last_used_at"         TEXT             DEFAULT NULL,
    "last_error_at"        TEXT             DEFAULT NULL,
    "last_error_msg"       TEXT             DEFAULT NULL,
    PRIMARY KEY ("account_id")
);
