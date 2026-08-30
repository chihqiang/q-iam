# q-iam 权限设计

本文档描述 q-iam 的权限模型与实现机制：从**策略（Policy）** 到**授权（Grant）** 再到**判定（Check）** 的完整设计，以及数据权限、缓存与前端过滤机制。

---

## 1. 总体架构

q-iam 采用类云厂商 IAM 的三层权限模型，核心思想是 **"策略" 与 "主体" 解耦，通过 "授权" 建立关联**：

```text
┌─────────────────────────────────────────────────────────────┐
│                        三层权限模型                          │
│                                                             │
│  ① 语句池  Statement（授权语句，独立菜单管理）── DataScope  │
│            （共享引用，可被多个策略关联）    (数据范围)       │
│              ▲ 多对多关联（q_iam_policy_statements）        │
│  ① 策略层  Policy（策略只负责关联语句，不内嵌编辑）         │
│                                                             │
│  ② 授权层  PolicyAttachment（策略 ↔ 主体 的绑定关系）       │
│            主体：账号 account / 账号组 group / 应用 app       │
│                                                             │
│  ③ 判定层  PermissionSet.Check(action)                     │
│            Deny 优先 → Allow 匹配 → 拒绝                    │
└─────────────────────────────────────────────────────────────┘
```

设计原则：

- **语句池共享引用**：授权语句独立成池管理（独立菜单），一条语句可被多个策略关联；语句变更后，所有关联它的策略同步生效（权限缓存联动失效）
- **策略只负责关联**：策略新增/编辑仅选择关联已有语句（`statement_ids`），不内嵌编辑语句
- **策略可复用**：一份策略可授权给多个主体（账号/账号组/应用）
- **权限叠加**：账号最终权限 = 直接绑定策略 + 所属账号组绑定策略
- **显式拒绝优先**：`Deny` 优先级高于 `Allow`，安全兜底
- **动作与资源分离**：动作（`iam:account:read`）决定"能不能做"，数据范围（`data_scopes`）决定"能看哪些数据"

---

## 2. 核心实体（数据模型）

### 2.1 策略 Policy（`q_iam_policies`）

| 字段 | 说明 |
| --- | --- |
| `name` | 策略名（唯一） |
| `type` | `system`（系统内置，不可改/删）\| `custom`（自定义） |
| `status` | 启用/禁用（禁用后授权关系不生效） |
| `version` | 策略版本 |
| `statements` | 关联的授权语句（多对多，经关联表 `q_iam_policy_statements`） |

系统内置策略（`type=system`，不可修改/删除）：

| 策略名 | 规则 | 说明 |
| --- | --- | --- |
| `AdministratorAccess` | `Allow *` | 全权限，种子数据自动创建并授权给内置 `admin` 账号 |
| `ConsoleAccess` | 控制台各模块动作（账号/账号组/权限策略/授权语句/授权/应用/审计/数据清理） | 供普通账号/账号组/应用授权，以访问管理控制台各模块 |

### 2.2 授权语句 Statement（`q_iam_statements`，语句池）

授权语句独立成池管理（独立菜单）：

```json
{
  "effect": "Allow",          // Allow | Deny
  "action": "iam:account:read,iam:account:write",  // 逗号分隔，支持 *
  "description": "只读账号",
  "resource": "*",           // 资源（支持通配，默认 *）
  "scopes": []                // 数据范围（见 2.3）
}
```

- 可被多个策略共享引用（多对多，关联表 `q_iam_policy_statements` 仅存 `policy_id` + `statement_id`）
- 修改语句后，所有关联它的策略对应主体的权限缓存即时失效
- 被策略关联的语句禁止删除（须先解除全部关联）；系统内置语句（`created_by=0`）不可删除
- 非 admin 仅可管理本人创建的语句，且只能关联本人创建或系统内置的语句

### 2.3 数据范围 DataScope（`q_iam_statement_scopes`）

定义该规则能作用于哪部分数据（数据权限），挂在 Statement 之下：

| `scope_type` | 含义 | 关键字段 |
| --- | --- | --- |
| `all` | 全部数据（无限制） | — |
| `group` | 本用户分组的数据（多行=多组并集） | `group_id` |
| `self` | 仅本人数据（按归属字段） | `owner_field`（值为当前账号 ID） |
| `attribute` | 按数据属性/标签过滤（多行=OR） | `attr_key` + `attr_value` |

### 2.4 授权关系 PolicyAttachment（`q_iam_policy_attachments`）

将策略绑定到主体：

| 字段 | 说明 |
| --- | --- |
| `principal_type` | 主体类型：`account` \| `group` \| `app`（DB CHECK 约束强校验） |
| `principal_id` | 主体 ID |
| `policy_id` | 绑定的策略 ID |
| `created_by` | 授权人账号 ID |

---

## 3. 动作（Action）命名规范

动作采用冒号分层的命名（`模块:动作[:子动作]`），支持 `*` 通配：

| 动作 | 含义 |
| --- | --- |
| `iam:account:read` | 查看账号 |
| `iam:account:write` | 创建/修改/删除账号 |
| `iam:group:read` / `iam:group:write` | 账号组读写 |
| `iam:policy:read` / `iam:policy:write` | 策略读写 |
| `iam:grant` | 授权管理（绑定/解绑策略） |
| `iam:app:read` / `iam:app:write` | 应用读写 |
| `iam:audit:read` | 查看操作审计 |
| `iam:system:cleanup` | 历史数据清理 |

通配示例：`iam:*` 匹配所有 `iam` 模块动作；`*` 匹配全部动作。

---

## 4. 权限判定语义

### 4.1 判定算法（`PermissionSet.Check`）

```text
Check(action):
    allowed = false
    for rule in rules:
        if 规则 action 不匹配 action: continue
        if rule.effect == Deny: return false   # 显式拒绝优先
        allowed = true
    return allowed
```

| 场景 | 结果 |
| --- | --- |
| 无任何规则匹配 | 拒绝 |
| 有 `Allow` 匹配，无 `Deny` 匹配 | 允许 |
| 有 `Deny` 匹配（无论是否有 Allow） | **拒绝** |

> **Deny 优先**是安全设计：即使某账号同时被授予了允许与拒绝，拒绝生效。

### 4.2 通配匹配（`globMatch`）

动作匹配支持 `*` 通配（匹配任意字符序列），非正则：

- `iam:account:read` 精确匹配 `iam:account:read`
- `iam:*` 匹配所有以 `iam:` 开头的动作
- `*` 匹配全部
- `iam:account:read,iam:account:write` 逗号列表任一项命中即匹配

---

## 5. 授权模型

### 5.1 三种主体

| 主体类型 | 说明 |
| --- | --- |
| `account` | 账号（直接授权） |
| `group` | 账号组（组内所有账号生效） |
| `app` | 应用（OAuth2 客户端，client_credentials 令牌） |

### 5.2 权限来源聚合

账号的最终权限 = **直接绑定策略** + **所属启用账号组绑定的策略**：

```text
直接:  账号 ──绑定──> 策略A
间接:  账号 ──属于──> 账号组 ──绑定──> 策略B
                                ┌
最终权限 = 策略A 的规则 ∪ 策略B 的规则
```

- 禁用的账号组不参与聚合
- 禁用的策略不参与聚合
- 应用主体（`app`）按单一主体查询其绑定的策略

### 5.3 内置管理员

内置 `admin` 账号在权限中间件中**硬编码放行**（拥有全部权限），不参与 `PermissionSet` 判定；其他账号一律按策略集合校验。同时 `admin` 账号**不可删除**（删除保护，见 `logic/account.go`）。

---

## 6. 权限解析与执行流程

```text
请求 → Auth 中间件（JWT）→ LoadAccount 中间件（加载账号）
     → Permission 中间件（动作级校验）
          ├─ admin？→ 放行
          ├─ 加载 PermissionSet（含缓存）
          └─ Check(action)？→ 通过 / 403
     → Handler（业务处理）
```

- **动作级校验**：`middleware.Permission` 校验当前账号是否拥有路由声明的动作权限（如 `iam:account:read`）
- **数据级过滤**：`data_scopes` 由资源方（子系统）通过 `/auth/data-permissions` 拉取后自行做行级过滤

路由声明示例（`route/route.go`）：

```go
// 读操作
authed.AddRoutes([]httpx.Route{
    {Method: http.MethodGet, Path: "/accounts", Handler: accountHandler.List},
}, httpx.WithMiddleware(perm("iam:account:read")))

// 写操作（审计 + 权限）
authed.AddRoutes([]httpx.Route{
    {Method: http.MethodPost, Path: "/accounts", Handler: accountHandler.Create},
}, httpx.WithMiddlewares(auditW("account", "", perm("iam:account:write"))...))
```

---

## 7. 权限缓存

权限集缓存**始终启用**，后端统一使用 infra-go `cache.Cache` 接口注入（见 `svc/servicecontext.go`）：

| 后端 | 适用场景 | 说明 |
| --- | --- | --- |
| `MemCache`（未配置 Redis） | 单机 | 进程内实现（惰性删除 + 过期扫描 + LRU），不跨节点 |
| `RedisCache`（配置 Redis 后） | 多节点水平扩展 | 基于 Redis，多节点共享，高并发 |

| 项 | 值 |
| --- | --- |
| 缓存键 | `perm:acct:{id}` / `perm:group:{id}` / `perm:app:{id}` |
| TTL | 60 秒（兜底） |
| 主动失效 | 权限来源变更时立即失效（与缓存后端无关，MemCache/RedisCache 均生效） |

**失效触发点**（权限来源变化）：

| 操作 | 失效对象 |
| --- | --- |
| 授权/解绑（`/grants`） | 该主体；group 连带组内全部账号 |
| 账号组成员变更 | 组（连带新成员）+ 被移除/被替换的旧成员 |
| 账号的组关联变更 / 账号删除 | 该账号 |
| 账号组状态变更（启→禁） | 该组（连带组内账号） |
| 策略规则修改/删除 | 所有引用该策略的主体 |

> 无论使用 `MemCache` 还是 `RedisCache`，授权/策略变更都会主动失效对应缓存，保证即时生效；TTL 仅作为异常兜底。
>
> 注意：未配置 Redis 时 `MemCache` 为进程内实现，多实例部署下权限缓存失效不跨节点，生产多节点部署请配置 Redis。

---

## 8. 对外权限接口

| 接口 | 认证 | 说明 |
| --- | --- | --- |
| `GET /auth/me` | 用户 | 返回当前账号信息 + 生效权限规则（`permissions`，含来源策略名），**供前端过滤菜单/按钮** |
| `GET /auth/data-permissions` | 用户/应用 | 权限规则 + `data_scopes` + 主体所属组，**供子系统做数据级过滤** |
| `GET /oauth/userinfo` | 用户/应用令牌 | 用户/应用信息 + 权限规则（用户令牌返回用户权限，应用令牌返回应用权限） |
| `POST /grants` | `iam:grant` | 绑定策略到主体 |
| `DELETE /grants` | `iam:grant` | 解绑策略 |
| `GET /grants/principals/{type}/{id}` | `iam:grant` | 查主体已绑定策略 |
| `GET /grants/policies/{id}` | `iam:grant` | 查策略被哪些主体绑定 |
| `POST /cleanup/history` | `iam:system:cleanup` | 清理历史数据（审计日志/过期刷新令牌） |

### 前端菜单过滤

> 管理控制台为 q-iam 内置前端（本仓库 `ui/` 目录，构建产物经 Go embed 打进二进制）。

前端侧边栏菜单由路由表生成（`meta.action`），根据 `/auth/me` 返回的 `permissions` 过滤，示意代码见前端仓库 `src/layouts/AdminLayout.vue`：

---

## 9. 数据权限使用示例

子系统调用 `/auth/data-permissions` 获取当前主体的数据范围，对查询附加行级过滤条件：

```json
{
  "subject_type": "user",
  "user": { "account_id": 2, "account_name": "alice", "group_ids": [1, 3] },
  "permissions": [
    {
      "effect": "Allow",
      "action": "iam:order:read",
      "source": "OrderPolicy",
      "data_scopes": [
        { "scope_type": "group", "group_id": 3 },
        { "scope_type": "self", "owner_field": "owner_id" }
      ]
    }
  ]
}
```

资源方据此生成过滤条件（示意）：

| scope_type | 生成条件（示意 SQL） |
| --- | --- |
| `group` | `WHERE group_id IN (1, 3)`（取所属组并集） |
| `self` | `WHERE owner_id = 2`（当前账号 ID） |
| `attribute` | `WHERE attr_key = 'x' AND attr_value = 'y'`（多行 OR） |
| `all` | 无过滤 |

---

## 10. 最佳实践与安全注意

1. **最小权限**：为账号/组只授予其职责所需的最小动作集合，避免 `*` 全开。
2. **优先用账号组**：同一角色的一批账号用账号组统一授权，避免逐个账号重复绑定。
3. **系统策略保护**：`AdministratorAccess` 为 `system` 类型，后端禁止修改/删除，防止锁死系统。
4. **Deny 谨慎使用**：Deny 优先级最高，误配可能导致权限被误回收。
5. **禁用优先于删除**：临时收回权限用"禁用策略/账号"，可随时恢复；删除会级联清理授权关系。
6. **数据权限必配**：涉及敏感数据（订单/用户信息）的接口，除动作级权限外应叠加 `data_scopes` 数据级过滤。
7. **权限变更即时性**：权限集缓存（`MemCache` 或 `RedisCache`）在授权/策略变更时主动失效，TTL 仅兜底，变更即时生效，无需担心滞后。

---

## 11. 权限相关代码索引

| 文件 | 职责 |
| --- | --- |
| `model/policy.go` | Policy / Statement / DataScope / PolicyStatementLink / PolicyAttachment / PrincipalType 模型 |
| `logic/permission.go` | 权限加载（LoadPermissionSet）、判定（Check）、通配匹配、缓存 |
| `logic/statement.go` | 授权语句（语句池）CRUD：独立菜单管理，被策略关联禁止删除 |
| `logic/policy.go` | 策略 CRUD + 关联语句（statement_ids，共享引用） |
| `logic/grant.go` | 授权/解绑（Grant / Revoke）、失效权限缓存 |
| `middleware/permission.go` | 动作级权限校验中间件（admin 放行） |
| `db/migrate.go` | 种子数据（内置 admin + AdministratorAccess/ConsoleAccess 系统策略） |
| `svc/servicecontext.go` | 服务上下文：集中装配权限逻辑与 infra-go cache（MemCache / RedisCache）缓存注入 |
| `route/route.go` | 路由声明各动作所需权限 |
