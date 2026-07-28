你正在维护仓库：https://github.com/SymphonyW/Go-Commerce

开始编码前必须：
1. 阅读 README.md、docs/TECHNICAL_DOCUMENT.md，以及本任务涉及的代码和测试。
2. 先输出简短实施计划，再开始修改。
3. 不修改与当前任务无关的业务逻辑。
4. 保持现有 REST API、gRPC 接口和前端行为兼容，除非任务明确要求调整契约。
5. 所有新增核心逻辑必须包含单元测试；涉及 MySQL、Redis、RabbitMQ 并发语义时必须增加 integration test。
6. 运行并确保以下命令通过：
   gofmt -w .
   go vet ./...
   go test ./...
   go test -tags=integration ./...
   go build ./...
7. 修改完成后同步 README、OpenAPI 或技术文档中与本任务直接相关的内容。
8. 最后输出：
   - 修改文件列表
   - 核心设计说明
   - 测试结果
   - 尚存风险
   - 推荐 commit message