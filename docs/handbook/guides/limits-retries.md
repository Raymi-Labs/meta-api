# 限流、超时与重试

平台限流可能来自账户、令牌、模型、渠道或上游。文档不提供一个适用于所有模型的固定 RPM、TPM 或并发数；以当前设置、错误提示和服务约定为准。

## 设计客户端并发

给同一个业务工作队列设置并发上限，避免所有用户请求同时冲向模型。分别设置连接超时、等待首字节超时和整体任务超时；长文本、推理、音频和视频不能共用一个过短的超时值。

流式输出可以改善等待体验，但不会取消请求限流，也不能保证每隔固定秒数都有数据。反向代理应避免缓冲 SSE，并保证读取超时符合业务。

## 有限重试策略

| 错误类型               | 默认处理                                              |
| ---------------------- | ----------------------------------------------------- |
| 无效密钥、权限、无余额 | 停止并提示修复，不自动重试                            |
| 参数或上下文错误       | 修改请求后再提交                                      |
| 429                    | 若有 `Retry-After` 则遵循，否则指数退避并加入随机抖动 |
| 临时网络或 5xx         | 仅在没有确认结果时有限重试                            |
| 已产生内容或任务 ID    | 保存现有结果、查询任务状态，避免自动重新创建          |

平台不保证每个 429 都带 `Retry-After`；该字段可能是秒数，也可能是 HTTP 日期。应用应同时设置最大重试次数和总等待预算，不能无限循环。

```python
import random
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime

def retry_delay(attempt, retry_after=None):
    if retry_after is not None:
        value = str(retry_after).strip()
        if value.isdigit():
            return float(value)
        try:
            deadline = parsedate_to_datetime(value)
            if deadline.tzinfo is None:
                deadline = deadline.replace(tzinfo=timezone.utc)
            return max(0.0, (deadline - datetime.now(timezone.utc)).total_seconds())
        except (ValueError, TypeError, OverflowError):
            pass
    return min(60.0, 2 ** min(max(0, attempt), 6)) + random.uniform(0, 1)
```

上面只计算等待时间，不自动重放请求。若服务要求的等待超过本次业务预算，应停止或延后任务，不能截短等待后继续请求。决定是否重试时，还应判断请求是否可能已被处理、是否涉及任务创建以及有没有输出。

## 上游回退与应用重试

网关可能按照管理员配置尝试其他渠道；这与应用自己再次发送整个请求不同。两层重试叠加会放大延迟和负载。应用应记录一次用户操作关联的所有尝试，避免看见一次界面点击就认为只有一次请求。

不要通过轮换密钥、账号或端点来绕过限流。确有稳定吞吐需求时，通过[企业定制](/enterprise)说明模型、峰值并发、平均输入输出量和时延目标。

## 验收要覆盖失败路径

测试取消请求、断网、首字节慢、流中途断开和临时限流，确认界面不会误报完成，日志可定位，队列不会继续无限积压。费用以实际使用日志和结算记录为准，不从重试次数推断扣费。
