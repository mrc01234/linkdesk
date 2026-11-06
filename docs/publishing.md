# 发布到自己的 GitHub

2026-11-06 已发布至公开仓库：**[mrc01234/linkdesk](https://github.com/mrc01234/linkdesk)**，默认分支为 `main`。本地 `portfolio` 分支跟踪 `origin/main`；上游完整历史与 MIT 许可已保留。

后续提交后，在本项目目录运行 `git push origin HEAD:main` 即可更新；先用 `git status` 和 `git diff` 确认本次内容。云端验证见 [Actions](https://github.com/mrc01234/linkdesk/actions)。

以下保留为在其他账号下新建仓库时的流程参考；当前仓库已经创建，不需要重复创建或添加 origin。

## 1. 创建空仓库

在你自己的 GitHub 账号下新建 `linkdesk`，建议 Public。不要勾选自动创建 README、LICENSE 或 .gitignore，这些文件已在本地。

建议描述：

> Go + SQLite 自托管链接工作台，支持搜索分页、有效期与二维码；基于开源短链项目二次开发。

建议 Topics：golang、sqlite、url-shortener、self-hosted、portfolio。

## 2. 推送本地分支

先在项目根目录检查：

```bash
git status
git remote -v
git log -3 --oneline
```

保留 upstream 指向原仓库，把下面的“你的用户名”替换成实际 GitHub 用户名：

```bash
git remote add origin https://github.com/你的用户名/linkdesk.git
git push -u origin portfolio:main
```

不要把新的改造推到 upstream。若已配置 origin，先检查其地址，不要盲目再添加或强制推送。

GitHub 身份验证使用你本机正常的 Git 登录流程；不要把令牌写入代码、URL 或聊天。

## 3. 发布后检查

- 默认分支为 main，README 正常显示。
- Actions 的 verify 工作流实际通过；本地测试通过不等于云端 CI 已通过。
- 仓库不含 .env、数据库、构建缓存、个人资料或真实凭据。
- LICENSE 与 UPSTREAM.md 可见。
- 克隆到新目录能按 README 启动。
- 需要 Release 时再上传经过检查的二进制分发包，并注明平台与版本。

## 4. 简历链接

使用你实际创建后的 GitHub 地址。不填不存在的在线演示地址，不写未测量的 QPS 或性能提升比例。
