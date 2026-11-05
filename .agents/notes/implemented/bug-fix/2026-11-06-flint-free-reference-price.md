# Agent Note: Flint 免费变体参考价

Status: implemented

## Problem

生产对外模型 flint-v1.0:free 缺少参考价，旧 recursive/flint-v1.0:free 资料虽有输入/输出零价，却被厂家来源过滤排除。用户要求该免费变体显示输入、输出均为0。OpenRouter 自身的该变体页面明确公布两项免费；这不是从本站倍率或开源许可推断厂家推理价格。

## Decision

复用模型管理 official_pricing 和公开 pricing，不新增计费或页面。仅为已核实的 Flint v1.0 免费变体及其明确对外别名放行 OpenRouter 精确模型页的 USD / million_tokens 零价快照：输入、输出必须显式为0，其他费目只能未知或0，限定单档及精确模型规则。价格说明明确这是 OpenRouter 免费变体；厂家价和自动同步的来源校验保持原语义。通过管理 API 给两条模型资料写入同一核实快照，保留渠道状态及本站售价。

## Existing notes audit

- [厂家来源边界](../../implemented/bug-fix/2026-11-01-manufacturer-official-pricing.md)：部分取代“所有平台变体均空态”，增加明确免费变体的小范围例外；其他转售报价仍过滤。
- [官方参考价](../../implemented/feature/2026-10-30-model-official-pricing.md)：复用零与未知区分、人工资料、详情与分组一致性。
- [价格覆盖](../../implemented/bug-fix/2026-11-01-official-pricing-coverage.md)：不扩大抓取或自动同步，失败和人工保护不变。

## Alternatives considered

- 只补零价数据：现有公开过滤仍拒绝含冒号的型号，无法完成展示。
- 从本站倍率0推断官方价：本站促销不代表供应商价格，不采用。
- 放开所有 OpenRouter 报价或所有 :free 后缀：会把普通代理报价、未经核实型号或不同变体套用为官方价，不采用。

## Verification

Go 定向回归覆盖精确来源及两个名称输出输入/输出0，以及缺值、正价、错型号、错来源、伪装域名、额外收费和宽泛匹配拒绝；普通厂家价和旧代理价行为保持；公开 Pricing 序列化保留数值0。9项前端零价/分组比较回归和 Go 服务端构建通过。模型资料已通过生产管理 API 保存并读回；公开展示仍需部署修复版后核对，不将资料保存视为上线成功。

## Consequences

参考资料是2026-11-06人工核实快照，并非永久免费保证；来源变化需重新核实。未来其他免费变体需要独立证据，不能凭后缀自动推导。
