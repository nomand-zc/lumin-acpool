# 需求文档：重构 ConvertRequest 请求转换器

## 引言

当前 `req_converter.go` 中的 `ConvertRequest` 函数将所有转换逻辑集中在一个文件中，包含消息预处理、system prompt 提取、工具构建、历史消息构建、当前消息构建等多个职责，导致代码模块化程度低、可读性差、扩展性不足。

相比之下，`resp_converter.go` 采用了"注册器 + 策略模式"的架构：`ConvertResponse` 本身只负责路由分发，具体的解析逻辑被拆分到 `parser/` 子包下的多个独立文件中，每个文件只负责一种消息类型的处理，并通过 `init()` 自动注册。

本次重构目标是参考 `ConvertResponse` 的架构设计，将 `ConvertRequest` 的各个处理阶段拆分为独立的、职责单一的构建器（builder），提升代码的模块化程度、可读性和扩展性，同时保持与现有测试的完全兼容。

---

## 需求

### 需求 1：建立 `builder` 子包，定义统一的构建器接口

**用户故事：** 作为一名开发者，我希望 `ConvertRequest` 的各处理阶段通过统一接口进行组织，以便新增或修改某一阶段时不影响其他阶段。

#### 验收标准

1. WHEN 创建 `builder` 子包时，THEN 系统 SHALL 在 `converter/builder/` 目录下定义核心接口和公共类型（如 `BuildContext`、`MessageBuilder` 接口等）。
2. WHEN 定义 `BuildContext` 时，THEN 系统 SHALL 包含构建过程中各阶段共享的中间状态字段（如 `SystemPrompt`、`Messages`、`History`、`KiroTools`、`CurrentContent`、`CurrentImages`、`CurrentToolResults` 等）。
3. IF 某个构建阶段需要读取上一阶段的输出，THEN 系统 SHALL 通过 `BuildContext` 传递，而非通过函数参数层层传递。

---

### 需求 2：将消息预处理逻辑拆分为独立构建器

**用户故事：** 作为一名开发者，我希望消息预处理（移除末尾 `{` 的 assistant 消息、合并相邻同 role 消息）被封装为独立模块，以便单独测试和维护。

#### 验收标准

1. WHEN 执行消息预处理时，THEN 系统 SHALL 将 `preprocessMessages` 逻辑封装到 `builder/preprocess_builder.go` 中。
2. WHEN 预处理构建器执行时，THEN 系统 SHALL 将处理后的消息列表写入 `BuildContext.Messages`。
3. IF 预处理后消息列表为空，THEN 系统 SHALL 在 `BuildContext` 中标记终止标志，使后续构建器跳过执行。

---

### 需求 3：将 system prompt 提取逻辑拆分为独立构建器

**用户故事：** 作为一名开发者，我希望 system prompt 的提取和合并逻辑被封装为独立模块，以便在不影响其他逻辑的情况下修改 system prompt 的处理策略。

#### 验收标准

1. WHEN 提取 system prompt 时，THEN 系统 SHALL 将相关逻辑封装到 `builder/system_prompt_builder.go` 中。
2. WHEN system prompt 构建器执行时，THEN 系统 SHALL 将提取的 system prompt 写入 `BuildContext.SystemPrompt`，并将非 system 消息写入 `BuildContext.Messages`。
3. IF 存在多条 system 消息，THEN 系统 SHALL 将其内容用 `\n\n` 连接后合并为一个 system prompt。

---

### 需求 4：将工具列表构建逻辑拆分为独立构建器

**用户故事：** 作为一名开发者，我希望工具过滤、截断、占位逻辑被封装为独立模块，以便在不影响消息构建逻辑的情况下调整工具处理策略。

#### 验收标准

1. WHEN 构建工具列表时，THEN 系统 SHALL 将 `buildKiroTools` 逻辑封装到 `builder/tools_builder.go` 中。
2. WHEN 工具构建器执行时，THEN 系统 SHALL 将构建好的工具列表写入 `BuildContext.KiroTools`。
3. IF 工具列表为空或全部被过滤，THEN 系统 SHALL 使用占位工具（`no_tool_available`）填充。

---

### 需求 5：将历史消息构建逻辑拆分为独立构建器

**用户故事：** 作为一名开发者，我希望历史消息的构建逻辑（包括 system prompt 合并到首条 user 消息、图片保留策略、user/assistant 消息转换）被封装为独立模块。

#### 验收标准

1. WHEN 构建历史消息时，THEN 系统 SHALL 将相关逻辑封装到 `builder/history_builder.go` 中。
2. WHEN 历史消息构建器执行时，THEN 系统 SHALL 将构建好的 `[]HistoryItem` 写入 `BuildContext.History`。
3. IF system prompt 非空且第一条消息为 user 消息，THEN 系统 SHALL 将 system prompt 与第一条 user 消息内容合并。
4. IF system prompt 非空且第一条消息不是 user 消息，THEN 系统 SHALL 将 system prompt 单独作为一条 user 历史消息插入。
5. WHEN 处理历史消息中的图片时，THEN 系统 SHALL 按照 `keepImageThreshold` 策略决定保留或替换为占位符。

---

### 需求 6：将当前消息（currentMessage）构建逻辑拆分为独立构建器

**用户故事：** 作为一名开发者，我希望最后一条消息（currentMessage）的解析和构建逻辑被封装为独立模块，以便单独处理 user/tool/assistant 等不同角色的末尾消息场景。

#### 验收标准

1. WHEN 构建当前消息时，THEN 系统 SHALL 将相关逻辑封装到 `builder/current_message_builder.go` 中。
2. IF 最后一条消息为 assistant 消息，THEN 系统 SHALL 将其加入 history，并将 `currentContent` 设为 `"Continue"`。
3. IF 最后一条消息为 user 消息且包含 ContentParts，THEN 系统 SHALL 分别提取文本内容和图片内容。
4. IF 最后一条消息为 tool 消息，THEN 系统 SHALL 将其转换为 `ToolResult` 写入 `BuildContext.CurrentToolResults`。
5. IF `currentContent` 为空且有 toolResults，THEN 系统 SHALL 将 `currentContent` 设为 `"Tool results provided."`。
6. IF `currentContent` 为空且无 toolResults，THEN 系统 SHALL 将 `currentContent` 设为 `"Continue"`。

---

### 需求 7：重构 `ConvertRequest` 为流水线调度器

**用户故事：** 作为一名开发者，我希望 `ConvertRequest` 函数本身只负责编排和调度各构建器，不包含任何具体的转换逻辑，以便整体流程一目了然。

#### 验收标准

1. WHEN 调用 `ConvertRequest` 时，THEN 系统 SHALL 按顺序依次调用各构建器，并通过 `BuildContext` 传递中间状态。
2. WHEN 任意构建器标记终止时，THEN 系统 SHALL 提前返回 `nil`，不继续执行后续构建器。
3. WHEN 所有构建器执行完毕后，THEN 系统 SHALL 由一个独立的 `assembler`（或最终构建器）将 `BuildContext` 中的数据组装为最终的 `*Request` 并返回。
4. WHEN 重构完成后，THEN 系统 SHALL 保证所有现有的 `req_converter_test.go` 测试用例通过，行为与重构前完全一致。

---

### 需求 8：公共辅助函数集中管理

**用户故事：** 作为一名开发者，我希望各构建器共用的辅助函数（如 `convertImage`、`convertSchema`、`parseJSONOrString`、`deduplicateToolResults`、`getMessageText`）被集中放置在 `builder/helpers.go` 中，避免重复定义。

#### 验收标准

1. WHEN 多个构建器需要使用同一辅助函数时，THEN 系统 SHALL 将该函数定义在 `builder/helpers.go` 中，供所有构建器引用。
2. WHEN 辅助函数迁移完成后，THEN 系统 SHALL 删除 `req_converter.go` 中的重复定义。
