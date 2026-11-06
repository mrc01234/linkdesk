# LinkDesk · 链接工作台

一个面向个人和小团队的自托管链接管理工具：创建短链接、设置有效期、搜索与分页管理、查看访问次数和下载二维码。

基于 [mstgnz/self-hosted-url-shortener](https://github.com/mstgnz/self-hosted-url-shortener) 二次开发，保留原作者 MIT 许可与 Git 历史。新增功能及来源边界见 [UPSTREAM.md](UPSTREAM.md)。本轮实现使用 AI 辅助，适合作为能够运行、解释和继续修改的个人学习项目，不表示从零独立开发。

**验证状态（2026-11-06）：** Windows 本地测试、竞态检测、实际 HTTP 运行和源码包重测已通过；首次 [GitHub Actions](https://github.com/mrc01234/linkdesk/actions/runs/42142463593) 的 Linux 测试、构建及 Docker 鉴权运行检查均通过。浏览器视觉与交互仍待检查，详细记录见 [verification.md](docs/verification.md)。

## 功能

- Web / REST API 创建链接，支持自定义短码与二维码。
- 按短码或目标网址搜索；按全部、有效、已到期筛选。
- SQL 层分页、总数与稳定倒序；空列表返回 `[]`。
- 可选到期时间；到期返回 HTTP 410，不跳转、不累加访问次数。
- SQLite 文件持久化，自动兼容上游不含到期字段的数据库。
- Basic Auth 保护管理入口，公开短链跳转和存活检查。
- 删除使用 POST / DELETE；阻止跨站浏览器写请求。
- 中文响应式管理页面、复制链接和二维码下载。

## 快速启动

### 从源码启动

需要 Go **1.25+**（建议当前受支持版本）以及 GCC。SQLite 驱动需要 CGO。

在项目根目录：

```bash
go mod download
go run ./cmd
```

打开 **http://localhost:8080**。默认仅监听 `127.0.0.1`，首次启动会创建 `data/linkdesk.db`，无需安装 MySQL，也不会使用你的 `golong_study` 数据库。

Windows 若 GCC 不在 PATH，可指定编译器后运行：

```powershell
$env:CC = '你的 GCC 目录\bin\gcc.exe'
$env:CGO_ENABLED = '1'
go run ./cmd
```

也提供 `scripts/dev.ps1`，支持 run / test / build，并能识别本学习工作区已安装的便携 GCC：

```powershell
.\scripts\dev.ps1 -Mode run -Port 8082
```

如果执行策略阻止脚本，可使用前面的 Go 命令，不必修改全局执行策略。

### Docker Compose

```bash
cp .env.example .env
# 编辑 .env，填写 AUTH_PASSWORD，按需修改端口和 BASE_URL
docker compose up --build -d
docker compose logs -f
```

Windows 复制命令为 `Copy-Item .env.example .env`。原生 Go 不自动读取 `.env`，该文件供 Compose 使用。

Compose 将端口绑定到本机，要求配置管理账号和密码，数据保存在命名卷。停止服务：

```bash
docker compose down
```

不要为普通重启添加 `-v`，它会删除数据卷。Docker 构建和鉴权冒烟测试已写入 CI；本机是否实际验证见 [验证记录](docs/verification.md)。

## 演示流程

1. 在首页创建一个自定义短码，例如 `go-guide`，有效期选 1 小时。
2. 打开“链接管理”，搜索 `guide`，切换状态和每页条数。
3. 点击短链接，返回列表后查看访问次数。
4. 创建更多链接，查看分页；搜索不存在的词，查看空状态。
5. 重启服务，确认链接保留。到期失效、空结果、旧库迁移等边界由集成测试覆盖。
6. 删除不再需要的测试链接。

访问次数统计的是跳转请求，可能包含机器人和刷新，不等于独立用户数。

## API

本机未启用鉴权时可以直接调用。启用后使用 HTTP Basic Auth，例如 curl 的 `-u 用户名` 会提示输入密码。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/shorten` | 创建短链接 |
| GET | `/api/urls?q=&status=all&page=1&page_size=10` | 搜索分页 |
| GET | `/api/url/{code}` | 查看元数据，包含过期状态 |
| DELETE | `/api/url/{code}` | 删除，重复删除也返回成功 |
| GET | `/{code}` | 公开跳转；不存在 404，到期 410 |
| GET | `/qr/{code}` | 下载二维码，管理鉴权保护 |
| GET | `/healthz` | 公开存活检查，不代表全面数据库健康检查 |

创建：

```http
POST /api/shorten
Content-Type: application/json

{
  "url": "https://go.dev/doc/",
  "custom_code": "go-docs",
  "expires_at": null
}
```

`expires_at` 可省略、为 null，或传带时区的 RFC3339 时间（必须晚于当前时间）；以秒为精度保存。成功返回 200，包含 id、short_code、long_url、short_url、created_at、clicks 和 expires_at。输入错误 400，短码冲突 409，不支持的媒体类型 415；错误响应为纯文本，服务端故障不向客户端暴露数据库细节。

PowerShell 示例：

```powershell
$body = @{
    url = 'https://go.dev/doc/'
    custom_code = 'go-docs'
    expires_at = (Get-Date).ToUniversalTime().AddHours(1).ToString('o')
} | ConvertTo-Json

Invoke-RestMethod -Uri 'http://localhost:8080/api/shorten' -Method Post -ContentType 'application/json' -Body $body
Invoke-RestMethod -Uri 'http://localhost:8080/api/urls?q=go&page=1&page_size=10'
```

列表响应：

```json
{
  "urls": [],
  "total": 0,
  "page": 1,
  "page_size": 10,
  "total_pages": 0
}
```

- `page`：1～1,000,000；`page_size`：1～100，默认 10。
- `q`：去掉两端空白，最多 200 字节；对短码和原地址做字面子串搜索。百分号和下划线没有通配含义；ASCII 大小写不敏感，未实现完整 Unicode 大小写折叠。
- `status`：all / active / expired。
- 固定按 `created_at DESC, id DESC` 排序；同一次列表查询的总数与条目使用同一读事务。
- 页码超过末页时返回空数组，不自动跳回第一页。
- API 管理响应禁止缓存；短链 302 也禁止缓存，以便后续到期或删除生效。
- 相比上游，`/api/urls` 默认只返回第一页，并增加分页元数据，这是有意的行为变更。

## 配置与部署边界

| Flag | 环境变量 | 默认 |
| --- | --- | --- |
| --host | HOST | 127.0.0.1 |
| --port | PORT | 8080 |
| --db | DB_PATH | data/linkdesk.db |
| --base-url | BASE_URL | http://localhost:8080 |
| --username | AUTH_USERNAME | 空 |
| --password | AUTH_PASSWORD | 空 |
| --templates | — | templates |

命令行覆盖环境变量。更改端口或域名时一起更改 BASE_URL；从项目根目录启动，以找到 templates 和 static。

公开部署需要配置管理账号密码，并在反向代理处启用 HTTPS。应用拒绝在无管理账号密码时绑定非回环地址。Basic Auth 沿用上游设计，属于单管理员管理方式；没有注册、租户隔离、密码找回或开放注册服务的防滥用体系。

请勿直接开放成匿名短链平台。搜索为子串扫描，大页码使用 OFFSET；当前目标是小规模单实例，未声称高并发或百万数据性能。SQLite 文件应放在本地磁盘；备份前先停止应用，再复制数据库及仍存在的 WAL/SHM 伴随文件。不要在运行中只复制主 db 文件当作可靠备份。

## 开发与测试

```bash
go test -race -count=1 ./...
go vet ./...
go build -trimpath -o dist/linkdesk ./cmd
```

Windows 构建输出名改为 `dist/linkdesk.exe`。测试使用临时 SQLite 数据库，不改演示数据。

```text
cmd/          启动、配置、HTTP 生命周期
handler/      HTTP / CLI、输入校验、响应和模板
service/      URL 校验、短码与有效期规则
database/     SQLite、兼容迁移、分页查询
model/        链接、分页与错误类型
templates/    中文服务端渲染页面
static/       CSS 与复制交互
docs/         架构、验证、发布与简历说明
```

- [架构与面试讲解](docs/architecture.md)
- [验证记录与限制](docs/verification.md)
- [发布到 GitHub](docs/publishing.md)
- [项目经历表述与补学清单](docs/portfolio.md)

## 许可

MIT。原作者版权声明完整保留，见 [LICENSE](LICENSE)。
