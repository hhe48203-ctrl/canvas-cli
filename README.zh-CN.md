# Canvas CLI — 面向 AI Agent 的 Canvas LMS 命令行工具

[English](README.md) | 简体中文

[![CI](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hhe48203-ctrl/canvas-cli)](https://github.com/hhe48203-ctrl/canvas-cli/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Canvas CLI 为 AI Agent 提供一套可控的 Canvas LMS 接口。Agent 可以查询
课程和作业、下载课程文件、准备提交内容、使用用户提供的答案完成 Classic Quiz，
还可以访问 1100 多个 Canvas REST API 操作。

命令返回结构化 JSON 或 YAML。读取操作可以直接执行；上传、提交等写操作必须带
`--confirm`，因此 Agent 能在执行前先展示准确的目标和内容。

## 安装

把仓库 URL 直接交给 Agent：

```text
请从 https://github.com/hhe48203-ctrl/canvas-cli 安装 Canvas CLI。
使用仓库声明的 Go 版本，把 canvas 可执行文件安装到 PATH 中用户可写的目录，
然后运行 canvas --help 验证。未经我确认不要使用 sudo 或修改 shell 配置。
Canvas Token 由我自己设置，不要让我把 Token 粘贴到聊天中。
```

也可以手动安装：

```bash
git clone https://github.com/hhe48203-ctrl/canvas-cli.git
cd canvas-cli
go build -o canvas .
mkdir -p "$HOME/.local/bin"
install -m 0755 canvas "$HOME/.local/bin/canvas"
```

所需 Go 版本以 [`go.mod`](go.mod) 为准。

## 配置

在 Canvas 的 **Account → Settings → Approved Integrations** 创建 Access
Token，并通过环境变量保存：

```bash
export CANVAS_BASE_URL="https://canvas.example.edu"
export CANVAS_API_TOKEN="your-token"

canvas auth status
canvas auth set-url "https://canvas.example.edu"
```

CLI 不会写入 `CANVAS_API_TOKEN`。不要把 Token 放进提示词、源码、命令参数或
Git 提交。

## Agent Skill

可以添加下面这样的 `SKILL.md`，让 Agent 遵循安全流程：

```markdown
---
name: canvas-lms
description: 通过 canvas CLI 使用 Canvas LMS，处理课程、作业、文件、提交和 Quiz。
---

# Canvas LMS

处理 Canvas 任务时优先使用 `canvas`，不要操作网页。

规则：
- 第一次请求前运行 `canvas auth status`。
- 优先使用 `--json`，使用返回的 ID，不猜测 ID。
- 准备答案前先读取作业或 Quiz 详情。
- 所有写操作前展示目标和提交内容。
- 只有用户明确批准后才添加 `--confirm`。
- 不得索取或显示 `CANVAS_API_TOKEN`。

常用读取：
    canvas courses list --all-pages --json
    canvas assignments list COURSE_ID --all-pages --json
    canvas assignments show COURSE_ID ASSIGNMENT_ID --json
    canvas files list COURSE_ID --all-pages --json

用户批准后提交：
    canvas assignments submit COURSE_ID ASSIGNMENT_ID \
      --file answer.pdf --confirm --json

调用其他端点：
    canvas api search KEYWORD
    canvas api describe OPERATION_ID
    canvas api invoke OPERATION_ID --json
```

例如，可以直接对 Agent 说：

> 找到我正在上的生物课，列出本周到期的作业，并准备下一份提交。真正提交前，
> 先把准确的课程、作业和文件列表给我确认。

## 常用命令

```bash
# 课程和作业
canvas courses list --all-pages --json
canvas assignments list COURSE_ID --all-pages --json
canvas assignments show COURSE_ID ASSIGNMENT_ID --json

# 文件
canvas files list COURSE_ID --all-pages --json
canvas files download FILE_ID --destination ./lecture.pdf

# 文本、URL 或多文件提交
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --text "My response" --confirm --json
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --url "https://example.com/work" --confirm --json
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file answer.pdf --file appendix.pdf --confirm --json

# Classic Quizzes
canvas quizzes list COURSE_ID --all-pages --json
canvas quizzes start COURSE_ID QUIZ_ID --confirm --json
canvas quizzes questions SUBMISSION_ID --all-pages --json
canvas quizzes answer SUBMISSION_ID \
  --answers-file answers.json --confirm --json
canvas quizzes complete COURSE_ID QUIZ_ID SUBMISSION_ID \
  --attempt ATTEMPT --validation-token TOKEN --confirm --json
```

使用 `canvas --help` 或 `canvas <command> --help` 查看全部参数和示例。

## 完整 Canvas API

内置目录覆盖 Canvas 官方 OpenAPI 描述中的 1100 多个操作：

```bash
canvas api search modules
canvas api describe context_modules_api.create
canvas api invoke courses.list \
  --query enrollment_type=student --all-pages --json
canvas api invoke METHOD /api/v1/example \
  --query key=value --body request.json --confirm --json
```

`api describe` 会显示方法、路径和参数。`api invoke` 支持重复的 `--path`、
`--query`、`--header` 和 `--form`，也支持 `--body` 和 `--all-pages`；
写方法必须带 `--confirm`。

权威 API 说明请查看
[Instructure Developer Documentation](https://developerdocs.instructure.com/services/canvas)。

## 输出与安全

- `--json` 和 `--yaml` 返回稳定的成功或错误 envelope；
- `--all-pages` 直接跟随 Canvas 分页链接；
- 文件上传采用流式传输，并完成 Canvas 的多阶段上传流程；
- 修改数据的操作必须带 `--confirm`；
- Quiz 答案必须由用户提供，CLI 不会求解或猜测；
- Canvas 权限和学校政策仍然有效。

## 开发

```bash
go test ./...
go build ./...
scripts/update-api-catalog.sh
```

该脚本会从 Instructure 官方 API 文档更新内置目录。贡献流程见
[CONTRIBUTING.md](CONTRIBUTING.md)。

## License

[MIT](LICENSE)
