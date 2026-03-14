CREATE TABLE IF NOT EXISTS `providers` (
    `provider_type`           VARCHAR(128) NOT NULL COMMENT '供应商类型',
    `provider_name`           VARCHAR(128) NOT NULL COMMENT '供应商实例名称',
    `status`                  INT          NOT NULL DEFAULT 1 COMMENT '供应商状态',
    `priority`                INT          NOT NULL DEFAULT 0 COMMENT '优先级',
    `weight`                  INT          NOT NULL DEFAULT 1 COMMENT '权重',
    `tags`                    JSON                  DEFAULT NULL COMMENT '标签（JSON格式）',
    `supported_models`        JSON                  DEFAULT NULL COMMENT '支持的模型列表（JSON数组）',
    `usage_rules`             JSON                  DEFAULT NULL COMMENT '用量规则（JSON数组）',
    `metadata`                JSON                  DEFAULT NULL COMMENT '扩展元数据（JSON格式）',
    `account_count`           INT          NOT NULL DEFAULT 0 COMMENT '账号总数',
    `available_account_count` INT          NOT NULL DEFAULT 0 COMMENT '可用账号数',
    `created_at`              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    `updated_at`              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (`provider_type`, `provider_name`),
    INDEX `idx_status` (`status`),
    INDEX `idx_priority` (`priority`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='供应商信息表';
