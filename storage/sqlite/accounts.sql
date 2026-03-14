CREATE TABLE IF NOT EXISTS "accounts" (
    "id"                 TEXT    NOT NULL,
    "provider_type"      TEXT    NOT NULL,
    "provider_name"      TEXT    NOT NULL,
    "credential"         TEXT    NOT NULL,
    "status"             INTEGER NOT NULL DEFAULT 1,
    "priority"           INTEGER NOT NULL DEFAULT 0,
    "tags"               TEXT             DEFAULT NULL,
    "metadata"           TEXT             DEFAULT NULL,
    "usage_rules"        TEXT             DEFAULT NULL,
    "cooldown_until"     TEXT             DEFAULT NULL,
    "circuit_open_until" TEXT             DEFAULT NULL,
    "created_at"         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at"         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "version"            INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY ("id")
);

CREATE INDEX IF NOT EXISTS "idx_accounts_provider" ON "accounts" ("provider_type", "provider_name");
CREATE INDEX IF NOT EXISTS "idx_accounts_status" ON "accounts" ("status");
CREATE INDEX IF NOT EXISTS "idx_accounts_priority" ON "accounts" ("priority");
