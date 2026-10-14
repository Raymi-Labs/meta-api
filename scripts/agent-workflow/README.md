# 可迁移的项目工作流 1.0.0

将“先核对已有能力 → 记录真实决策 → 实现与验收 → 自动校验”带到其他项目、其他电脑。
分发包仅含工作流、模板与工具，不含 new-api 的业务笔记、项目能力地图、账号或机器绝对路径。

## 推荐：安装到项目

下载 Release 的 ZIP，解压后保留完整 `project-workflow` 目录。需要 Python 3.9+；校验还需要 Bun（CI 固定 1.3.14）。安装与打包仅使用 Python 标准库，不联网下载依赖。

```sh
# 第一次先预览；路径含空格时加引号
python3 project-workflow/workflow.py install "/path/to/project"
# 确认计划后应用；--ci 可选，生成独立 GitHub Actions
python3 project-workflow/workflow.py install "/path/to/project" --apply --ci
cd "/path/to/project"
python3 .agents/workflow.py doctor
python3 .agents/workflow.py verify
```

Windows 使用相同命令，把 `python3` 换成 `python`，路径换成实际项目路径。
安装器向 AGENTS.md 追加独立标记块，不替换原规则；地图和流程说明仅在不存在时创建。需要 Agent 根据代码补全地图，安装成功不代表能力审计完成。
已有 ADR、自定义笔记树或类似规则的项目，应先由 Agent 检查并整合，不盲目重复安装。

将安装产生的文件连同真实业务笔记提交到目标仓库。之后在新电脑克隆仓库、安装 Python 与 Bun，即可继续使用，不依赖原电脑的全局配置。若文件被项目忽略，审阅后只对本次需要的文件使用 `git add -f`。

## 可选：作为 Codex Skill 随身携带

将解压后的完整 `project-workflow` 文件夹放到新电脑的 Codex skills 目录（通常为 `~/.codex/skills/`；Windows 通常在用户目录下的 `.codex/skills/`，设置了 CODEX_HOME 时使用其 skills 子目录）。不要只复制 SKILL.md；已有同名目录时先比较，勿覆盖自定义内容。重新打开会话后可说：

> 使用 project-workflow，把这套能力核对、决策笔记和自动校验接入当前项目，保留原有规则，按真实代码补全功能地图。

全局 Skill 用于辅助接入；日常工作依靠项目内 AGENTS.md 和固定版本的笔记 Skill。

## 升级与冲突

下载新包，重复 install 预览及 --apply。安装清单 `.agents/workflow-install.json` 保存版本与托管文件 SHA256。
托管文件与上次安装一致时才能更新；本地自定义冲突会使整个预检失败，不写任何文件。请比较新旧内容，人工整合后再决定是否接纳新版，安装器不提供强制覆盖开关。首次安装遇到不同的已有工具文件也同样拒绝；相同文件可接纳。
项目自己的笔记、地图、说明不归升级器覆盖。保留安装清单；删除它会失去自动识别旧版文件的能力。
升级器不会自动删除新版中移除的旧文件。发生磁盘写入失败可能留下部分安装，处理后可重试；建议安装前保持可恢复的 Git 工作区。

## 构建与验证

在来源仓库运行：

```sh
python3 -m unittest discover -s scripts/agent-workflow/tests -v
python3 scripts/agent-workflow/build.py /tmp/agent-workflow-dist
```

ZIP 与 SHA256SUMS.txt 是最终交付物；独立标签为 `agent-workflow-v<版本>`，不会触发主应用二进制/桌面/ARM 镜像发布。CI 在 Linux、macOS、Windows 验证后发布附件，不改变应用版本或生产部署。
复用的笔记工具来源及许可说明保存在 payload/write-notes-like-deepseek/UPSTREAM.md。保留上游归属；该固定提交没有独立 LICENSE，不补造许可文本。
