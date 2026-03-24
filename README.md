# Go Commerce

## 项目简介

Go Commerce是一个基于微服务架构的电子商务系统，使用Go语言开发后端服务，React开发前端应用，提供完整的购物体验。

## 主要特性

- **微服务架构**：采用模块化设计，各服务独立部署和扩展
- **用户认证**：基于JWT的安全认证系统
- **产品管理**：完整的产品CRUD操作，支持分类和搜索
- **购物车功能**：基于Redis的购物车管理
- **订单管理**：完整的订单创建和跟踪流程
- **响应式前端**：使用React构建的现代化用户界面

## 快速启动

### 环境准备

- Go 1.24.0或更高版本
- Node.js 16.0或更高版本
- Docker和Docker Compose
- MySQL 8.0
- Redis 7.0+
- RabbitMQ 3.0+

### 依赖安装

#### 后端依赖

```bash
# 安装Go依赖
go mod download
```

#### 前端依赖

```bash
# 进入前端目录
cd frontend

# 安装npm依赖
npm install
```

### 配置设置

1. 确保Docker服务正在运行
2. 启动基础服务（MySQL、Redis、RabbitMQ）：

```bash
docker-compose up -d mysql redis rabbitmq
```

### 启动服务

#### 后端服务

```bash
# 启动认证服务
go run ./cmd/auth-service

# 启动产品服务
go run ./cmd/product-service

# 启动订单服务
go run ./cmd/order-service

# 启动购物车服务
go run ./cmd/cart-service

# 启动API网关
go run ./cmd/api-gateway
```

#### 前端服务

```bash
# 进入前端目录
cd frontend

# 启动开发服务器
npm run dev
```

### 访问应用

- 前端应用：http://localhost:5173
- API网关：http://localhost:8081
- RabbitMQ管理界面：http://localhost:15672 (用户名: guest, 密码: guest)

## 使用示例

### 注册新用户

```bash
curl -X POST http://localhost:8081/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "password123", "email": "test@example.com"}'
```

### 登录获取令牌

```bash
curl -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "password123"}'
```

### 获取产品列表

```bash
curl http://localhost:8081/api/products
```

## 相关文档

- [技术文档](doc/TECHNICAL_DOCUMENT.md)：详细的技术架构和实现说明