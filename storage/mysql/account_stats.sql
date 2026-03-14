CREATE TABLE IF NOT EXISTS `account_stats` (
    `account_id`           VARCHAR(255) NOT NULL COMMENT '账号ID',
    `total_calls`          BIGINT       NOT NULL DEFAULT 0 COMMENT '总调用次数',
    `success_calls`        BIGINT       NOT NULL DEFAULT 0 COMMENT '成功调用次数',
    `failed_calls`         BIGINT       NOT NULL DEFAULT 0 COMMENT '失败调用次数',
    `consecutive_failures` INT          NOT NULL DEFAULT 0 COMMENT '连续失败次数',
    `last_used_at`         DATETIME(3)           DEFAULT NULL COMMENT '最后使用时间',
    `last_error_at`        DATETIME(3)           DEFAULT NULL COMMENT '最后错误时间',
    `last_error_msg`       TEXT                  DEFAULT NULL COMMENT '最后错误消息',
    PRIMARY KEY (`account_id`),
    CONSTRAINT `fk_account_stats_account` FOREIGN KEY (`account_id`)
        REFERENCES `accounts` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号运行时统计表';
