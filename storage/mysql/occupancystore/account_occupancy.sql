CREATE TABLE IF NOT EXISTS `account_occupancy` (
    `account_id` VARCHAR(255) NOT NULL COMMENT '账号ID',
    `count`      BIGINT       NOT NULL DEFAULT 0 COMMENT '当前并发占用计数',
    PRIMARY KEY (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号并发占用计数表';
