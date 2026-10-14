# 决策笔记使用说明

本项目使用固定版本的 [write-notes-like-deepseek](../.agents/skills/write-notes-like-deepseek/UPSTREAM.md)。Skill 保存在仓库内，后续会话通过 AGENTS.md 入口读取；不修改全局 Codex 记忆或其他项目规则。

## 日常工作

1. 从 [功能地图](FEATURE_MAP.md) 和代码核对相邻页面、接口、数据与权限，明确目标；按主题搜索 `.agents/notes/` 的 proposed、implemented、rejected，不将归档当作现行依据。
2. 非平凡决策先用 [提案模板](../.agents/skills/write-notes-like-deepseek/templates/proposed.md) 写现状、复用证据、影响范围、真实备选、验收和风险。已有笔记覆盖的事实修改优先原地更新。局部机械改动免写。
3. 在用户已有授权内自主推进；不是每篇提案都要新一轮批准。重要未决取舍才询问。提案文件存在不表示功能已经实现，也不自动授权生产部署。
4. 完成后改为 implemented：Proposal 改 Decision，验收/风险改为实际验证及后果，代码与笔记同次提交。决定翻转时新建并承接旧理由；完全被取代的已实施笔记按上游流程归档。失效提案转 rejected 或删除，不归档为已实施历史。
5. 运行 `make verify-notes`。对应的产品验收、Go/前端测试仍需根据实际改动执行。

路径为 `.agents/notes/{proposed,implemented,rejected,archived}/{类别}/yyyy-mm-dd-topic.md`；类别限 feature、bug-fix、simplification、architecture、process、testing。只创建用到的目录。不维护笔记总索引；功能地图是代码导航，不逐篇罗列笔记。

## 命令（仓库根目录，Bun）

```sh
make verify-notes
bun .agents/skills/write-notes-like-deepseek/scripts/archive-agent-note.ts <旧笔记路径> --superseded-by <新笔记路径>
bun .agents/skills/write-notes-like-deepseek/scripts/build-board.ts --bundle .agents/notes /tmp/newapi-decisions.html 'new-api 工程决策'
```

归档脚本可能修改文件，执行后检查入站链接和 Git diff。看板可选，不纳入产品页面或提交生成物；打包会包含笔记内容，不要把内部决策看板默认公开。笔记不得保存密码、密钥或真实客户隐私。

## 校验边界

CI 在 main 推送和 PR 时运行三项检查：目录/链接、格式/状态、归档哈希与历史基线。归档基线分别使用 push 的 before 或 PR 的 base.sha，需完整 Git 历史。失败显示 CI 失败；是否禁止合并由仓库分支保护配置决定，本次不改变分支保护。

脚本只查结构，不会判断“相关功能是否查全”“备选是否合理”“是否漏写重要决策”。这些必须在方案和审查中提供证据。涉及共享统计时，还要核对相同过滤条件的数据一致性。不要用多写文档替代读代码。

## 跨项目与跨电脑迁移

使用[独立迁移包](../scripts/agent-workflow/README.md)进行安装、预览、冲突检查和升级。迁移包不含本项目业务笔记；已有项目规则与能力地图保留。新电脑克隆已接入的目标仓库并安装 Python/Bun 后即可校验；可选的全局 Codex Skill 仅辅助接入。
