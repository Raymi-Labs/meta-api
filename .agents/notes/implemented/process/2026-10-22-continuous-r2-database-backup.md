# Agent Note: 复用既有 R2 的周期数据库备份

Status: implemented

## Problem

生产已有 rclone 的 Cloudflare r2 连接、独立凭据文件和 newapi-prod-backup 桶。2026-10-22 实查整个桶只有2026-09-04的一份约1.43GiB数据库转储及校验文件，未找到周期任务；旧脚本仅比较上传后大小。用户明确要求复用现有桶与连接，持续更新备份。生产磁盘仅约10GiB可用，三份部署备份及 LAST_BACKUP_DIR 继续受保护。

## Decision

后续 [MySQL PITR](2026-10-23-mysql-pitr.md) 的 opt-in 路径承接快照坐标、DDL 保护和连续归档；本篇默认旧模式仍保留作回退，其不采集坐标的边界不变。

独立 root 运维入口以每日北京时间04:10的 systemd timer 触发 MySQL 逻辑全量备份。复用既有 r2 后端及 AWS 环境凭据，在同桶专用 encrypted 前缀增加 crypt 层；随机密码、salt及恢复配置在用户 Mac 私有目录独立保存，秘密不进入仓库、日志或回执。既有明文备份与所有部署备份保持不变，不自动删除远端历史。

MySQL引擎检查及一致性dump期间共享发布锁；mysqldump、gzip、rclone rcat流式连接，不在本机落大型临时dump。网络背压可能延长快照及发布锁，设硬时限并观察首轮业务健康；不把客户端CPU限制描述为数据库扫描限速。每次使用唯一run目录，任一进程失败均不得发布完成标记；失败不覆盖以前成功备份。

上传后解密读回完整对象，验证字节数、SHA-256及gzip完整性；manifest读回通过后最后上传并核验SUCCESS标记，原子记录latest-success与各轮状态。下载验证阶段释放发布锁。失败以非零退出和私有状态落盘，systemd有限重试；云端未完成对象不视为备份。不采集binlog坐标或刷新binlog，避免引入全局读锁等待，不宣称PITR或生产数据库已恢复验证。

## Existing notes audit

- [恢复包与日志离机](../../implemented/process/2026-10-22-verified-recovery-archives.md)：部分重叠；修正既有R2事实，数据库周期备份由本篇承接，日志退役仍是独立流程。
- [存储维护](../../implemented/process/2026-10-17-server-storage-maintenance.md)：部分重叠；保持三份部署备份、回滚保护及原维护任务。
- [发布安全](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：相邻决定；复用发布锁，不改变应用更新入口或线上版本。

## Alternatives considered

- 直接定时运行旧脚本：改动最少，但会累积本机大备份、缺少加密和远端内容核验，不能直接启用。
- 每轮先落本地完整dump再上传：可单独重试网络阶段并较早释放数据库快照，但当前剩余磁盘不足以长期积累，选流式并以唯一新run重试失败任务。
- 只上传发布备份：无需再次读取数据库，但备份频率仍依赖发布，不能满足持续备份。
- 自动轮换或删除历史远端：可控制费用，但本轮目标是建立可信的持续备份；先保留历史，后续再确定保留策略。

## Verification

- macOS存储回归69项中64项通过、5项Linux专属跳过；生产机独立测试目录69项全部通过，包含真实rclone本地crypt和bash/timeout取消。覆盖dump/压缩/上传失败、远端不一致、完成标记失败、锁竞争、配置权限及凭据边界。
- 生产预检确认71张基础表全部为InnoDB、MySQL8.0.46；首轮真实备份使用run 20261022T101612Z-393f05b4221f4edebea5f0b6cf92ceb3。实际运行回执保存在/var/lib/newapi-r2-backup；timer仅在该轮完整读回成功且服务正常退出后启用，不能用任务已启动或activating阶段的Result=success替代完成验收。
- Mac私有恢复配置完成真实R2随机测试对象上传、解密读回和内容比对，仅删除该测试对象。恢复目录0700、文件0600；不复制生产R2访问凭据到恢复目录，访问令牌丢失可从同一Cloudflare账户重新签发。每日timer配置与systemd calendar验证通过，失败间隔30分钟、12小时最多启动3次。
- 安装前旧脚本、原rclone配置及SHA核验记录保留在服务器/root/newapi-r2-backup-install-20261022，已安装内容与本地SHA相同。生产应用版本、容器及发布状态不变。make verify-notes和差异检查通过；现有release-safety CI自动包含新增35项回归及已有存储检查。这是独立运维工具，不发布应用Release。

## Consequences

全量扫描与网络背压仍增加I/O并延长一致性快照；45分钟容器内转储上限、170分钟整轮上限及低磁盘检查约束运行。启动要求5GiB可用，流式块间低水位为3GiB；阻塞时依赖硬时限。客户端加密需要独立保管解密材料。读回校验只能证明内容一致，不能替代完整数据库恢复及账务验收；每日备份也不自动提供任意时间点恢复。远端历史持续增长，后续需明确保留周期。

## References

- [MySQL mysqldump](https://dev.mysql.com/doc/refman/8.0/en/mysqldump.html)：InnoDB一致性快照与DDL限制。
- [rclone rcat](https://rclone.org/commands/rclone_rcat/)：stdin不能整轮重放，因此失败用唯一新run重做。
- [rclone crypt](https://rclone.org/crypt/)：数据加密、密码及salt的恢复依赖。
