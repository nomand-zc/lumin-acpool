CREATE TABLE IF NOT EXISTS "account_occupancy" (
    "account_id" TEXT    NOT NULL,
    "count"      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY ("account_id")
);
