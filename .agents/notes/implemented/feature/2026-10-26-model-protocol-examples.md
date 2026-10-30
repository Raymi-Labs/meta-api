# Agent Note: 模型快速开始协议选择

Status: implemented

## Problem

快速开始将协议作为按钮，仅根据模型声明生成示例。用户要求下拉选择并提供 Responses；公开目录的 claude-fable-5-1 仅声明 anthropic/openai，网关全局存在 Responses 路径，但不能据此证明该模型所有分组支持。

## Decision

协议和代码语言采用同样的原生下拉与内缩箭头，保留键盘操作。详情对已有文本协议且非图像模型提供 Chat Completions、Responses、Messages 示例，新增模板标注未声明，选中后明确提示确认分组接口；默认仍选择已声明协议。无协议、图像、视频等模型不无条件补文本模板。

示例生成器默认继续严格使用声明；仅详情显式启用模板选项，不修改输入模型、/api/pricing、API端点区、筛选或文档声明。代码生成与复制使用选中协议、语言和当前模型，绝不发起调用。

## Existing notes audit

- [模型指标与示例](../../implemented/feature/2026-10-25-model-catalog-metrics.md)：保留七语言生成器，部分扩展“仅声明协议”的示例选择。
- [独立模型详情](../../implemented/feature/2026-10-25-model-detail-page.md)：保持实际接口声明和权限，示例模板不作为支持能力的证据。

## Alternatives considered

- 给所有Claude模型追加后端Responses声明可让现有UI直接显示，但Claude/AWS的入站Responses转换未实现；现有桥是Chat/Messages到上游Responses，不能反向推断。
- 继续隐藏未声明Responses最保守，却无法满足用户显式选择示例；使用标注清楚的可选模板，保留实际能力来源。
- 新增Responses到Messages的协议桥可扩展运行能力，但远超本轮快速开始界面范围，且需要工具调用、流事件、计费和取消语义设计，不在此处隐式实现。

## Verification

- 快速开始与运行时文档18项回归通过，七语言路径/请求体/特殊模型名/URL安全保持正确；未声明模板明确标记，原模型不被修改；图像、视频、embedding和空声明不补文本模板。
- Chrome 验收截图中的claude-fable-5-1：协议改为原生下拉，可选Responses并显示input请求；Python/TypeScript切换保留协议，跳到同厂商另一模型时恢复已声明默认协议。
- 原生下拉保留键盘与焦点样式；两个箭头距右侧14px，390px页面无整页横向溢出；语言提示覆盖17语言。
- 格式、静态检查、语言资源检查、主站与文档构建及预渲染通过。

## Consequences

用户可查看和复制Responses等文本协议模板，同时实际API能力仍由后台声明决定。选中未声明的模板可能无法在具体分组调用，因此逐项和选中状态均提示；不改后台渠道，不宣称完成真实Responses调用验证。协议选择按模型身份隔离，代码语言偏好在切换模型时保留。

[协议分层展示](2026-10-28-model-protocol-display.md)扩展卡片/表格到端点区的可见入口，共享声明与模板资格，保留本篇默认只选已声明协议的规则；混合视频/音频/嵌入端点与仅非文本输出不再自动补文本模板。

[接入信息合并](../simplification/2026-10-30-model-detail-quickstart-only.md)移除独立详情的重复端点区；广场 Responses 示例改到快速开始并预选模板，旧 #endpoints 兼容跳转；声明元数据与筛选语义不变。
