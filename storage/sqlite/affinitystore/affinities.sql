CREATE TABLE IF NOT EXISTS "affinities" (
    "affinity_key" TEXT NOT NULL,
    "target_id"    TEXT NOT NULL,
    "updated_at"   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    PRIMARY KEY ("affinity_key")
);

CREATE INDEX IF NOT EXISTS "idx_affinities_target_id" ON "affinities" ("target_id");
