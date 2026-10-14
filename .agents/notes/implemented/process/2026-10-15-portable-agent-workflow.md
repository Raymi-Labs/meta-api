# Agent Note: 项目能力核对与决策记录工具包

Status: implemented

## Problem

当前流程依赖手工复制 Skill、合并规则和适配 CI，容易漏文件或覆盖目标项目约定。

## Decision

复用固定版本的笔记 Skill，增加 Python 标准库安装器、可独立分发的 ZIP 和 Codex 入口 Skill。安装先预览；应用前检查全部冲突，升级仅覆盖与上次安装哈希一致的托管文件。项目自己的能力地图和笔记不随包升级覆盖。通过仓库提交使流程随项目跨电脑迁移。

## Existing capabilities and impact

现有能力见[项目流程](2026-09-29-project-decision-notes.md)。既有笔记与本方案部分重叠，保留并互链；本方案只管理迁移和分发，不替换现有项目流程。新增 scripts/agent-workflow 和独立 CI，主应用发布排除工具包标签。对外名称和分发标识为 project-decision-kit，项目内安装清单、规则标记及校验入口保持兼容。

## Alternatives considered

- 仅全局安装 Skill：配置简便，但其他电脑和协作者不能随仓库获得规则与校验，因此项目安装为主要入口，全局 Skill 作为辅助。
- 手工复制全部目录：无需安装器，但易携带业务笔记并覆盖规则，因此只打包模板及工具，预检冲突。

## Testing

本地 7 项回归通过：空项目、重复安装、保留原规则及地图、托管文件升级/冲突拒绝、路径含空格、ZIP 自包含、校验失败传递和可重复构建。CI 配置覆盖 Linux/macOS/Windows，远程结果以实际运行记录为准。包只包含白名单工具和模板。

## Consequences

自包含包让项目和电脑间迁移不再依赖来源仓库，但增加工具版本及安装清单维护。Python 与 Bun 需事先安装；结构校验不证明业务正确。已有自定义 ADR 和规则需要 Agent 按项目适配，安装器不能推断业务入口。
