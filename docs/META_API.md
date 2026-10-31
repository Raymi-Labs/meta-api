# Meta API · Raymi Labs

Meta API 是 Raymi Labs 维护的大模型 API 网关，基于 QuantumNous/new-api。公司仓库使用 `meta-api` 名称，保留上游项目的署名、许可证、Go 模块路径及现有程序名。

## 仓库入口

| 用途 | 地址 |
|---|---|
| 公司主仓库与后续提交 | https://github.com/Raymi-Labs/meta-api |
| 公司版本发布 | https://github.com/Raymi-Labs/meta-api/releases |
| 原个人仓库与历史 Release | https://github.com/nanashiwang/new-api |
| 上游项目 | https://github.com/QuantumNous/new-api |
| 在线服务 | https://meta-api.vip |
| 用户文档 | https://docs.meta-api.vip |

公司仓库从原个人仓库复制已推送的分支、标签和完整 Git 历史；原仓库继续保留。Issues、历史 Release 页面及附件不会随 Git 推送自动复制，查阅原版本时使用原仓库。后续常规应用发布采用正式语义化版本 `vX.Y.Z`，结束新增 `v1.0.0-compat.N` 的命名方式。历史 compat 标签、Release 和产物保留，不复用旧标签发布不同源码；递增规则见 [版本号约定](installation/release-artifacts.md#版本号约定)。

## 本地目录与 Git

新克隆使用：

```sh
git clone https://github.com/Raymi-Labs/meta-api.git
cd meta-api
git remote add upstream https://github.com/QuantumNous/new-api.git
```

已有主工作区的目录改为 `meta-api`，旧 `newapi` 路径保留指向它的兼容符号链接，供现有会话和工具继续访问。关联工作树通过 `git worktree repair` 修复；不要直接删除 `.git/worktrees`。

已有工作区的 `origin` 指向公司仓库，`legacy` 保留原个人仓库，`upstream` 仍指向上游。`main` 跟踪 `origin/main`；其他既有本地分支保留原来的跟踪目标，发布前需核对 `git remote -v` 与 `git branch -vv`。本地未提交文件不随仓库迁移上传。

## 发布与部署

公司 main 通过 PR 合入。组织要求 Actions 使用完整提交 SHA 且来源受限；常规发布使用获准的 checkout/setup-go/setup-bun 与执行器自带 Docker、gh CLI，ARM64 镜像在原生执行器构建。历史标签、nightly/alpha/Electron 和独立工具包流程未在公司仓库重新适配；不能据此认定其可运行。原仓库继续提供历史产物。

复用 [现有发布流程](installation/release-artifacts.md)：应用标签构建 Linux AMD64/ARM64 程序、独立文档压缩包及其校验文件，并生成双架构镜像。公司镜像地址为：

```text
ghcr.io/raymi-labs/meta-api:<版本标签>
ghcr.io/raymi-labs/meta-api:sha-<提交前七位>
```

GHCR 地址由 `GITHUB_REPOSITORY` 派生；镜像构建和清单发布作业显式使用 `packages: write`。公司仓库使用自身的 `GITHUB_TOKEN`；未配置 Docker Hub 凭据时仅发布到 GHCR。不要复制个人凭据或为公司仓库配置上游 Docker Hub 发布凭据。

现有程序和 Release 附件仍使用 `new-api-*` 名称，Go 模块仍为 `github.com/QuantumNous/new-api`。服务端名称由既有 `SystemName` 配置提供；本次仓库命名不修改生产后台设置、页面品牌或线上数据。

公司仓库发布不会自动切换生产镜像来源。生产的更新、回退和恢复继续遵循 [受保护入口](installation/one-click-update.md)，公司服务器另行安装 `meta-api` 管理命令并通过 `/etc/meta-api/release.conf` 将更新源指向 `ghcr.io/raymi-labs/meta-api` 与 `Raymi-Labs/meta-api`；镜像现已公开。安装工具与切换来源不会升级正在运行的应用，状态、菜单及快捷命令见上述运维说明。

公司服务器实际部署目录统一为 `/opt/meta-api`，历史 `/opt/newapi` 保留兼容链接；数据不复制、容器与网络身份不改名。迁移须先安装对应的归档兼容脚本并验证连续备份，详见 [部署目录](installation/deployment-directory.md)。
