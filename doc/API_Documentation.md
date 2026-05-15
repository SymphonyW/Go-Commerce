# API 文档

## 1. 认证接口

### 1.1 用户注册

- **端点**：`POST /api/register`
- **描述**：注册新用户
- **参数**：
  - `username` (string)：用户名
  - `password` (string)：密码
  - `email` (string)：邮箱
  - `role` (string，可选)：用户角色，可选 `customer` 或 `merchant`，默认 `customer`
- **响应**：
  ```json
  {
    "user_id": 1,
    "token": "JWT_TOKEN",
    "role": "customer"
  }
  ```

### 1.2 用户登录

- **端点**：`POST /api/login`
- **描述**：用户登录并获取JWT令牌
- **参数**：
  - `username` (string)：用户名
  - `password` (string)：密码
- **响应**：
  ```json
  {
    "user_id": 1,
    "token": "JWT_TOKEN",
    "role": "customer"
  }
  ```

## 2. 产品接口

### 2.1 获取产品列表

- **端点**：`GET /api/products`
- **描述**：获取产品列表，支持分页和分类筛选
- **参数**：
  - `page` (int)：页码，默认1
  - `page_size` (int)：每页数量，默认10
  - `category` (string)：分类筛选
- **响应**：
  ```json
  {
    "products": [
      {
        "id": 1,
        "name": "Product Name",
        "description": "Product Description",
        "price": 99.99,
        "stock": 100,
        "category": "Electronics",
        "image_url": "https://example.com/image.jpg",
        "merchant_id": 1
      }
    ],
    "total": 1
  }
  ```

### 2.2 获取产品详情

- **端点**：`GET /api/products/:id`
- **描述**：获取单个产品的详细信息
- **参数**：
  - `id` (int)：产品ID
- **响应**：
  ```json
  {
    "product": {
      "id": 1,
      "name": "Product Name",
      "description": "Product Description",
      "price": 99.99,
      "stock": 100,
      "category": "Electronics",
      "image_url": "https://example.com/image.jpg",
      "merchant_id": 1
    }
  }
  ```

## 3. 商家管理接口（v2.0新增）

### 3.1 创建商家

- **端点**：`POST /api/merchants`
- **描述**：创建新商家
- **权限**：需要登录，且角色为 `merchant` 或 `admin`
- **参数**：
  - `name` (string)：商家名称
  - `contact_info` (string)：联系方式
- **说明**：
  - 服务端会自动把新商家绑定到当前登录用户
  - `customer` 无权创建商家
- **响应**：
  ```json
  {
    "merchant": {
      "id": 1,
      "name": "Test Merchant",
      "contact_info": "test@merchant.com",
      "owner_user_id": 2,
      "created_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 3.2 获取商家信息

- **端点**：`GET /api/merchants/:id`
- **描述**：获取商家详细信息
- **参数**：
  - `id` (int)：商家ID
- **响应**：
  ```json
  {
    "merchant": {
      "id": 1,
      "name": "Test Merchant",
      "contact_info": "test@merchant.com",
      "owner_user_id": 2,
      "created_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 3.3 列出商家

- **端点**：`GET /api/merchants`
- **描述**：获取商家列表
- **参数**：
  - `page` (int)：页码，默认1
  - `page_size` (int)：每页数量，默认10
- **响应**：
  ```json
  {
    "merchants": [
      {
        "id": 1,
        "name": "Test Merchant",
        "contact_info": "test@merchant.com",
        "owner_user_id": 2,
        "created_at": "2026-03-24T00:00:00Z"
      }
    ],
    "total": 1
  }
  ```

### 3.4 商家添加商品

- **端点**：`POST /api/merchants/products`
- **描述**：商家添加新商品
- **权限**：需要登录，且角色为 `merchant` 或 `admin`
- **参数**：
  - `merchant_id` (int)：商家ID
  - `name` (string)：商品名称
  - `description` (string)：商品描述
  - `price` (float)：商品价格
  - `stock` (int)：商品库存
  - `category` (string)：商品分类
  - `image_url` (string)：商品图片URL
- **说明**：
  - `merchant` 只能操作归属于自己的商家
  - `admin` 可以操作任意商家
- **响应**：
  ```json
  {
    "product_id": 1
  }
  ```

### 3.5 商家删除商品

- **端点**：`DELETE /api/merchants/products`
- **描述**：商家删除自有商品
- **权限**：需要登录，且角色为 `merchant` 或 `admin`
- **参数**：
  - `merchant_id` (int)：商家ID
  - `product_id` (int)：商品ID
- **说明**：
  - `merchant` 只能删除自己商家下的商品
  - `admin` 可以删除任意商家下的商品
- **响应**：
  ```json
  {
    "success": true
  }
  ```

## 4. 订单流程接口（v2.0新增）

### 4.1 创建订单

- **端点**：`POST /api/orders`
- **描述**：用户创建订单；订单金额由后端基于商品真实价格计算，响应中的商品名称和价格为下单时快照
- **参数**：
  - `items` (array)：订单项
    - `product_id` (int)：商品ID
    - `quantity` (int)：商品数量
- **说明**：
  - 客户端不需要、也不应提交商品名称或商品价格
  - 服务端会以商品当前真实价格计算订单总金额
  - 订单创建后会保存商品名称与价格快照，后续商品改价不会影响历史订单展示
- **响应**：
  ```json
  {
    "order": {
      "id": 1,
      "user_id": 1,
      "items": [
        {
          "product_id": 1,
          "product_name": "Test Product",
          "price": 99.99,
          "quantity": 1
        }
      ],
      "total_amount": 99.99,
      "status": "pending",
      "created_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 4.2 获取订单详情

- **端点**：`GET /api/orders/:id`
- **描述**：获取订单详细信息
- **参数**：
  - `id` (int)：订单ID
- **响应**：
  ```json
  {
    "order": {
      "id": 1,
      "user_id": 1,
      "items": [
        {
          "product_id": 1,
          "product_name": "Test Product",
          "price": 99.99,
          "quantity": 1
        }
      ],
      "total_amount": 99.99,
      "status": "pending",
      "created_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 4.3 获取订单列表

- **端点**：`GET /api/orders`
- **描述**：获取用户的订单列表
- **响应**：
  ```json
  {
    "orders": [
      {
        "id": 1,
        "status": "pending",
        "total_amount": 99.99,
        "created_at": "2026-03-24T00:00:00Z"
      },
      {
        "id": 2,
        "status": "completed",
        "total_amount": 199.98,
        "created_at": "2026-03-23T00:00:00Z"
      }
    ]
  }
  ```

### 4.4 取消订单

- **端点**：`PUT /api/orders/:id/cancel`
- **描述**：当前用户取消自己的订单
- **说明**：
  - 仅允许 `pending -> cancelled`
  - 已支付、已发货、已完成订单都不能取消
- **响应**：
  ```json
  {
    "success": true,
    "message": "订单取消成功"
  }
  ```

### 4.5 商家发货

- **端点**：`PUT /api/orders/:id/ship`
- **描述**：商家或管理员把已支付订单推进为已发货
- **权限**：需要登录，且角色为 `merchant` 或 `admin`
- **说明**：
  - 仅允许 `paid -> shipped`
  - 服务端会根据订单项中的商家快照校验资源归属
  - 普通商家只能为自己名下商品构成的订单发货；混合商家订单当前仅允许 `admin` 发货
- **响应**：
  ```json
  {
    "success": true,
    "message": "订单已发货"
  }
  ```

### 4.6 确认收货

- **端点**：`PUT /api/orders/:id/complete`
- **描述**：订单所属用户确认收货
- **说明**：
  - 仅允许 `shipped -> completed`
  - `user_id` 由网关从 JWT 注入，不信任客户端传入
- **响应**：
  ```json
  {
    "success": true,
    "message": "订单已完成"
  }
  ```

## 5. 支付接口

> 以下接口都要求登录；`user_id` 由网关从 JWT 注入，不信任客户端自行传入。

### 5.1 创建支付

- **端点**：`POST /api/payments`
- **描述**：为当前用户的待支付订单创建一条模拟支付记录
- **参数**：
  - `order_id` (int)：订单ID
  - `payment_method` (string)：支付方式，当前支持 `mock_balance`、`mock_wechat`、`mock_alipay`
- **说明**：
  - 订单必须存在、归属当前用户且状态为 `pending`
  - 支付金额由服务端直接读取订单总金额，不接受客户端传入金额
  - 同一订单同一时刻只能存在一条活跃支付记录（`created` 或 `succeeded`）
- **响应**：
  ```json
  {
    "payment": {
      "id": 1,
      "payment_no": "pay-0123456789abcdef",
      "order_id": 1,
      "user_id": 1,
      "amount": 99.99,
      "status": "created",
      "payment_method": "mock_balance",
      "created_at": "2026-03-24T00:00:00Z",
      "updated_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 5.2 查询支付结果

- **端点**：`GET /api/payments/:id`
- **描述**：查询当前用户自己的支付记录
- **参数**：
  - `id` (int)：支付记录ID
- **响应**：
  ```json
  {
    "payment": {
      "id": 1,
      "payment_no": "pay-0123456789abcdef",
      "order_id": 1,
      "user_id": 1,
      "amount": 99.99,
      "status": "created",
      "payment_method": "mock_balance",
      "created_at": "2026-03-24T00:00:00Z",
      "updated_at": "2026-03-24T00:00:00Z"
    }
  }
  ```

### 5.3 模拟支付成功

- **端点**：`POST /api/payments/:id/success`
- **描述**：把当前用户自己的模拟支付标记为成功
- **说明**：
  - 仅允许 `created` 状态的支付单继续推进
  - 服务端会再次校验订单仍为 `pending` 且金额未变化
  - 成功后发布 `payment.succeeded` 事件，订单服务消费后把订单状态更新为 `paid`
- **响应**：
  ```json
  {
    "payment": {
      "id": 1,
      "status": "succeeded"
    }
  }
  ```

### 5.4 模拟支付失败

- **端点**：`POST /api/payments/:id/fail`
- **描述**：把当前用户自己的模拟支付标记为失败
- **说明**：
  - 当前实现把支付单标记为 `failed`
  - 订单继续保持 `pending`，便于用户再次发起新的支付尝试
- **响应**：
  ```json
  {
    "payment": {
      "id": 1,
      "status": "failed"
    }
  }
  ```

## 6. 购物车接口

### 6.1 添加商品到购物车

- **端点**：`POST /api/cart/items`
- **描述**：向购物车添加商品
- **参数**：
  - `product_id` (int)：商品ID
  - `quantity` (int)：商品数量
- **响应**：
  ```json
  {
    "item": {
      "product_id": 1,
      "product_name": "Product Name",
      "price": 99.99,
      "quantity": 1,
      "image_url": "https://example.com/image.jpg"
    }
  }
  ```

### 6.2 获取购物车

- **端点**：`GET /api/cart`
- **描述**：获取用户的购物车信息
- **响应**：
  ```json
  {
    "items": [
      {
        "product_id": 1,
        "product_name": "Product Name",
        "price": 99.99,
        "quantity": 1,
        "image_url": "https://example.com/image.jpg"
      }
    ],
    "total_amount": 99.99
  }
  ```

### 6.3 更新购物车商品数量

- **端点**：`PUT /api/cart/items`
- **描述**：更新购物车中商品的数量
- **参数**：
  - `product_id` (int)：商品ID
  - `quantity` (int)：商品数量
- **响应**：
  ```json
  {
    "item": {
      "product_id": 1,
      "product_name": "Product Name",
      "price": 99.99,
      "quantity": 2,
      "image_url": "https://example.com/image.jpg"
    }
  }
  ```

### 6.4 删除购物车商品

- **端点**：`DELETE /api/cart/items`
- **描述**：从购物车中删除商品
- **参数**：
  - `product_id` (int)：商品ID
- **响应**：
  ```json
  {
    "success": true
  }
  ```

### 6.5 清空购物车

- **端点**：`DELETE /api/cart`
- **描述**：清空用户的购物车
- **响应**：
  ```json
  {
    "success": true
  }
  ```
