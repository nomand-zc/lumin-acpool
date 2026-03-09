# 实施计划

- [ ] 1. 创建 `builder` 子包基础结构：`BuildContext` 与 `MessageBuilder` 接口
   - 在 `converter/builder/` 目录下新建 `context.go`，定义 `BuildContext` 结构体（含 `SystemPrompt`、`Messages`、`History`、`KiroTools`、`CurrentContent`、`CurrentImages`、`CurrentToolResults`、`ModelId`、`Done` 等字段）
   - 在 `context.go` 中定义 `MessageBuilder` 接口（`Build(ctx *BuildContext) error`）
   - _需求：1.1、1.2、1.3_

- [ ] 2. 迁移公共辅助函数到 `builder/helpers.go`
   - 将 `req_converter.go` 中的 `convertImage`、`convertSchema`、`parseJSONOrString`、`deduplicateToolResults`、`getMessageText` 迁移到 `builder/helpers.go`，并导出（首字母大写）
   - _需求：8.1、8.2_

- [ ] 3. 实现 `preprocess_builder.go`：消息预处理构建器
   - 将 `preprocessMessages` 逻辑封装为实现 `MessageBuilder` 接口的 `PreprocessBuilder` 结构体
   - 预处理后写入 `BuildContext.Messages`；若结果为空则设置 `BuildContext.Done = true`
   - _需求：2.1、2.2、2.3_

- [ ] 4. 实现 `system_prompt_builder.go`：system prompt 提取构建器
   - 将 system prompt 提取逻辑封装为 `SystemPromptBuilder`，写入 `BuildContext.SystemPrompt`，非 system 消息写回 `BuildContext.Messages`
   - 多条 system 消息用 `\n\n` 合并；若过滤后消息为空则设置 `BuildContext.Done = true`
   - _需求：3.1、3.2、3.3_

- [ ] 5. 实现 `tools_builder.go`：工具列表构建器
   - 将 `buildKiroTools` 逻辑封装为 `ToolsBuilder`，结果写入 `BuildContext.KiroTools`
   - 保留过滤（web_search/websearch、空名称、空描述）、截断、占位工具（`no_tool_available`）逻辑
   - _需求：4.1、4.2、4.3_

- [ ] 6. 实现 `history_builder.go`：历史消息构建器
   - 将历史消息构建逻辑（含 system prompt 合并到首条 user 消息、`buildHistoryUserMessage`、`buildAssistantMessage`、图片保留策略）封装为 `HistoryBuilder`，结果写入 `BuildContext.History`
   - _需求：5.1、5.2、5.3、5.4、5.5_

- [ ] 7. 实现 `current_message_builder.go`：当前消息构建器
   - 将最后一条消息的解析逻辑封装为 `CurrentMessageBuilder`，结果写入 `BuildContext.CurrentContent`、`BuildContext.CurrentImages`、`BuildContext.CurrentToolResults`
   - 覆盖 assistant/user/tool 三种末尾消息场景及 `currentContent` 兜底逻辑
   - _需求：6.1、6.2、6.3、6.4、6.5、6.6_

- [ ] 8. 实现 `assembler.go`：最终请求组装器
   - 将 `BuildContext` 中的数据组装为 `*Request`（含 `ConversationState`、`UserInputMessage`、`UserInputMessageContext`、`History`）
   - _需求：7.3_

- [ ] 9. 重构 `req_converter.go` 为流水线调度器，并清理旧代码
   - 将 `ConvertRequest` 函数改写为按序调用各 builder 的流水线（遇到 `Done=true` 提前返回 `nil`），删除已迁移的辅助函数和内联逻辑
   - 运行 `req_converter_test.go` 中所有测试用例，确保全部通过
   - _需求：7.1、7.2、7.4、8.2_
