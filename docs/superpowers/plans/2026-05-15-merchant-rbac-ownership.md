# 商家 RBAC 与资源归属 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为商家系统补齐最小可用的角色权限与资源归属边界，确保只有 merchant/admin 能执行写操作，且 merchant 只能管理自己的商家。

**Architecture:** 采用“注册时选择 customer/merchant 角色 + Merchant 绑定 owner_user_id”的最小方案。JWT 携带 `user_id` 与 `role`，网关负责认证与粗粒度角色拦截，商家服务基于真实用户与商家归属做最终授权判断，形成双层防线。

**Tech Stack:** Go 1.24、Gin、gRPC/protobuf、GORM、JWT、SQLite 测试库、React

---

## 文件结构

- `internal/auth/*`、`pkg/jwt/jwt.go`、`api/auth/*`：补充用户角色与 token claims
- `internal/merchant/*`、`api/merchant/*`：补充商家归属与服务端授权
- `cmd/api-gateway/main.go`：移动商家写路由并增加 `requireRole`
- `frontend/src/*`：保存角色并隐藏无权限入口
- `README.md`、`doc/*`：更新角色、权限与资源归属说明

## 任务拆解

1. 先补失败测试：JWT 角色、注册角色、网关 401/403、商家归属校验、公开查询接口。
2. 扩展 proto 与模型：`User.Role`、`Merchant.OwnerUserID`，并重新生成协议代码。
3. 实现认证链路：注册角色校验、JWT role claim、网关上下文透传。
4. 实现商家授权：创建时绑定 owner，写操作按 actor user 查询真实角色并校验归属，admin 放行。
5. 完成网关路由、前端最小适配与文档同步。
6. 执行全量验证：`go test ./...`。
