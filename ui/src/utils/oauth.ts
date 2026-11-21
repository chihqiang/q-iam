// OAuth2 授权相关工具：scope 解析与展示

// 常见 scope 的中文说明映射（未知 scope 回退显示原文）
const SCOPE_LABELS: Record<string, { label: string; desc: string }> = {
  openid: { label: '身份标识', desc: '获取你的唯一身份标识' },
  profile: { label: '基本信息', desc: '查看你的显示名等基本资料' },
  email: { label: '邮箱地址', desc: '查看你的邮箱' },
  phone: { label: '手机号', desc: '查看你的手机号' },
  'iam:read': { label: '只读权限', desc: '读取你的权限规则与数据范围' },
  'iam:write': { label: '读写权限', desc: '读取并管理你的权限规则' },
}

export interface ParsedScope {
  /** 原始 scope 字符串 */
  raw: string
  /** 展示名称 */
  label: string
  /** 说明 */
  desc: string
}

/**
 * 把空格分隔的 scope 字符串解析为权限列表。
 * 未传 scope（空串）时返回空数组，由调用方展示「基础信息」。
 */
export function parseScopes(scope: string): ParsedScope[] {
  const items = scope
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  return items.map((raw) => ({
    raw,
    ...(SCOPE_LABELS[raw] ?? { label: raw, desc: '该权限范围由应用自定义' }),
  }))
}
