---
description: git commit and push
model: GLM-5
subtask: true
---

# Git Commit & Push Workflow Guide

# Git Commit & Push Workflow (Copy-Paste Ready)

## Rules

1. **Required Commit Prefixes**

    - `docs:` All changes in `packages/web`

    - `tui:` Terminal/UI interaction changes

    - `core:` Core functionality/logic changes

    - `ci:` CI/CD configuration updates

    - `ignore: `.gitignore and ignore-related configs

    - `wip:` Work in progress (unfinished changes)

2. **Commit Message Standard**

    - Write **WHY** from an **end-user perspective** (not WHAT was done)

    - Be **specific** about user-facing changes (no generic messages)

3. **Pre-Commit Enforcement (MANDATORY)**

    - **Run pre-commit checks BEFORE committing**

    - Fix all issues if checks fail

    - **DO NOT commit until pre-commit passes**

    - **NEVER skip pre-commit** (no `--no-verify` / `-n` flags)

4. **Conflict Rule**

    - **DO NOT fix conflicts**

    - Notify me immediately if conflicts occur

---

## Full Commit & Push Commands

```Bash

# 1. Run pre-commit checks (required)
pre-commit run --all-files

# 2. Fix all failed checks → re-run until all pass

# 3. Stage changes
git add .

# 4. Commit with valid prefix & message
git commit -m "prefix: specific user-facing reason/change"

# 5. Push
git push
```

---

## Valid Commit Message Examples

```Bash

# packages/web → docs: prefix
git commit -m "docs: fix document page loading lag for faster user access"

# core feature fix
git commit -m "core: resolve user data export failure to restore full functionality"

# UI improvement
git commit -m "tui: adjust button layout for better user click accuracy"
```

---

## Git Status / Diff Commands

```Bash

# Short status
git status --short

# View unstaged changes
git diff

# View staged changes
git diff --cached
```