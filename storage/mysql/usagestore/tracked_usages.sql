CREATE TABLE IF NOT EXISTS `tracked_usages` (
    `id`             BIGINT       NOT NULL AUTO_INCREMENT COMMENT '自增主键',
    `account_id`     VARCHAR(255) NOT NULL COMMENT '账号ID',
    `rule_index`     INT          NOT NULL COMMENT '规则索引',
    `source_type`    INT          NOT NULL DEFAULT 0 COMMENT '来源类型',
    `time_granularity` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '时间粒度',
    `window_size`    INT          NOT NULL DEFAULT 0 COMMENT '窗口大小',
    `rule_total`     DOUBLE       NOT NULL DEFAULT 0 COMMENT '规则总量',
    `local_used`     DOUBLE       NOT NULL DEFAULT 0 COMMENT '本地已用量',
    `remote_used`    DOUBLE       NOT NULL DEFAULT 0 COMMENT '远端已用量',
    `remote_remain`  DOUBLE       NOT NULL DEFAULT 0 COMMENT '远端剩余量',
    `window_start`   DATETIME(3)           DEFAULT NULL COMMENT '窗口开始时间',
    `window_end`     DATETIME(3)           DEFAULT NULL COMMENT '窗口结束时间',
    `last_sync_at`   DATETIME(3)  NOT NULL COMMENT '上次同步时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_account_rule` (`account_id`, `rule_index`),
    INDEX `idx_account_id` (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用量追踪数据表';
