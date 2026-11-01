# Agent Note: Electron发布适配公司Actions规则

Status: implemented

## Problem

Electron在v1.12.4至v1.12.7连续startup_failure，尚未分配Windows构建机。只读组织/仓库API确认要求完整Actions SHA、仅允许指定来源；工作流使用浮动版本且softprops/action-gh-release不在允许名单。原门禁只检查Electron的Go版本，遗漏来源约束。

## Decision

复用[公司Actions策略](../../implemented/process/2026-10-27-company-actions-policy.md)，将Electron所有Actions固定获准源的完整SHA，以gh补传桌面附件并纳入来源与语法校验；不修改组织规则。保留Windows NSIS与portable两种产物及现有应用标识，不启用macOS/Linux桌面构建。

手动补包必须指定正式tag，源码VERSION、HEAD和tag提交一致；不移动历史标签。构建验证内嵌后端和ASAR版本及两个非空exe，附SHA256。并发发布时等待Linux正式Release及双架构服务端附件完整，只上传缺失桌面文件，不创建/修改Release或覆盖已有资产。上传失败保留Actions artifact供重试。

## Existing capability review

FEATURE_MAP公司发布入口 → electron-build.yml → Windows前端/Go/Electron打包 → Actions artifact → GitHub Release。服务端release.yml已采用gh CLI和缺失附件补传，optional流程已有不可变tag核对；Electron复用这些契约。electron/package.json保留原产品名、上游署名、运行时与打包目标；Node构建工具选择22，GitHub Action运行时统一Node24。既有npm install用于与package.json一致的依赖解析，不声称当前历史lock已与manifest匹配。

[Linux优先发布](../../implemented/process/2026-10-21-linux-first-release.md)与[公司Actions策略](../../implemented/process/2026-10-27-company-actions-policy.md)部分重叠，保留其发布/生产边界，补齐此前明确未迁移的Electron流程。无其他活跃Electron决策归属。

## Alternatives considered

- 放宽组织策略可让旧流程启动，但改变全组织安全边界；沿用公司已有获准SHA与gh路径。
- 只替换SHA可解决部分启动拒绝，但softprops仍未获准，手动构建不上传、并发创建Release还会与Linux竞争；同时接入只补充附件的上传步骤。
- 删除Electron触发能消除失败记录，但无法交付用户要求的桌面程序，不采用。

## Verification

- 发布策略检查和22项附件回归通过，包括校验和、缺失exe、完整附件保留、损坏/重复资产拒绝、部分补传与既有资产哈希不一致时拒绝混用、Linux未就绪及网络失败不写入。
- actionlint v1.7.12（本地已有二进制）验证Electron与发布门禁工作流通过；Go在线启动同版本工具因proxy.golang.org超时失败，使用现有同版本工具完成检查。
- CI启用ShellCheck后发现旧npm version命令的SC2086，补齐版本参数引用；本地actionlint环境没有ShellCheck，该差异由真实CI发现并修复。
- Windows首次实际构建通过（run 41309647233），上传阶段发现Windows GNU sha256sum二进制标记与原逐字文本比对不兼容；改为解析标准记录、按文件名/真实哈希核对，覆盖二进制标记、CRLF、顺序及重复/缺失/非法记录。最终完整上传仍需新运行验收。
- 笔记、diff检查通过。Windows实际构建和附件上传由本次Actions验收，不能以本地脚本通过代替Windows结果。

## Consequences

组织保护保持不变；Windows构建、版本核对和上传流程可重试且不会重写既有Release。等待Linux产物会增加上传作业时长，超时明确失败并保留Actions artifact。未新增签名凭据，安装包沿用既有无签名配置；真实个人Windows安装与生产部署不由CI成功代替。
