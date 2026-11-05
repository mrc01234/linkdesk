# 开源来源与改造范围

LinkDesk 基于 [mstgnz/self-hosted-url-shortener](https://github.com/mstgnz/self-hosted-url-shortener) 二次开发。

- 基线提交：`e051ac63a2644e055e355f6972292729ae693577`。
- 保留 Git 历史以及原作者 Mesut GENEZ 的 MIT 版权与许可文本，见 [LICENSE](LICENSE)。
- 本地 `upstream` 指向原仓库；LinkDesk 使用独立工作目录和分支。
- Go module 名称为 `linkdesk`，支持克隆后本地构建；不是可直接 `go install github.com/某人/linkdesk@latest` 的安装包。
- 本轮改造使用 AI 辅助实现与验证；不能据此声称从零独立开发上游功能。

## 上游已有

短码生成、自定义短码、重定向、点击计数、二维码、Basic Auth、SQLite 持久化、Web/API/CLI 基础，以及对应基础测试。

## LinkDesk 新增或改造

- 数据库层搜索、分页、总数和稳定排序；Web/API 共用筛选契约。
- 到期时间、状态筛选、旧库兼容迁移；过期重定向返回 410。
- 重做中文管理页面、空结果状态、分页导航与复制交互。
- 将管理页面删除操作从 GET 改为 POST，增加跨站写请求保护。
- 自定义短码并发冲突由数据库唯一约束兜底并映射为 409。
- 点击计数同步更新；健康检查独立于管理鉴权。
- 环境变量配置、本机默认绑定、非本机绑定要求鉴权、Docker 非 root 运行。
- SQLite/HTTP 集成测试、CI、部署文档及演示说明。

本项目面向个人或小团队的单实例链接管理，未进行生产规模容量或性能认证。
