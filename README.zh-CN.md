# 大学课程 Canvas CLI

[English](README.md) | 简体中文

[![CI](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/hhe48203-ctrl/canvas-cli/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hhe48203-ctrl/canvas-cli)](https://github.com/hhe48203-ctrl/canvas-cli/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

面向大学及高等教育课程的 Canvas LMS 命令行客户端，服务于学生、教师、助教和 AI Agent，提供常用课程工作流、稳定的结构化输出，以及可搜索的完整 Canvas REST API 操作目录。

> [!IMPORTANT]
> 本项目处于早期开发阶段，是面向大学课程使用场景的独立社区项目，不由 Instructure 官方开发或支持。执行提交作业、开始或完成 Quiz 等写操作前，请核对目标 Canvas LMS 实例、课程和参数。

这是一个用 Go 编写、面向大学 Canvas LMS 课程的命令行客户端，主要服务于两类用户：

- 学生、教师和助教：用短命令完成课程、作业、课程文件和 Quiz 工作流；
- AI Agent：用稳定的 JSON/YAML 输出、统一错误格式和 `api invoke` 调用 Canvas 接口。

Canvas 官方 API 文档位于 [Instructure Developer Documentation](https://developerdocs.instructure.com/services/canvas)。API 使用 OAuth2 / Access Token，CLI 通过 `Authorization: Bearer <token>` 认证，具体认证说明见 [OAuth2 文档](https://developerdocs.instructure.com/services/canvas/oauth2/file.oauth)。

## 可以用来做什么？

Canvas CLI 可以把日常 Canvas 操作带到终端：查看课程和作业、下载课程文件、提交作业、处理 Classic Quiz 工作流，或通过可搜索的 API 目录调用不常用的接口。相比反复操作网页，命令更方便复制、编写脚本、与 `jq` 等工具组合，也能在本地终端、自动化任务和 AI Agent 中复用同一套流程。还可以把常用命令、安全规则和学校特定流程整理成 Codex、Claude Code 或其他 Agent 的 skill：Agent 读取结构化数据并准备操作，写操作则继续通过 `--confirm` 由用户明确授权。

## 功能亮点

- 大学课程、作业、课程文件和 Classic Quizzes 的高层命令；
- 1100+ 个随发行版生成的 Canvas REST API 操作，可搜索、描述和调用；
- 自动跟随 Canvas `Link` header 获取全部分页结果；
- JSON、YAML 和终端表格输出，成功与错误均有稳定 envelope；
- Canvas 三阶段文件上传流程，包括带认证的完成确认；
- 所有写操作要求显式 `--confirm`；
- 每一级命令都提供 `--help`、参数说明和可复制示例。

## 安装

需要 Go 1.26 或更高版本：

```bash
git clone https://github.com/hhe48203-ctrl/canvas-cli.git
cd canvas-cli
go build -o canvas .
sudo install -m 0755 canvas /usr/local/bin/canvas
```

如果不希望写入系统目录，可以把构建产物放到自己的 `PATH`：

```bash
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/canvas" .
canvas --help
```

开发时也可以直接运行：

```bash
go run . --help
```

## 配置

推荐使用环境变量：

```bash
export CANVAS_BASE_URL="https://你的学校.instructure.com"
export CANVAS_API_TOKEN="你的 Access Token"
```

也可以保存 Canvas 地址：

```bash
canvas auth set-url https://你的学校.instructure.com
```

Token 不会写入配置文件；请不要把 Token 放进命令行参数、URL、Git 仓库或日志。Canvas 官方将 Access Token 视为等同于密码的凭据。

建议只授予当前工作所需的最小权限，并在 Token 泄露时立即到 Canvas 账户设置中撤销。

检查认证：

```bash
canvas auth status
canvas me
```

## 常用命令

### 课程和作业

```bash
canvas courses list
canvas courses list --all-pages
canvas courses list --query enrollment_type=student --query 'include[]=term'
canvas courses show COURSE_ID
canvas assignments list COURSE_ID --all-pages
canvas assignments show COURSE_ID ASSIGNMENT_ID
```

### 文件

```bash
canvas files list COURSE_ID
canvas files download FILE_ID --destination ./lecture.pdf
canvas files upload COURSE_ID ./notes.pdf --confirm
```

### 提交作业

写操作必须显式确认：

```bash
canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --file homework.pdf --confirm

canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --text "作业内容" --confirm

canvas assignments submit COURSE_ID ASSIGNMENT_ID \
  --url https://example.com/report --confirm
```

文件型作业会先执行 Canvas 的上传流程，再把返回的 file ID 提交给作业。相关限制和参数见 [Submissions API](https://developerdocs.instructure.com/services/canvas/resources/submissions) 与 [File Uploads](https://developerdocs.instructure.com/services/canvas/basics/file.file_uploads)。

### Quiz

当前命令覆盖经典 Canvas Quiz：

```bash
canvas quizzes list COURSE_ID
canvas quizzes show COURSE_ID QUIZ_ID
canvas quizzes start COURSE_ID QUIZ_ID --confirm
canvas quizzes questions SUBMISSION_ID
canvas quizzes answer SUBMISSION_ID --answers-file answers.json --confirm
canvas quizzes complete COURSE_ID QUIZ_ID SUBMISSION_ID \
  --attempt 1 --validation-token TOKEN --confirm
```

答案文件格式：

```json
{
  "attempt": 1,
  "validation_token": "token-from-start-response",
  "quiz_questions": [
    {"id": 101, "answer": "A"},
    {"id": 102, "answer": ["choice-1", "choice-3"]}
  ]
}
```

Canvas 的 Quiz 流程是创建答题会话、获取问题、提交答案、完成会话。参考 [Quiz Submissions API](https://developerdocs.instructure.com/services/canvas/resources/quiz_submissions) 和 [Quiz Submission Questions API](https://developerdocs.instructure.com/services/canvas/resources/quiz_submission_questions)。New Quizzes 使用独立 API，未封装的接口可以通过通用调用命令访问。

## 通用 API 调用

高层命令只覆盖常用工作流；随二进制附带的 API 目录覆盖生成时官方文档中的全部 REST endpoint。先搜索、查看参数，再调用：

```bash
canvas api search modules
canvas api describe context_modules_api.create
canvas api list --json
```

任意 Canvas endpoint 也可以直接用 method 和 path 调用：

```bash
canvas api invoke GET /api/v1/courses

canvas api invoke GET /api/v1/courses \
  --query enrollment_type=student

# 自动跟随 Canvas 的 opaque Link header，合并所有 JSON array 页面
canvas api invoke GET /api/v1/courses --all-pages

canvas api invoke GET /api/v1/courses/{course_id} \
  --path course_id=123
```

高层别名使用稳定的 operation ID；完整目录中的 ID 来自 Canvas 官方 controller/operation 名称：

```bash
canvas api list
canvas api describe courses.list
canvas api invoke courses.list --query enrollment_type=student
```

Canvas 常规 POST/PUT 参数通常使用 form encoding，可以直接重复传入 `--form`：

```bash
canvas api invoke POST /api/v1/courses/123/pages \
  --form 'wiki_page[title]=Overview' \
  --form 'wiki_page[body]=<p>Hello</p>' \
  --confirm
```

JSON 或其他已经编码的请求体可从文件或 stdin 读取：

```bash
canvas api invoke PUT /api/v1/courses/123 \
  --body request.json \
  --content-type application/json \
  --confirm

printf '%s' '{"query":"{ course(id: \"123\") { name } }"}' | \
  canvas api invoke POST /api/graphql --body - --confirm
```

其他通用参数：

- `--path name=value`：替换 operation path placeholder；
- `--query name=value`：添加 query 参数，可重复用于数组；
- `--form name=value`：添加 Canvas bracket-style 表单字段，可重复；
- `--header name=value`：添加请求头；
- `--all-pages`：对返回 JSON array 的 GET 自动取回全部页面；
- `--include-headers`：输出 HTTP status、headers、页数和 data。

通用命令的意义是：不必为 Canvas 的每个 endpoint 都手写一层 CLI。OpenAPI 是 API 的机器可读说明书，可用于生成 Go 客户端、参数帮助、请求/响应模型和测试；高层命令则负责把多个 API 请求组合成更适合人的工作流。Canvas 官方文档说明其 API 文档可以生成 OpenAPI 3.0 规范。

仓库随附从 Canvas 官方在线 API 文档生成的完整 operation 目录。更新目录：

```bash
scripts/update-api-catalog.sh
go test ./...
```

脚本下载官方 `all_resources.html`，转换为 OpenAPI 元数据，再生成 `internal/api/generated.go`。也可以使用 Canvas 源码通过 `bundle exec rake doc:openapi` 生成更丰富的官方 OpenAPI，然后执行：

```bash
scripts/generate-openapi.sh path/to/canvas.openapi.yaml
go test ./...
```

生成器保留 operation 分组、描述、path/query/body 参数、必填状态、类型、枚举、默认值、content type、response 和认证 scope。path-level 参数与 `components/parameters`、`components/requestBodies` 引用也会被解析。若目录被有意清空，CLI 仍保留少量稳定别名，并始终支持直接传入 HTTP method 和 path。

> “完整”指目录生成时 Canvas LMS 官方 REST API 文档中的 endpoint。Canvas Studio、Data Access Platform、New Quizzes 等独立产品拥有不同的 base URL、认证和版本生命周期，不应被误称为 Canvas LMS REST API；它们仍可在合适的 `--base-url` 下使用原始 method/path 调用。

## 命令帮助

每一级命令都有用途说明和可复制示例：

```bash
canvas --help
canvas api --help
canvas api invoke --help
canvas api search modules
canvas api describe context_modules_api.create
canvas assignments submit --help
canvas quizzes answer --help
canvas files download --help
```

`api describe` 的终端输出会列出参数位置、是否必填、类型、默认值或枚举、描述、request body、认证 scope、response 和一条可执行调用骨架；`--json`/`--yaml` 可获得同样信息的结构化形式。

## 输出和 Agent 使用

输出格式：

```bash
canvas courses list --json
canvas courses list --yaml
canvas courses list --output table
```

规则：

- 交互终端默认使用表格风格输出；
- 管道或重定向时默认输出 JSON；
- `--json`、`--yaml`、`--output` 可以显式覆盖默认值；
- 成功结果使用 `{ "ok": true, "data": ... }` envelope；
- 在 JSON/YAML 模式下，失败结果使用 `{ "ok": false, "error": ... }` envelope 并写入 stderr；
- 写操作必须使用 `--confirm`，适合 Agent 在执行前进行显式授权检查。

例如：

```bash
canvas courses list --json | jq '.data[] | {id, name}'
```

## 项目结构

```text
.
├── cmd/                 # Cobra 命令和高层工作流
├── internal/canvas/     # HTTP、认证 Header、JSON/Form、文件上传
├── internal/api/        # operation ID 和 endpoint 元数据
├── internal/config/     # Canvas 地址和 Token 配置
├── internal/output/     # JSON/YAML/表格和统一 envelope
├── scripts/             # API 目录生成脚本
├── tools/               # Canvas 文档与 OpenAPI 生成器
├── .github/workflows/   # GitHub Actions CI
├── main.go
└── README.md
```

## 开发和测试

```bash
go test ./...
go vet ./...
go build ./...
go run . --help
go run . api invoke --help
```

测试使用本地 `httptest` Mock Server 验证：

- Bearer Token 和 query 参数；
- HTTP 错误转换；
- Canvas 多阶段文件上传；
- CLI 编译和命令帮助。

默认不会连接真实 Canvas，也不会提交真实作业或完成真实 Quiz。

## 参与贡献

欢迎提交 issue 和 pull request。开发环境、验证命令与 API 目录更新流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

提交日志、测试 fixture 和问题描述中不得包含真实 Access Token、学生数据、课程内容或其他受保护信息。

## 许可证

本项目采用 [MIT License](LICENSE)。Canvas、Canvas LMS 和 Instructure 是其各自权利人的商标；本项目与 Instructure 无隶属或背书关系。

## 设计边界

- 本项目面向大学及高等教育机构使用的 Canvas LMS 课程环境；
- CLI 调用的是 Canvas 官方 REST API，但具体可访问内容仍由账号角色、Token 和课程权限决定；
- 不实现绕过访问码、IP 限制、时间限制或尝试次数限制的功能；
- Quiz 命令只传输用户或 Agent 明确提供的答案，不自动生成或猜测答案；
- Canvas OpenAPI 负责接口层覆盖，高层命令只为高频业务流程提供更好的用户体验。
