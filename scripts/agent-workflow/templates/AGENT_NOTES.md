# 决策笔记使用说明

先从 FEATURE_MAP.md 定位并核对代码，再检索 `.agents/notes/` 中除 archived 外的相关笔记。
非平凡改动遵循仓库内 `.agents/skills/write-notes-like-deepseek/SKILL.md`，已有归属优先更新；用户及项目授权高于 Skill 默认确认步骤。

笔记路径为 `.agents/notes/{proposed,implemented,rejected,archived}/{类别}/yyyy-mm-dd-topic.md`。
类别为 feature、bug-fix、simplification、architecture、process、testing。仅创建用到的目录。
新决策先 proposed，实施并验证后改为 implemented，正文改为当前事实，与代码一起提交；机械改动免写。

在项目根目录执行 `python3 .agents/workflow.py verify`。Windows 可使用 `python` 替代 `python3`。
需要 Python 3.9+ 和 Bun；无需安装 npm 依赖。`doctor` 检查运行环境与托管文件完整性。
CI 如需禁止合并，还需仓库分支保护规则；结构检查不判断业务方案正确，也不能证明所有重要决策都已记录。

新电脑克隆仓库即可获得规则、模板及校验器。项目业务笔记、AGENTS.md 和本说明应随仓库提交，不需要全局 Skill。
升级从新版项目能力核对与决策记录工具包运行 `workflow.py install <项目目录>` 预览，再加 `--apply`。安装器拒绝覆盖本地修改；能力地图和本说明始终保留。
敏感信息不应写入笔记；部署、提交、发布遵循本项目授权。
