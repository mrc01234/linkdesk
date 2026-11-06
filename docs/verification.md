# 本地验证记录

本地验证日期：2026-11-05；公开发布与云端验证更新：2026-11-06。测试由编码助手及 GitHub Actions 执行，不等同于用户独立实现。

## 已通过

环境：Windows amd64，Go 1.27.0，便携 GCC，真实 SQLite 驱动。

- 上游基线 go test ./... 通过。
- 改造并更新依赖后，go test -race -count=1 ./... 通过。
- go vet ./... 通过。
- Windows 可执行文件构建通过。
- PowerShell 开发脚本语法解析通过。
- SQLite 测试：排序同时间时的 ID 顺序、跨页结果、总数、空页、字面特殊字符、大小写搜索、状态过滤、取消查询、唯一冲突。
- 旧 schema 自动迁移保留链接与点击数，新到期字段在关闭重开后保留。
- HTTP 集成测试使用真实模板与临时 SQLite：空列表、分页、非法参数、表单、API、Basic Auth、公开跳转、过期 410、过期不计数、跨站写拦截、GET 不能删除、并发短码冲突、HTML 转义。
- 表单使用 POST/Redirect/GET；两次访问结果页不新增数据。
- 已运行本地 Windows 程序并通过 HTTP 实测：存活检查、搜索分页、管理页 HTML、真实时间到期后的 410 / 0 次点击、删除。
- 停止并重启程序后，6 条演示链接保留。
- 实测表单创建返回 303，访问两次结果页后仍只新增 1 条。

## 2026-11-06 公开发布与云端验证

- 公开仓库：[mrc01234/linkdesk](https://github.com/mrc01234/linkdesk)，默认分支 main；保留上游完整 Git 历史及 MIT 许可。
- 首次 [Actions 运行 42142463593](https://github.com/mrc01234/linkdesk/actions/runs/42142463593) 结果为 success，Go 与 container 两个任务均通过。
- 本次已验证的代码提交：`8c3969ae05eb9dbab36961a946e38e7d9b96d646`。
- Ubuntu 上格式检查、带竞态检测的测试、go vet 和 Linux 二进制构建均通过。
- Docker 镜像构建通过；容器启动后公开健康检查返回成功，未认证管理请求返回 401，正确认证后能读取管理 API。
- 本机仍没有 Docker 命令；上述 Docker 结果来自 GitHub 托管 runner，不冒充 Windows 本地验证。

## 尚未验证

- 浏览器连接宿主不可用；尝试打开 Edge 的电脑控制应用审批超时。因此本轮未完成真实浏览器视觉检查、移动屏幕检查或剪贴板交互验证。HTML 渲染与页面路由已经通过测试，不能据此宣称视觉验收完成。
- 未执行生产压测；没有 QPS、性能提升比例或大规模用户数结论。
- 未进行公网 HTTPS 部署或不同主机环境的完整验收；公开 GitHub 仓库不等于已提供在线托管服务。

## 数据范围

自动测试只使用临时数据库。人工 HTTP 冒烟使用被 Git 忽略的 data/demo.db，演示内容为公开文档地址及 example.com 示例。没有读取或改动 MySQL 的 golong_study，也没有复制旧短链仓库的 study.db。
