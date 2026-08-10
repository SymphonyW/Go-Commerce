# Observability

本地观测栈通过 Docker Compose profile 启动，覆盖 metrics、distributed tracing 和预置 Grafana dashboards。

```bash
docker compose --profile observability up -d --build
docker compose ps
```

访问地址：

| 组件 | 地址 | 说明 |
| --- | --- | --- |
| Grafana | `http://localhost:3000` | `admin / admin` |
| Prometheus | `http://localhost:9090` | 服务指标与 Collector 指标 |
| Tempo | `http://localhost:3200` | Trace backend HTTP API |
| OTEL Collector gRPC | `localhost:4317` | 服务 OTLP trace export |
| OTEL Collector HTTP | `localhost:4318` | 预留 OTLP HTTP |

## Trace Flow

服务通过 `OTEL_ENABLED` 控制是否启用 OpenTelemetry。Docker Compose 默认启用，并把 `OTEL_EXPORTER_OTLP_ENDPOINT` 指向 `otel-collector:4317`。Collector 或 Tempo 不可用时，业务服务仍会启动；`readyz` 不依赖 Grafana、Prometheus、Collector 或 Tempo。

同步链路：

```text
API Gateway HTTP span
  -> gRPC client span
  -> downstream gRPC server span
```

异步链路：

```text
producer span
  -> RabbitMQ headers traceparent
  -> consumer span
```

Outbox worker 从事件 payload 恢复 `request_id`、`trace_id`，并把这些 correlation fields 写入 RabbitMQ headers。出于事件存储精简考虑，outbox payload 不保存完整 span context。

## Logs

各服务使用统一 JSON slog 输出，字段包含：

- `service`
- `request_id`
- `trace_id`
- `span_id`

不会记录密码、JWT 或完整请求体。

## Metrics

所有服务在各自健康检查端口暴露 `/metrics`。关键指标包括：

| 指标 | 说明 |
| --- | --- |
| `http_requests_total` | HTTP 请求数 |
| `http_request_duration_seconds` | HTTP 延迟直方图 |
| `grpc_server_requests_total` | gRPC 服务端请求数 |
| `grpc_server_duration_seconds` | gRPC 服务端延迟直方图 |
| `orders_created_total` | 下单尝试结果 |
| `orders_cancelled_total` | 取消订单结果 |
| `orders_paid_total` | 支付成功驱动的订单 paid 迁移 |
| `insufficient_stock_total` | 库存不足 |
| `payments_succeeded_total` | 支付成功 |
| `outbox_pending_events` | Outbox pending backlog |
| `outbox_oldest_pending_seconds` | 最老 pending 事件年龄 |
| `outbox_publish_failures_total` | Outbox 发布失败 |
| `consumer_failures_total` | 消费者处理失败 |

指标 label 避免使用 `order_id`、`user_id` 这类高基数字段。

## Dashboards

Grafana 预置 dashboards：

| Dashboard | 内容 |
| --- | --- |
| `Go Commerce - API Gateway` | QPS、4xx/5xx、P50/P95/P99、下游 gRPC 错误 |
| `Go Commerce - Transaction` | 下单量、下单成功率、库存不足率、支付成功量、超时取消量 |
| `Go Commerce - Messaging` | Outbox backlog、最老事件年龄、retry、failed events、consumer failures |

完成一次注册、下单、支付成功后，可以在 Grafana Explore 选择 Tempo，并按 service 搜索：

```text
service.name = api-gateway
```

典型链路应能看到：

```text
HTTP POST /api/orders
  -> order-service/CreateOrder
  -> mysql.transaction
  -> outbox-worker publish
  -> RabbitMQ consume
  -> payment consumer
  -> order paid
```
