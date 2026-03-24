# Go Commerce

## 项目简介

Go Commerce是一个基于微服务架构的电子商务系统，使用Go语言开发后端服务，React开发前端应用，提供完整的购物体验。

## 主要特性

- **微服务架构**：采用模块化设计，各服务独立部署和扩展
- **用户认证**：基于JWT的安全认证系统
- **产品管理**：完整的产品CRUD操作，支持分类和搜索
- **购物车功能**：基于Redis的购物车管理
- **订单管理**：完整的订单创建和跟踪流程
- **商家管理**：支持商家注册、商品管理（增删操作）
- **用户下单**：支持用户下单，检查库存，生成订单记录
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

# 启动商家服务
go run ./cmd/merchant-service

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

- 前端应用：<http://localhost:5173>
- API网关：<http://localhost:8081>
- RabbitMQ管理界面：<http://localhost:15672> (用户名: guest, 密码: guest)

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

### 创建商家

```bash
curl -X POST http://localhost:8081/api/merchants \
  -H "Content-Type: application/json" \
  -d '{"name": "Test Merchant", "contact_info": "test@merchant.com"}'
```

### 商家添加商品

```bash
curl -X POST http://localhost:8081/api/merchants/products \
  -H "Content-Type: application/json" \
  -d '{"merchant_id": 1, "name": "Test Product", "description": "Test Description", "price": 99.99, "stock": 100, "category": "Electronics", "image_url": "https://example.com/image.jpg"}'
```

### 商家删除商品

```bash
curl -X DELETE http://localhost:8081/api/merchants/products \
  -H "Content-Type: application/json" \
  -d '{"merchant_id": 1, "product_id": 1}'
```

### 用户下单

```bash
curl -X POST http://localhost:8081/api/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"items": [{"product_id": 1, "product_name": "Test Product", "price": 99.99, "quantity": 1}]}'
```

### 购物车操作

#### 添加商品到购物车

```bash
curl -X POST http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1, "quantity": 1}'
```

#### 获取购物车

```bash
curl -X GET http://localhost:8081/api/cart \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### 更新购物车商品数量

```bash
curl -X PUT http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1, "quantity": 2}'
```

#### 删除购物车商品

```bash
curl -X DELETE http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1}'
```

#### 清空购物车

```bash
curl -X DELETE http://localhost:8081/api/cart \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## 相关文档

- [技术文档](doc/TECHNICAL_DOCUMENT.md)：详细的技术架构和实现说明

