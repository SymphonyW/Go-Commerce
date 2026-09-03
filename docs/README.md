# 文档导航

该目录用于存放 Go-Commerce 的补充文档。根目录 [README](../README.md) 面向首次进入仓库的读者，负责快速说明项目定位、核心链路、快速开始和当前边界；本目录文档用于展开技术细节、接口说明与面试讲解。

| 文档 | 用途 |
| --- | --- |
| [技术文档](TECHNICAL_DOCUMENT.md) | 架构设计、交易链路、消息机制、权限模型与当前不足。 |
| [观测栈文档](OBSERVABILITY.md) | OpenTelemetry、Prometheus、Tempo、Grafana 的本地启动、指标和 dashboards。 |
| [数据库迁移](../db/README.md) | Goose 迁移命令、Compose migrate 服务、baseline 和 AutoMigrate 开关。 |
| [API 文档](API_Documentation.md) | 面向阅读的 REST API 说明，包含鉴权、分页、错误码和请求 / 响应示例。 |
| [面试文档](INTERVIEW.md) | 围绕真实实现整理项目介绍、亮点讲解和常见追问。 |
| [OpenAPI 3.0](../openapi.yaml) | 机器可读接口契约，可用于导入 API 工具或后续接入 Swagger UI。 |

推荐阅读顺序：

1. 想快速理解项目：先看根目录 [README](../README.md)。
2. 想了解架构和工程取舍：看 [技术文档](TECHNICAL_DOCUMENT.md)。
3. 想调接口：看 [API 文档](API_Documentation.md) 或 [OpenAPI 3.0](../openapi.yaml)。
4. 想准备面试 / 答辩：看 [面试文档](INTERVIEW.md)。
