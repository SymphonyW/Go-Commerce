# Go Commerce API 文档

> 基础地址：`http://localhost:8080`
> OpenAPI 3.0 描述：[`openapi.yaml`](../openapi.yaml)

本文档用于阅读和理解接口；如需工具消费、接口导入或后续接入 Swagger UI，应以 `openapi.yaml` 为准。

## 1. 约定

### 1.1 鉴权标记

| 标记 | 含义 |
| --- | --- |
| Public | 无需登录 |
| JWT | 需要 `Authorization: Bearer <token>` |
| Merchant/Admin | 需要 JWT，且角色为 `merchant` 或 `admin` |

### 1.2 通用响应

错误响应通常为：

```json
{
  "error": "message"
}
```

常见状态码：

| 状态码 | 含义 |
| --- | --- |
| `400` | 参数错误 |
| `401` | 未登录或 token 无效 |
| `403` | 权限不足 |
| `404` | 资源不存在 |
| `409` | 前置条件冲突，例如状态不允许 |
| `500` | 服务内部错误 |

> 当前网关仍有少量历史接口未统一做 gRPC -> HTTP 错误映射，例如部分认证、购物车、旧版商家接口在下游报错时仍可能返回 `500`。这是当前实现状态，不是理想契约。

### 1.3 `Idempotency-Key`

| 接口 | Header 要求 | 当前实现 |
| --- | --- | --- |
| `POST /api/orders` | 必填 | 已落库，支持同 key 同请求回放 |
| `PUT /api/orders/:id/cancel` | 必填 | 网关要求 header，业务层依赖状态机天然幂等 |
| `POST /api/payments/:id/success` | 必填 | 网关要求 header，业务层依赖支付状态天然幂等 |

示例：

```http
Idempotency-Key: order-20260517-0001
```

### 1.4 分页、排序与筛选

| 场景 | 参数 |
| --- | --- |
| 商品列表 | `page`、`page_size`、`category`、`keyword`、`sort_by`、`order`、`min_price`、`max_price` |
| 商家控制台商品 | `page`、`page_size`、`merchant_id`（仅 `admin` 需要） |
| 商家控制台订单 | `page`、`page_size`、`merchant_id`（`admin` 可选） |

当前限制：

- `GET /api/merchants` 在网关层固定返回第 `1` 页、每页 `10` 条，尚未暴露查询参数。
- `GET /api/orders` 在网关层没有传分页参数，因此当前等价于返回第 `1` 页、每页 `10` 条。

## 2. 接口总览

| 分组 | 接口 | 权限 |
| --- | --- | --- |
| Auth | `POST /api/register` | Public |
| Auth | `POST /api/login` | Public |
| Product | `GET /api/products` | Public |
| Product | `GET /api/products/:id` | Public |
| Merchant Public | `GET /api/merchants` | Public |
| Merchant Public | `GET /api/merchants/:id` | Public |
| Merchant Legacy | `POST /api/merchants` | Merchant/Admin |
| Merchant Legacy | `POST /api/merchants/products` | Merchant/Admin |
| Merchant Legacy | `DELETE /api/merchants/products` | Merchant/Admin |
| Merchant Console | `GET /api/merchant/profile` | Merchant/Admin |
| Merchant Console | `GET /api/merchant/products` | Merchant/Admin |
| Merchant Console | `POST /api/merchant/products` | Merchant/Admin |
| Merchant Console | `PUT /api/merchant/products/:id` | Merchant/Admin |
| Merchant Console | `DELETE /api/merchant/products/:id` | Merchant/Admin |
| Merchant Console | `GET /api/merchant/orders` | Merchant/Admin |
| Order | `POST /api/orders` | JWT |
| Order | `GET /api/orders` | JWT |
| Order | `GET /api/orders/:id` | JWT |
| Order | `PUT /api/orders/:id/cancel` | JWT |
| Order | `PUT /api/orders/:id/ship` | Merchant/Admin |
| Order | `PUT /api/orders/:id/complete` | JWT |
| Payment | `POST /api/payments` | JWT |
| Payment | `GET /api/payments/:id` | JWT |
| Payment | `POST /api/payments/:id/success` | JWT |
| Payment | `POST /api/payments/:id/fail` | JWT |
| Cart | `POST /api/cart/items` | JWT |
| Cart | `GET /api/cart` | JWT |
| Cart | `PUT /api/cart/items` | JWT |
| Cart | `DELETE /api/cart/items` | JWT |
| Cart | `DELETE /api/cart` | JWT |

## 3. Auth

### 3.1 注册

- `POST /api/register`
- 权限：Public

请求体：

```json
{
  "username": "demo_user",
  "password": "password123",
  "email": "demo@example.com",
  "role": "customer"
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `username` | 是 | 用户名 |
| `password` | 是 | 密码 |
| `email` | 是 | 邮箱 |
| `role` | 否 | 仅允许 `customer` / `merchant`，默认 `customer`；`admin` 不能自助注册 |

响应：

```json
{
  "user_id": 1,
  "token": "JWT_TOKEN",
  "role": "customer"
}
```

常见错误：

| 状态码 | 场景 |
| --- | --- |
| `400` | JSON 解析失败 |
| `500` | 当前实现下，重复用户名、非法角色等下游错误可能被网关直接表现为 `500` |

### 3.2 登录

- `POST /api/login`
- 权限：Public

请求体：

```json
{
  "username": "demo_user",
  "password": "password123"
}
```

响应同注册接口。

## 4. Product

### 4.1 商品列表

- `GET /api/products`
- 权限：Public

查询参数：

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `page` | int | `1` | 页码 |
| `page_size` | int | `10` | 每页条数，最大 `100` |
| `category` | string | - | 精确分类筛选 |
| `keyword` | string | - | 模糊匹配商品名或描述 |
| `sort_by` | string | `created_at` | 可选 `created_at / price / stock` |
| `order` | string | `desc` | 可选 `asc / desc` |
| `min_price` | number | - | 最低价 |
| `max_price` | number | - | 最高价 |

示例：

```bash
curl "http://localhost:8080/api/products?page=2&page_size=20&category=book&keyword=Go&sort_by=price&order=asc&min_price=50&max_price=120"
```

响应：

```json
{
  "products": [
    {
      "id": 1,
      "name": "Go in Action",
      "description": "book",
      "price": 88.8,
      "stock": 20,
      "category": "book",
      "image_url": "https://example.com/book.png",
      "merchant_id": 1
    }
  ],
  "total": 1
}
```

常见错误：

| 状态码 | 场景 |
| --- | --- |
| `400` | `min_price > max_price` |
| `500` | 服务内部错误 |

### 4.2 商品详情

- `GET /api/products/:id`
- 权限：Public

响应：

```json
{
  "product": {
    "id": 1,
    "name": "Go in Action",
    "description": "book",
    "price": 88.8,
    "stock": 20,
    "category": "book",
    "image_url": "https://example.com/book.png",
    "merchant_id": 1
  }
}
```

## 5. Merchant

### 5.1 公开商家列表

- `GET /api/merchants`
- 权限：Public
- 当前网关固定查询第 `1` 页、每页 `10` 条

响应：

```json
{
  "merchants": [
    {
      "id": 1,
      "name": "Demo Shop",
      "contact_info": "shop@example.com",
      "created_at": "2026-05-17T08:00:00Z",
      "owner_user_id": 2
    }
  ],
  "total": 1
}
```

### 5.2 公开商家详情

- `GET /api/merchants/:id`
- 权限：Public

### 5.3 创建商家（兼容接口）

- `POST /api/merchants`
- 权限：Merchant/Admin

请求体：

```json
{
  "name": "Demo Shop",
  "contact_info": "shop@example.com"
}
```

说明：

- 服务端会把新商家绑定到当前登录用户。
- `customer` 无权创建商家。

### 5.4 兼容版商品写接口

#### `POST /api/merchants/products`

请求体：

```json
{
  "merchant_id": 1,
  "name": "Demo Product",
  "description": "desc",
  "price": 99.9,
  "stock": 10,
  "category": "book",
  "image_url": "https://example.com/product.png"
}
```

#### `DELETE /api/merchants/products`

请求体：

```json
{
  "merchant_id": 1,
  "product_id": 10
}
```

权限说明：

- `merchant` 只能操作自己名下商家。
- `admin` 可跨商家操作。

### 5.5 商家控制台

#### 获取当前店铺

- `GET /api/merchant/profile`
- 权限：Merchant/Admin
- `merchant`：默认返回自己最早创建的店铺
- `admin`：必须传 `merchant_id`

#### 商品列表

- `GET /api/merchant/products`
- 权限：Merchant/Admin

查询参数：

| 参数 | 说明 |
| --- | --- |
| `page` | 默认 `1` |
| `page_size` | 默认 `10`，最大 `100` |
| `merchant_id` | `admin` 用于指定目标店铺 |

#### 新增商品

- `POST /api/merchant/products`
- 权限：Merchant/Admin

请求体：

```json
{
  "name": "Demo Product",
  "description": "desc",
  "price": 99.9,
  "stock": 10,
  "category": "book",
  "image_url": "https://example.com/product.png"
}
```

#### 更新商品

- `PUT /api/merchant/products/:id`
- 权限：Merchant/Admin
- 至少提交一个字段

请求体示例：

```json
{
  "price": 88.8,
  "stock": 20
}
```

#### 删除商品

- `DELETE /api/merchant/products/:id`
- 权限：Merchant/Admin

#### 查看商家订单

- `GET /api/merchant/orders`
- 权限：Merchant/Admin

查询参数：

| 参数 | 说明 |
| --- | --- |
| `page` | 默认 `1` |
| `page_size` | 默认 `10`，最大 `100` |
| `merchant_id` | `admin` 可选；不传时可查看全部订单 |

说明：

- 查询基于 `order_items.merchant_id` 快照。
- 混合商家订单只返回当前商家可见的订单项。

## 6. Order

### 6.1 创建订单

- `POST /api/orders`
- 权限：JWT
- `Idempotency-Key`：必填

请求体：

```json
{
  "items": [
    {
      "product_id": 1,
      "quantity": 2
    }
  ]
}
```

响应：

```json
{
  "order": {
    "id": 1,
    "user_id": 8,
    "items": [
      {
        "product_id": 1,
        "product_name": "Demo Product",
        "price": 99.9,
        "quantity": 2
      }
    ],
    "total_amount": 199.8,
    "status": "pending",
    "created_at": "2026-05-17T08:00:00Z",
    "cancel_reason": ""
  }
}
```

说明：

- 前端不能提交金额。
- 商品名称和价格均来自下单时的后端快照。

常见错误：

| 状态码 | 场景 |
| --- | --- |
| `400` | 缺少幂等键、空订单、非法数量、库存不足 |
| `404` | 商品不存在 |
| `409` | 同一幂等键复用但请求体不同，或请求仍在处理中 |

### 6.2 订单列表

- `GET /api/orders`
- 权限：JWT
- 当前等价于第一页、每页十条

### 6.3 订单详情

- `GET /api/orders/:id`
- 权限：JWT
- 只能查询自己的订单

### 6.4 取消订单

- `PUT /api/orders/:id/cancel`
- 权限：JWT
- `Idempotency-Key`：当前网关要求必填

响应：

```json
{
  "success": true,
  "message": "订单取消成功"
}
```

说明：

- 只允许 `pending -> cancelled`
- 已支付、已发货、已完成订单不能取消

### 6.5 发货

- `PUT /api/orders/:id/ship`
- 权限：Merchant/Admin

说明：

- 只允许 `paid -> shipped`
- 普通商家只能为自己名下商品组成的订单发货
- 混合商家订单当前仅允许 `admin` 发货

### 6.6 确认收货

- `PUT /api/orders/:id/complete`
- 权限：JWT

说明：

- 只允许 `shipped -> completed`
- 只能由订单所属用户确认

## 7. Payment

### 7.1 创建支付

- `POST /api/payments`
- 权限：JWT

请求体：

```json
{
  "order_id": 1,
  "payment_method": "mock_balance"
}
```

可选支付方式：

- `mock_balance`
- `mock_wechat`
- `mock_alipay`

响应：

```json
{
  "payment": {
    "id": 1,
    "payment_no": "pay-0123456789abcdef",
    "order_id": 1,
    "user_id": 8,
    "amount": 199.8,
    "status": "created",
    "payment_method": "mock_balance",
    "created_at": "2026-05-17T08:00:00Z",
    "updated_at": "2026-05-17T08:00:00Z"
  }
}
```

说明：

- 金额直接来自订单，不接收客户端金额。
- 一个订单同一时刻只能有一条活跃支付记录。

### 7.2 查询支付

- `GET /api/payments/:id`
- 权限：JWT
- 只能查询自己的支付单

### 7.3 支付成功

- `POST /api/payments/:id/success`
- 权限：JWT
- `Idempotency-Key`：当前网关要求必填

响应：

```json
{
  "payment": {
    "id": 1,
    "status": "succeeded"
  }
}
```

### 7.4 支付失败

- `POST /api/payments/:id/fail`
- 权限：JWT

说明：

- 支付单变为 `failed`
- 订单保持 `pending`
- 用户可以重新创建新的支付尝试

## 8. Cart

### 8.1 添加商品

- `POST /api/cart/items`
- 权限：JWT

请求体：

```json
{
  "product_id": 1,
  "quantity": 1
}
```

### 8.2 获取购物车

- `GET /api/cart`
- 权限：JWT

响应：

```json
{
  "items": [
    {
      "product_id": 1,
      "product_name": "Demo Product",
      "price": 99.9,
      "quantity": 1,
      "image_url": "https://example.com/product.png"
    }
  ],
  "total_amount": 99.9
}
```

### 8.3 更新数量

- `PUT /api/cart/items`
- 权限：JWT

### 8.4 删除商品

- `DELETE /api/cart/items`
- 权限：JWT

### 8.5 清空购物车

- `DELETE /api/cart`
- 权限：JWT

## 9. 观测接口

| 接口 | 说明 |
| --- | --- |
| `GET /healthz` | API Gateway 存活探针 |
| `GET /readyz` | API Gateway 就绪探针 |
| `GET /metrics` | API Gateway Prometheus 指标 |

各内部服务也暴露各自独立的健康检查端口，见 `.env.example` 与 `docker-compose.yml`。
