---
description: git commit and push
model: GLM-5
subtask: true
---

# Git Commit & Push Workflow Guide


## 规范

1. **提交记录必须添加前缀**

- `docs:` 存放于 `packages/web` 目录的所有修改


- `core:` 核心功能、业务逻辑相关改动

- `ci:` 持续集成/持续部署配置更新

- `ignore:` .gitignore 文件及忽略规则相关配置修改

- `wip:` 未完成的临时开发改动

1. **提交说明文案规范**

- 从**终端用户视角**写清楚改动原因（不要只写做了什么操作）

- 精准描述对用户可见的变更内容，禁止笼统模糊文案

1. **提交前强制校验（必须执行）**
需要严格按照[commit验收标准](../../../docs/COMMIT_ACCEPTANCE.md)来做提交前的验收，若验收不通过，严禁提交，并输出验收不通过的原因和验收检测明细数据

1. **冲突处理规则**

- **禁止自行解决代码冲突**

- 若出现代码冲突，立刻告知我

---

## 完整提交&推送命令

```Bash

# 1. 运行预提交全量文件校验（必执行）
pre-commit run --all-files

# 2. 修复所有校验报错 → 重复运行，直至全部校验通过

# 3. 暂存所有改动文件
git add .

# 4. 按规范前缀+说明提交代码
git commit -m "前缀: 面向用户的具体改动原因/优化点"

# 5. 推送代码到远程仓库
git push
```

---

## 合规提交文案示例

```Bash

# packages/web 目录改动 → 使用 docs 前缀
git commit -m "docs: 优化文档页面加载卡顿问题，提升用户访问速度"

# 核心功能修复
git commit -m "core: 修复用户数据导出失败问题，恢复完整导出功能"

# 界面交互优化
git commit -m "tui: 调整按钮布局，提升用户点击操作精准度"
```

---

## 仓库状态 & 差异查看命令

```Bash

# 简洁查看文件改动状态
git status --short

# 查看未暂存的代码差异
git diff

# 查看已暂存待提交的代码差异
git diff --cached
```