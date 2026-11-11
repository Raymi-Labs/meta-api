# GIF 用户参数与渠道覆盖

Skill 已保留用户明确指定的型号、尺寸、质量、帧数和帧间隔；默认仍为512×512、medium、9帧、每帧200毫秒。部署新版程序不会自动更改数据库中的渠道参数覆盖。

## 仅调整旧固定GIF预设

管理员先导出目标生图渠道的当前参数覆盖，保留回退副本，再在对应渠道编辑器核对规则。不要将整个参数覆盖清空或把一份历史配置复制覆盖到当前渠道。

历史预设见 `relay/common/testdata/gpt26_gif_preset.json`。若当前渠道确实包含下列操作，仅移除这些**强制GIF值**的操作：

| path | mode | 旧 value | 条件 |
| --- | --- | --- | --- |
| size | set | 512x512 | output_format full gif，invert=false |
| quality | set | medium | 同上 |
| frame_count | set | 9 | 同上 |
| frame_duration_ms | set | 200 | 同上 |

保留非GIF条件分支、鉴权/路由字段及其他业务规则。当前Skill始终为缺省GIF请求填入默认值，因此移除上述覆盖后默认体验不变，显式用户参数可以原样送出。直接API调用者未提供参数时遵循上游缺省值。

`n=1`是当前一次交付一张图片的工作流，不在本次GIF帧参数调整范围内。不要将多个生成结果误当成GIF帧数。

## 验证与回退

无需实际生成：本地Skill `--dry-run --output-format gif --frame-count 24 --frame-duration-ms 80 --size 1024x1024 --quality high` 可检查出站参数。服务端参数覆盖可用已有ApplyParamOverride夹具核对，不能只凭Skill dry-run宣称线上已生效。

需要回退时仅恢复本次删除的四类匹配操作；保留此后其他修改。该文档是精确配置调整指引，不代表已操作生产。用户负责服务器部署与线上配置后，再用正常用户请求观察实际结果，不额外付费生成测试样本。
