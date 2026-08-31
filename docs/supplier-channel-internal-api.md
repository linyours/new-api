# 供应商渠道内部 API（V1）

本文档描述供应商系统调用的渠道管理接口。接口只用于服务端之间的内部调用，
不应直接暴露给浏览器或 C 端客户。

## 1. 基本信息

- Base URL 示例：`http://localhost:3000`
- 路径前缀：`/api/v1/internal/supplier/channels`
- 请求和响应格式：`application/json`
- 服务端配置：

```bash
SUPPLIER_INTERNAL_API_KEY=replace-with-a-long-random-secret
```

未配置 `SUPPLIER_INTERNAL_API_KEY` 时，所有供应商接口返回 `503`。

## 2. 鉴权与渠道归属

每个请求必须携带：

```http
Authorization: Bearer <SUPPLIER_INTERNAL_API_KEY>
X-Owner-User-Id: <供应商对应的用户 ID>
```

示例环境变量：

```bash
export BASE_URL=http://localhost:3000
export SUPPLIER_API_KEY='replace-with-a-long-random-secret'
export OWNER_USER_ID=123
```

归属规则：

1. `X-Owner-User-Id` 必须是正整数，并且对应的用户必须存在。
2. 创建渠道时，`owner_user_id` 只从请求头写入，不能由请求体指定。
3. 查询、修改和状态变更都强制匹配 `channel.id + owner_user_id`。
4. 非当前 owner 的渠道统一按不存在处理，返回 `404`。
5. 旧渠道和后台管理员创建的渠道默认 `owner_user_id=0`，供应商接口不能操作。
6. 所有响应都不会返回渠道密钥 `key`。

## 3. 接口一览

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/internal/supplier/channels` | 分页查询当前 owner 的渠道 |
| `GET` | `/api/v1/internal/supplier/channels/:id` | 查询当前 owner 的单个渠道 |
| `POST` | `/api/v1/internal/supplier/channels` | 创建渠道 |
| `PATCH` | `/api/v1/internal/supplier/channels/:id` | 修改渠道白名单字段 |
| `PUT` | `/api/v1/internal/supplier/channels/:id/status` | 启用或禁用渠道 |

## 4. 通用响应结构

成功：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

失败：

```json
{
  "success": false,
  "message": "错误说明"
}
```

渠道对象示例：

```json
{
  "id": 10,
  "owner_user_id": 123,
  "name": "supplier-openai",
  "type": 1,
  "base_url": "https://api.example.com",
  "models": "gpt-4o,gpt-4o-mini",
  "group": "default",
  "model_mapping": null,
  "test_model": "gpt-4o-mini",
  "auto_ban": 1,
  "status": 1,
  "created_time": 1786890000
}
```

> 响应中不会出现 `key`。

## 5. 分页查询渠道

```http
GET /api/v1/internal/supplier/channels?page=1&page_size=20
```

查询参数：

| 参数 | 类型 | 默认值 | 说明 |
|---|---:|---:|---|
| `page` | integer | `1` | 页码，小于 1 时按 1 |
| `page_size` | integer | `20` | 每页数量，最大 100 |

请求示例：

```bash
curl -sS "$BASE_URL/api/v1/internal/supplier/channels?page=1&page_size=20" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" | jq
```

响应示例：

```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "id": 10,
        "owner_user_id": 123,
        "name": "supplier-openai",
        "type": 1,
        "base_url": "https://api.example.com",
        "models": "gpt-4o,gpt-4o-mini",
        "group": "default",
        "model_mapping": null,
        "test_model": "gpt-4o-mini",
        "auto_ban": 1,
        "status": 1,
        "created_time": 1786890000
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

## 6. 查询单个渠道

```http
GET /api/v1/internal/supplier/channels/:id
```

请求示例：

```bash
curl -sS "$BASE_URL/api/v1/internal/supplier/channels/10" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" | jq
```

当前 owner 不拥有该渠道时：

```json
{
  "success": false,
  "message": "channel not found"
}
```

HTTP 状态码为 `404`。

## 7. 创建渠道

```http
POST /api/v1/internal/supplier/channels
Content-Type: application/json
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 渠道名称 |
| `type` | integer | 是 | 渠道类型，必须大于 0 |
| `key` | string | 是 | 上游密钥，只写入，不在响应中返回 |
| `base_url` | string/null | 否 | 上游地址 |
| `models` | string | 是 | 模型列表，逗号分隔 |
| `group` | string | 是 | 分组列表，逗号分隔 |
| `model_mapping` | string/null | 否 | 模型映射 JSON 字符串 |
| `test_model` | string/null | 否 | 测试模型 |
| `auto_ban` | integer/null | 否 | 是否自动禁用；省略时为 `1` |

请求示例：

```bash
curl -sS -X POST "$BASE_URL/api/v1/internal/supplier/channels" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "supplier-openai",
    "type": 1,
    "key": "sk-example",
    "base_url": "https://api.example.com",
    "models": "gpt-4o,gpt-4o-mini",
    "group": "default",
    "test_model": "gpt-4o-mini",
    "auto_ban": 1
  }' | jq
```

成功时返回 HTTP `201`。创建过程会：

1. 写入请求头对应的 `owner_user_id`；
2. 创建渠道；
3. 创建对应的 abilities；
4. 刷新渠道缓存。

## 8. 修改渠道

```http
PATCH /api/v1/internal/supplier/channels/:id
Content-Type: application/json
```

只允许修改下列字段：

```text
name
type
key
base_url
models
group
model_mapping
test_model
auto_ban
```

以下字段不能通过本接口修改：

```text
owner_user_id
status
cost_price
priority
weight
balance
used_quota
param_override
header_override
setting
settings
channel_info
```

请求示例：

```bash
curl -sS -X PATCH "$BASE_URL/api/v1/internal/supplier/channels/10" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "supplier-openai-updated",
    "key": "sk-new-example",
    "models": "gpt-4o-mini"
  }' | jq
```

PATCH 语义：未提交的白名单字段保持不变。修改成功后会同步 abilities 并刷新
渠道缓存。响应不会回传新旧密钥。

## 9. 启用或禁用渠道

```http
PUT /api/v1/internal/supplier/channels/:id/status
Content-Type: application/json
```

支持的状态：

| 状态 | 值 | 说明 |
|---|---:|---|
| 启用 | `1` | 渠道参与选渠 |
| 手动禁用 | `2` | 渠道不参与选渠 |

禁用示例：

```bash
curl -sS -X PUT "$BASE_URL/api/v1/internal/supplier/channels/10/status" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" \
  -H "Content-Type: application/json" \
  -d '{"status": 2}' | jq
```

启用示例：

```bash
curl -sS -X PUT "$BASE_URL/api/v1/internal/supplier/channels/10/status" \
  -H "Authorization: Bearer $SUPPLIER_API_KEY" \
  -H "X-Owner-User-Id: $OWNER_USER_ID" \
  -H "Content-Type: application/json" \
  -d '{"status": 1}' | jq
```

响应示例：

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 10,
    "status": 2,
    "changed": true
  }
}
```

`changed=false` 表示渠道已经是目标状态。状态变更会：

1. 同步 ability 启用状态；
2. 刷新渠道缓存；
3. 触发渠道状态变更 Hook。

## 10. 常见错误

| HTTP 状态码 | message 示例 | 原因 |
|---:|---|---|
| `400` | `invalid X-Owner-User-Id` | owner 请求头缺失或格式错误 |
| `400` | `owner user not found` | owner 对应的用户不存在 |
| `400` | `name, type, key, models and group are required` | 创建参数缺失 |
| `400` | `invalid status` | 状态不是 1 或 2 |
| `401` | `invalid supplier internal credential` | 内部密钥错误 |
| `404` | `channel not found` | 渠道不存在或不属于当前 owner |
| `503` | `supplier internal API is not configured` | 服务端未配置内部密钥 |

## 11. 安全建议

1. 使用不少于 32 字节的随机 `SUPPLIER_INTERNAL_API_KEY`。
2. 仅在可信内网开放该接口，并使用 HTTPS 或服务网格 mTLS。
3. 不要把内部密钥或渠道密钥写入前端、日志和错误信息。
4. 供应商系统完成自身用户鉴权后，再映射并传递可信的
   `X-Owner-User-Id`。
5. 定期轮换内部密钥，并限制供应商系统到该接口的网络访问范围。
