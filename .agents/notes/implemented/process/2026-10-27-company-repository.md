# Agent Note: Meta API 公司仓库与本地路径迁移

Status: implemented

## Problem

公司维护入口需要使用 Raymi-Labs/meta-api，本地主目录需要使用 meta-api。现有个人仓库同时承载历史 Release、已发布镜像和生产更新来源，主目录还持有多个关联工作树的 Git 数据。简单转移仓库或直接改路径会改变这些既有使用关系。

## Decision

- 按用户选择新建公开公司仓库，复制原远端已有分支和标签及其历史；原个人仓库继续存在，历史 Release 留在原地址。
- README 增加 Meta API / Raymi Labs 维护入口，保留原有 New API、QuantumNous 署名、许可证、模块路径、二进制命名和正文。运行时品牌仍使用既有 SystemName 配置。
- 复用仓库名派生 GHCR 地址的既有发布流程，继续 compat 版本序列；显式赋予多架构清单作业 packages: write，避免依赖原仓库默认令牌权限。
- 本地主目录改为 meta-api，旧 newapi 路径保留兼容符号链接，修复现存工作树的 Git 指针。origin 指向公司仓库，legacy 保留个人仓库，upstream 保留原上游；其他本地分支的既有跟踪关系保留。
- 历史导入期间仅关闭新公司仓库 Actions，避免批量重建历史版本；导入后恢复 Actions，再发布公司首个版本。生产镜像来源和服务不在迁移范围内。

## Related notes audit

- [Linux 优先发布](../../implemented/process/2026-10-21-linux-first-release.md)：部分重叠，继续管理构建矩阵与附件；本文只补充公司仓库的归属及权限。
- [受保护发布入口](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：部分重叠，生产入口保持独立，本轮不切换线上更新源。
- 首页、企业导航、邀请品牌相关笔记只管理运行时页面，本轮无页面代码调整，无需改写。

## Alternatives considered

- 转移原仓库：可保留 Issues、Releases 和 GitHub 重定向，但用户明确选择新建公司仓库并保留原仓库。
- 全量替换上游标识：命名表面统一，但违反现有项目保护约定，并影响模块、程序名和部署契约；采用新增公司维护入口。
- 直接移动且删除旧路径：目录最简洁，但现有会话和工具引用旧路径；保留兼容链接，并实际修复 Git 工作树。

## Verification

- 公司仓库导入 22 个已推送分支和 101 个原远端标签；导入前逐个核对本地标签与原远端 SHA；导入后对比两仓库全部 201 条分支/标签及剥离标签引用，完全一致。
- 五份 README 的原始内容逐字节保留；LICENSE、go.mod、web/package.json 未变。
- 发布策略、13 项发布脚本回归、四个发布相关工作流的 actionlint、make verify-notes 与 diff 检查通过。公司首版构建及产物以对应 Release 和 Actions 的实际结果为准。
- 主目录已改为 meta-api，旧 newapi 是兼容链接；7 个现存关联工作树完成修复，并验证全部 git-common-dir 指向新主目录。既有失效临时工作树记录未清理。

## Consequences

公司仓库独立承接后续提交、版本和镜像，原个人仓库保留历史使用关系。公司首版继续使用 compat 序列；应用运行行为不变。新仓库不自动继承个人仓库的 Secrets、Issues、Release 页面和包权限。首次构建没有缓存，正式发布必须核实 Linux 附件和镜像完成；旧目录兼容链接需保留到相关工具迁移完成。
