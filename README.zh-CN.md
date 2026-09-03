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

在 macOS 或 Linux 上安装最新的 `main` 版本；以后再次运行同一命令即可更新：

```bash
curl -fsSL https://raw.githubusercontent.com/hhe48203-ctrl/canvas-cli/main/scripts/install.sh | sh
```

脚本使用 Go 构建并验证二进制，然后原子安装到 `~/.local/bin/canvas`，不使用
`sudo`，也不修改 shell 配置。可用 `CANVAS_CLI_INSTALL_DIR` 更改安装目录，或用
`CANVAS_CLI_VERSION` 指定 tag/提交。Windows 用户请按下方步骤手动构建检出版本。

把仓库 URL 直接交给 Agent：

```text
请从 https://github.com/hhe48203-ctrl/canvas-cli 安装 Canvas CLI。
使用仓库声明的 Go 版本，把 canvas 可执行文件安装到 PATH 中用户可写的目录，
然后运行 canvas --help 验证。未经我确认不要使用 sudo 或修改 shell 配置。
Canvas Token 由我自己设置，不要让我把 Token 粘贴到聊天中。
```

如需构建当前检出版本：

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

## 本地使用日志

使用日志**默认开启**，仅保存在本机，不会上报。可以在 Agent 的运行环境中关闭：

```bash
export CANVAS_USAGE_LOG=0
```

取消设置该变量即可重新开启。每次 CLI 调用结束后追加一条 JSONL，包含参数错误
和执行失败。分页、文件传输只记录一条命令摘要；帮助和自动补全使用单独的 `kind`，
方便在统计时排除。

| 字段 | 含义 |
| --- | --- |
| `time`、`version` | UTC 完成时间；模块版本、Git 提交号，无法获取时为 `devel` |
| `kind`、`command` | `command`、`help` 或 `completion`；已注册的命令路径 |
| `operation_id` | `api invoke` / `api describe` 已识别的目录操作 ID，可用时记录 |
| `duration_ms`、`exit_code` | 命令耗时和退出码（0 或 1） |
| `error_kind` | 失败类别：`arguments`、`confirmation_required`、`configuration`、`http`、`network`、`io` 或 `execution` |
| `http_status` | 可获取的最后一次 HTTP 响应状态；之后发生网络或本地错误时，仍可能保留此状态 |

只记录这些白名单字段，不记录参数值、原始 URL、Token、文件路径、请求或响应正文、
原始错误消息；未知 operation ID 也不记录。日志失败不会改变 stdout、stderr 或退出码。
强制终止可能没有结束记录；命令成功不代表 Agent 完成了用户任务。

每天的文件名为 `YYYY-MM-DD.jsonl`，位于系统用户缓存目录：

- macOS：`~/Library/Caches/canvas-cli/logs`
- Linux：`${XDG_CACHE_HOME:-$HOME/.cache}/canvas-cli/logs`
- Windows：`%LocalAppData%\canvas-cli\logs`

Unix 上目录权限为 `0700`，文件为 `0600`。每次记录时清理过期文件，保留当前 UTC
日期及之前六天。每日文件采用 10 MiB 软上限：达到后跳过新记录，最多允许一条记录略微
超限；下一个 UTC 日期会开始新文件。关闭日志不会删除已有文件。

写入进程使用操作系统文件锁，互斥修复日志尾部并追加记录。写入失败时回滚；如果上次
写入中断留下了不完整的末条记录，下次追加前会先移除残片。无法使用文件锁时跳过日志，
命令仍正常执行。

运行命令后，可用 `jq` 汇总调用次数、失败率（0–1）和平均耗时，排除帮助与自动补全：

```bash
case "$(uname -s)" in
  Darwin) canvas_log_dir="$HOME/Library/Caches/canvas-cli/logs" ;;
  *) canvas_log_dir="${XDG_CACHE_HOME:-$HOME/.cache}/canvas-cli/logs" ;;
esac

jq -s '
  map(select(.kind == "command"))
  | group_by([.command, .operation_id])
  | map({
      command: .[0].command,
      operation_id: .[0].operation_id,
      calls: length,
      failure_rate: ((map(select(.exit_code != 0)) | length) / length),
      avg_duration_ms: ((map(.duration_ms) | add) / length)
    })
  | sort_by(-.calls)
' "$canvas_log_dir"/*.jsonl
```

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
