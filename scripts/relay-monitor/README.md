# Flawless 整机流量监控

入口：`https://meta-api.vip/relay-monitor/`，由 Nginx 独立 Basic Auth 保护，与 New API 登录态无关。HTML 与 JSON 均要求认证。凭据不进入仓库。

## 组成与部署

- Ubuntu 软件包 `vnstat` 负责公网网卡 `ens3` 的日/月/累计流量；`/var/lib/vnstat/` 是持久化数据库，应备份，不应在常规更新时重建。
- `collect.py` 部署到 `/opt/relay-monitor/collect.py`。无登录权限的 `relay-monitor` 系统用户通过 oneshot service 每分钟采集，只能写 `/var/lib/relay-monitor/`。需有读取 vnStat 数据库和使用 ICMP datagram socket 的权限。
- 两个 systemd unit 部署到 `/etc/systemd/system/`，运行 daemon-reload 后启用 timer。vnStat daemon 与采集器均设 `TZ=Asia/Shanghai`，保持日/月划分及 CLI 日期解释一致；vnStat `SaveInterval 1` 表示每分钟保存。先设置时区再初始化统计，已有历史数据库不可通过重建修正时区。
- `index.html` 部署到 `/var/lib/relay-monitor/index.html`。既有 `meta-api.vip` HTTPS server 加入下方 location，不改业务 location。`/etc/nginx/relay-monitor.htpasswd` 由 `htpasswd` 创建，权限 root:www-data 640，至少使用随机强密码。

```nginx
location = /relay-monitor { return 302 /relay-monitor/; }
location ^~ /relay-monitor/ {
    alias /var/lib/relay-monitor/;
    index index.html;
    autoindex off;
    auth_basic "Flawless Monitor";
    auth_basic_user_file /etc/nginx/relay-monitor.htpasswd;
    add_header Cache-Control "no-store" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
}
```

配置后 `nginx -t`，再 reload。采集服务使用系统 `python3`，无需额外 Python 依赖、数据库服务或监听新端口。安装器/密码不得放进公开页面目录。

## 指标与失败行为

- vnStat 累计始于安装完成，不能补回此前的日/月历史；内核启动后计数单列并注明重启归零。接收/发送均统计，不能把两者之和当作供应商计费或套餐余量。
- 即时速率为每分钟内约 2 秒采样平均值，非链路上限；5 次 ICMP 的 RTT 平均值、mdev 和丢包率描述跳板到业务源站。最近 24 小时采样最多 1440 条，不产生无限增长文件。
- 快照原子替换。采集失败时上一份数据保留，超过 150 秒提示过期；HTTP/鉴权失败有明确提示。vnStat 不可用/ICMP 无回复显示错误，不伪造正常值。
- LibreSpeed 的电脑直连和中转测速沿用既有 8989/8990 入口，用户主动触发；不周期性跑吞吐测试。浏览器 HTTP 抖动与 ICMP mdev 口径不同。

## 运维与回退

`systemctl status vnstat relay-monitor.timer`；`systemctl show relay-monitor.service -p Result -p ExecMainStatus`；`TZ=Asia/Shanghai vnstat -i ens3`。备份 vnStat DB 时应通过 SQLite 在线备份或停止 vnStat 后复制，避免复制中间状态。

停用监控时停用 timer 并从 Nginx 移除两个监控 location，校验后 reload；保留 vnStat DB 以免丢失历史。只更新脚本/页面不重建数据库、不重新生成密码。此次为独立服务器运维工具，不发布 New API 应用版本。
