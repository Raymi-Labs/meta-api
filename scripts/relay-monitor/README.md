# 轻量跳板流量监控

入口：`https://meta-api.vip/relay-monitor/`，按用户要求公开只读访问，无需账号密码。HTML/JSON 不含凭据、请求日志或客户端身份。

## 数据与口径

- vnStat 持久化公网网卡 `ens3` 收发累计；后续更新不得重建数据库或清零。
- 5 分钟明细仅保留 24 小时（`5MinuteHours 24`），小时数字汇总保留8天以覆盖滚动窗口左边界（`HourlyDays 8`），估算只取其中最近168小时；排行榜关闭（`TopDayEntries 0`）。日汇总保留最近一天（`DailyDays 1`），月/年/总计保留数字汇总。页面不展示历史日表。
- 自有曲线每分钟采样约 2 秒，最多保留 24 小时/1440 条；JSON 只输出曲线、当前链路和汇总数字，不再输出 vnStat 全量历史。
- 预计本月总流量 = 本月实际累计 + 最近168小时平均每秒速率 × 本月剩余秒数；按北京时间的自然月末计算，接收/发送分别计算再合计。当天也参与滚动窗口，不要求完整自然日。
- 不足168小时，按已有有效观测秒数求平均，仍提供预估并标注时长；无有效采样时间才显示暂无数据。小时桶在窗口边界按重叠时长比例分摊，属于近似值，不能预测未来突然增长或节假日。
- 最近24小时流量卡与曲线仍用24小时；预估单独使用小时数字汇总，JSON不输出小时历史。保留内部 `forecast_30d` 便于兼容/对比，页面使用 `forecast_month`，两者标签不可混用。若本月监控并非从月初开始，“已用”仅代表已记录部分，安装前历史无法恢复。
- 升级前停止 vnStat 并备份，在已有5分钟明细上合并小时汇总，仅补缺失桶；核对 interface 累计不变，不重建数据库。旧5分钟明细已清理的时段不补造数据。
- 预估不把2秒瞬时速率当依据。统计包含双向中转、测速及管理流量，不能等同供应商账单或套餐余量。
- 到源站的平均延迟/mdev/丢包率每分钟测 5 次 ICMP；电脑实际上传下载沿用 LibreSpeed，由用户主动触发。

## 服务器状态

复用2秒采样窗口读取 `/proc/stat`，CPU百分比排除 idle/iowait，不重复计算 guest；vCPU 显示系统逻辑处理器数量。内存用 MemTotal−MemAvailable，避免把可回收缓存误算为不可用；显示 Swap 已用/总量。系统盘显示已用、可用、总量及与 df 同口径的已用/(已用+可用)比例。负载分别为1/5/15分钟，运行时间来自 /proc/uptime。

这些指标只保存当前快照，不增加历史明细、依赖或采集等待。CPU无有效窗口显示暂无数据，系统读取异常显式报错，流量采集继续运行；不公开进程、环境变量或文件列表。

## 部署与服务

依赖 Ubuntu `vnstat`、Python 3、既有 Nginx。`collect.py` 放入 `/opt/relay-monitor/`，页面放入 `/var/lib/relay-monitor/index.html`，两个 systemd unit 放入 `/etc/systemd/system/`。无 shell 的 `relay-monitor` 系统用户只需写状态目录、读 vnStat 数据库和 ICMP datagram socket 权限。设置 `StateDirectory=relay-monitor`，启用 `relay-monitor.timer` 与 `vnstat`。

vnStat daemon 与 collector 均设 `TZ=Asia/Shanghai`，`SaveInterval 1` 每分钟保存。日期文本由服务端生成，避免浏览器时区扩展改写。快照原子替换；读取失败保留上一份数据，页面在 150 秒过期或读取失败时提示；vnStat 汇总过期也独立提示。

在主域名 HTTPS server 内添加：

```nginx
location = /relay-monitor { return 302 /relay-monitor/; }
location ^~ /relay-monitor/ {
    alias /var/lib/relay-monitor/;
    index index.html;
    autoindex off;
    add_header Cache-Control "no-store" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
}
```

运行 `nginx -t` 后 reload。只对监控路径移除 Basic Auth，不修改应用登录权限。原 htpasswd 已移到服务器 root 专用回退目录，旧登录文件不再使用。

检查：`systemctl status vnstat relay-monitor.timer`、`systemctl show relay-monitor.service -p Result -p ExecMainStatus`、`TZ=Asia/Shanghai vnstat -i ens3`。估算边界回归：`python3 -m unittest discover -s scripts/relay-monitor -p 'test_*.py'`。

迁移清理先停止 vnStat、备份数据库，再按各自保留期删除超龄明细和过期 day 汇总；保留 interface 累计、month/year 汇总，核对清理前后累计完全一致。正常运行由 vnStat 保留策略及 collector 滚动裁剪自动清理，无需另起清理服务。备份不在公开路径。

停用时关闭 timer 并移除两个监控 location，校验后 reload，保留数据库。独立运维工具无应用发布流程，不发布 New API 版本。
