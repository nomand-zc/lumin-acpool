CREATE TABLE IF NOT EXISTS `affinities` (
    `affinity_key` VARCHAR(512) NOT NULL COMMENT '亲和键',
    `target_id`    VARCHAR(255) NOT NULL COMMENT '绑定目标ID',
    `updated_at`   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (`affinity_key`),
    INDEX `idx_target_id` (`target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='亲和绑定关系表';
