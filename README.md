# 冰芯解冻完整性放行服务

本项目为极地冰芯样品提供从批次登记、运输温度偏差核查、完整性风险分级、解冻方案双人审批、分阶段观测、独立复核到放行报告封存的质量闭环。服务采用本地 JSON 快照和追加式哈希链审计，不依赖外部系统。

## 构建、运行与测试

```text
go test ./...
go run ./cmd/iceguard -addr=127.0.0.1:19081
go run ./cmd/iceguard -self-check -addr=127.0.0.1:19081
```

监听地址也可通过 `PORT` 环境变量指定端口，绑定地址固定为 `127.0.0.1`。主要 API 为 `/v1/batches` 及其 `transport`、`risk-summary`、`thaw-plan`、`observations`、`remediation`、`release-review` 和 `release-report` 子路径；使用 `X-Request-ID` 和 `X-Expected-Revision` 实现幂等与乐观并发控制。

批次登记会规范化冰芯编号、钻取位置和深度区间，并拒绝同位置的重复编号及重叠层位。`GET /v1/batches` 支持 `drill_site`、`status`、`integrity_grade`、`has_unmet_preconditions`、`limit` 和 `cursor` 筛选分页，并返回状态/完整性/风险待办统计。运输端点支持 `temperature_intervals`，自动形成低/高温超限分钟数、最大偏离、累计暴露量、严重度与可追溯风险评分；批次进入方案审批后仅可通过同一路径补充带操作者的处置、质量批准和双人复核。方案退回保留版本历史，`GET /v1/batches/{id}/thaw-plan` 返回当前与历史版本差异；阶段观测可通过 `GET /v1/batches/{id}/observations` 查询进度和异常聚合，并通过 `/v1/batches/{id}/observations/{observation_id}/correction` 追加更正。`GET /v1/batches/{id}/release-review` 提供放行资格门禁和审计链摘要；整改任务可逐项提交和独立复验，封存报告查询会返回报告摘要、链尾摘要和审计链验真回执。
