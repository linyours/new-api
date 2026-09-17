# 创建渠道 / 查询额度



Access Token 在控制台「个人设置」里生成，调用方角色必须是管理员。

```bash
export BASE_URL=http://xxxxx
export ADMIN_ACCESS_TOKEN='your-admin-access-token'
```

## 创建 Vertex 渠道

`POST /api/channel/`

request：

```bash
curl --url "$BASE_URL/api/channel/" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  --data-raw '{
    "mode": "single",
    "channel": {
      "type": 41,
      "name": "vertex-gemini",
      "key": "{\"type\":\"service_account\",\"project_id\":\"your-gcp-project\",\"private_key_id\":\"replace-me\",\"private_key\":\"-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----\\n\",\"client_email\":\"sa@your-gcp-project.iam.gserviceaccount.com\"}",
      "models": "gemini-2.5-flash,gemini-2.5-pro,gemini-2.5-flash-lite",
      "group": "default",
      "auto_ban": 1,
      "other": "{\"default\": \"global\"}",
      "settings": "{\"vertex_key_type\":\"json\"}"
    }
  }'
```

response：

```json
{
  "success": true,
  "message": ""
}
```

字段：

| 字段 | 含义 |
|---|---|
| `type` | `41` 是 Vertex AI |
| `mode` | 必须是 `single` / `batch` / `multi_to_single`。上单把 key 用 `single` |
| `key` | `settings.vertex_key_type=json` 时是服务账号 JSON 字符串 |
| `other` | 必填，必须是含 `default` 的地区 JSON，例如 `{"default":"global"}` |
| `group` | 字符串，逗号分隔；不要传 `groups` 数组 |

创建成功不返回渠道 ID。后面用渠道名或已知 ID 查询。

## 查询渠道信息（状态，额度，封禁原因）

`GET /api/channel/search`

`keyword` 可以是渠道 ID 或名称。`id = keyword` 或 `name LIKE %keyword%`。

request：

```bash
curl --url "$BASE_URL/api/channel/search?keyword=vertex-gemini&id_sort=true&p=1&page_size=20&type=41" \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN"
```

response：

```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "id": 4317,
        "type": 41,
        "key": "",
        "status": 1,
        "name": "vertex-gemini",
        "other": "{\"default\": \"global\"}",
        "balance": 0,
        "balance_updated_time": 0,
        "models": "gemini-2.5-flash,gemini-2.5-pro,gemini-2.5-flash-lite",
        "group": "default",
        "used_quota": 0,
        "auto_ban": 1,
        "other_info": "",
        "settings": "{\"vertex_key_type\":\"json\"}"
      }
    ],
    "total": 1,
    "type_counts": {
      "41": 1
    }
  }
}
```

已知 ID 时也可以：

```bash
curl --url "$BASE_URL/api/channel/4317" \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN"
```

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 4317,
    "type": 41,
    "status": 1,
    "name": "vertex-gemini",
    "used_quota": 0,
    "balance": 0,
    "other_info": "",
    "settings": "{\"vertex_key_type\":\"json\"}"
  }
}
```

字段：

| 字段 | 含义 |
|---|---|
| `status` | `1` 启用，`2` 手动禁用，`3` 自动封禁 |
| `other_info` | JSON 字符串。自动封禁时里面有 `status_reason`、`status_time` |
| `used_quota` | new-api 内部已用额度 |
| `balance` | 部分上游美元余额。Vertex 不支持，恒为 `0` |
| `key` | 列表/详情固定返回空字符串，不是密钥丢了 |
