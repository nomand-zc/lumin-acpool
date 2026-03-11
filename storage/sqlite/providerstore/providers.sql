CREATE TABLE IF NOT EXISTS "providers" (
    "provider_type"           TEXT    NOT NULL,
    "provider_name"           TEXT    NOT NULL,
    "status"                  INTEGER NOT NULL DEFAULT 1,
    "priority"                INTEGER NOT NULL DEFAULT 0,
    "weight"                  INTEGER NOT NULL DEFAULT 1,
    "tags"                    TEXT             DEFAULT NULL,
    "supported_models"        TEXT             DEFAULT NULL,
    "usage_rules"             TEXT             DEFAULT NULL,
    "metadata"                TEXT             DEFAULT NULL,
    "account_count"           INTEGER NOT NULL DEFAULT 0,
    "available_account_count" INTEGER NOT NULL DEFAULT 0,
    "created_at"              TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at"              TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    PRIMARY KEY ("provider_type", "provider_name")
);

CREATE INDEX IF NOT EXISTS "idx_providers_status" ON "providers" ("status");
CREATE INDEX IF NOT EXISTS "idx_providers_priority" ON "providers" ("priority");
