# API 文档

## 1. 认证接口

### 1.1 用户注册

- **端点**：`POST /api/register`
- **描述**：注册新用户
- **参数**：
  - `username` (string)：用户名
  - `password` (string)：密码
  - `email` (string)：邮箱
- **响应**：
  ```json
  {
    "user_id": 1,
    "token": "JWT_TOKEN"
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
    "token": "JWT_TOKEN"
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
- **参数**：
  - `name` (string)：商家名称
  - `contact_info` (string)：联系方式
- **响应**：
  ```json
  {
    "merchant": {
      "id": 1,
      "name": "Test Merchant",
      "contact_info": "test@merchant.com",
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
        "created_at": "2026-03-24T00:00:00Z"
      }
    ],
    "total": 1
  }
  ```

### 3.4 商家添加商品

- **端点**：`POST /api/merchants/products`
- **描述**：商家添加新商品
- **参数**：
  - `merchant_id` (int)：商家ID
  - `name` (string)：商品名称
  - `description` (string)：商品描述
  - `price` (float)：商品价格
  - `stock` (int)：商品库存
  - `category` (string)：商品分类
  - `image_url` (string)：商品图片URL
- **响应**：
  ```json
  {
    "product_id": 1
  }
  ```

### 3.5 商家删除商品

- **端点**：`DELETE /api/merchants/products`
- **描述**：商家删除自有商品
- **参数**：
  - `merchant_id` (int)：商家ID
  - `product_id` (int)：商品ID
- **响应**：
  ```json
  {
    "success": true
  }
  ```

## 4. 订单流程接口（v2.0新增）

### 4.1 创建订单

- **端点**：`POST /api/orders`
- **描述**：用户创建订单
- **参数**：
  - `items` (array)：订单项
    - `product_id` (int)：商品ID
    - `product_name` (string)：商品名称
    - `price` (float)：商品价格
    - `quantity` (int)：商品数量
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

## 5. 购物车接口

### 5.1 添加商品到购物车

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

### 5.2 获取购物车

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

### 5.3 更新购物车商品数量

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

### 5.4 删除购物车商品

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

### 5.5 清空购物车

- **端点**：`DELETE /api/cart`
- **描述**：清空用户的购物车
- **响应**：
  ```json
  {
    "success": true
  }
  ```
