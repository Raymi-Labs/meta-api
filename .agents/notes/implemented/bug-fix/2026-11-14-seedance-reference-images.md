# Agent Note: Seedance 2.5 参考照片角色映射

Status: implemented

## Problem

客户端已有 images/本地照片编码，豆包适配器生成 image_url 时却不带 role；2.5 单张参考和首帧语义无法稳定区分。metadata.content 还会覆盖统一 images，导致照片被静默丢弃。

## Decision

复用统一视频入口、鉴权/分组/模型映射和同一出站计费快照。仅对映射后的两个明确 2.5 型号，默认统一图片为 reference_image；支持显式 reference、first_frame、first_last_frame 模式以及已有 metadata.content 角色。拒绝混用、数量/模式不符和重复来源；帧模式要求 adaptive。旧型号保持原行为。已有客户端无须升级即可获益。

## Existing notes audit

[共同契约](../../implemented/bug-fix/2026-10-26-video-task-common-contract.md)部分重叠，保留计费与一次提交决定；[客户端](../../implemented/feature/2026-10-29-video-generation-skill.md)部分重叠，保留上传编码和恢复。Seedance 官方展示元数据与视频定价提案无关。

## Alternatives considered

- 只在客户端构造 metadata.content 可绕过缺口，但旧集成仍丢角色且重复协议知识；选择服务端统一映射。
- 按图片数量猜首尾帧容易实现，但会把两张参考照片误当时间端点；仅显式模式指定帧语义。

## Verification

完整 Validate/Build 测试覆盖图片、别名映射、帧模式、混用/数量/重复来源和旧型号；共同快照测试验证图片角色、计费参数与发送一致。relay/router/豆包适配器回归通过。43项客户端测试及下载包一致性通过；专项 race、前端构建、服务端构建/版本检查、29项发布脚本测试和笔记校验通过。前端本地 prerender 因缺 Chromium 按既有策略跳过，服务端保留动态元数据兜底。无收费生成或生产部署。

## Consequences

供应商可用性和真人照片权限需账号验收；本次仅照片，不宣称音视频参考、编辑或延长已实现。
