import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import type { Component } from 'vue'
import { useAuthStore } from '@/stores/auth'
import {
  ShieldCheck,
  UserCog,
  Users,
  KeyRound,
  Blocks,
  ScrollText,
} from '@lucide/vue'

// ===== 管理后台路由（单一数据源）=====
// 侧边栏菜单由路由表自动生成，无需重复维护。
// 每个子路由的 meta 携带侧边栏信息：
//   - title:     页面标题 / 菜单显示名
//   - icon:      侧边栏图标
//   - menuGroup: 侧边栏分组标题（有该字段的路由才显示在菜单中）
const adminChildren: RouteRecordRaw[] = [
  {
    path: 'accounts',
    name: 'accounts',
    component: () => import('@/views/accounts/index.vue'),
    meta: { title: '账号管理', icon: Users, menuGroup: '身份管理', action: 'iam:account:read' },
  },
  {
    path: 'groups',
    name: 'groups',
    component: () => import('@/views/groups/index.vue'),
    meta: { title: '账号组', icon: UserCog, menuGroup: '身份管理', action: 'iam:group:read' },
  },
  {
    path: 'policies',
    name: 'policies',
    component: () => import('@/views/policies/index.vue'),
    meta: { title: '权限策略', icon: ShieldCheck, menuGroup: '权限管理', action: 'iam:policy:read' },
  },
  {
    path: 'grants',
    name: 'grants',
    component: () => import('@/views/grants/index.vue'),
    meta: { title: '授权管理', icon: KeyRound, menuGroup: '权限管理', action: 'iam:grant' },
  },
  {
    path: 'apps',
    name: 'apps',
    component: () => import('@/views/apps/index.vue'),
    meta: { title: '应用管理', icon: Blocks, menuGroup: '集成管理', action: 'iam:app:read' },
  },
  {
    path: 'audit',
    name: 'audit',
    component: () => import('@/views/audit/index.vue'),
    meta: { title: '操作审计', icon: ScrollText, menuGroup: '安全审计', action: 'iam:audit:read' },
  },
  {
    // 个人中心：不进侧边栏菜单（meta 不带 menuGroup/icon），通过顶部栏用户下拉进入
    path: 'profile',
    name: 'profile',
    component: () => import('@/views/profile/index.vue'),
    meta: { title: '个人中心' },
  },
]

// ===== 侧边栏菜单（由路由表生成）=====

export interface MenuEntry {
  /** 完整路由路径，如 /accounts */
  path: string
  /** 菜单显示名 */
  label: string
  /** 侧边栏图标 */
  icon: Component
  /** 所需权限动作（如 iam:account:read），无则无需权限 */
  action?: string
}

export interface MenuGroup {
  /** 分组标题 */
  label: string
  items: MenuEntry[]
}

// 菜单分组顺序（决定侧边栏展示顺序）
const menuGroupOrder = ['身份管理', '权限管理', '集成管理', '安全审计']

/** 从路由表生成侧边栏菜单分组 */
export function getMenuGroups(): MenuGroup[] {
  const byGroup = new Map<string, MenuEntry[]>()
  for (const child of adminChildren) {
    const meta = child.meta as
      | { title?: string; icon?: Component; menuGroup?: string; action?: string }
      | undefined
    if (!meta?.menuGroup || !meta.icon) continue
    const items = byGroup.get(meta.menuGroup) ?? []
    items.push({
      path: '/' + child.path,
      label: meta.title ?? child.path,
      icon: meta.icon,
      action: meta.action,
    })
    byGroup.set(meta.menuGroup, items)
  }
  // 按指定顺序返回分组；未在 order 中的排最后
  const groups: MenuGroup[] = []
  for (const label of menuGroupOrder) {
    const items = byGroup.get(label)
    if (items?.length) groups.push({ label, items })
    byGroup.delete(label)
  }
  for (const [label, items] of byGroup) {
    groups.push({ label, items })
  }
  return groups
}

// 路由表
const routes: RouteRecordRaw[] = [
  {
    // 统一认证页：登录/注册/OAuth 授权确认由 URL 参数切换
    //   /auth                        → 登录（默认）
    //   /auth?mode=register          → 注册
    //   /auth?client_id=...&redirect_uri=... → 授权确认（未登录时页内登录）
    path: '/auth',
    name: 'auth',
    component: () => import('@/views/auth/index.vue'),
    meta: { title: '认证', public: true },
  },
  {
    // 无控制台权限提示页（注册账号仅用于 OAuth2 授权登录）
    path: '/no-console',
    name: 'no-console',
    component: () => import('@/views/no-console/index.vue'),
    meta: { title: '无控制台权限', public: true },
  },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    redirect: '/accounts',
    meta: { requiresAuth: true },
    children: adminChildren,
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/accounts',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 全局前置守卫：登录态 + 控制台访问权限控制
router.beforeEach((to) => {
  const auth = useAuthStore()

  // 需要登录的页面：未登录跳转统一认证页（记录来源，登录后回跳）
  if (to.meta.requiresAuth && !auth.isLoggedIn) {
    return { path: '/auth', query: { mode: 'login', redirect: to.fullPath } }
  }

  // 已登录但无控制台权限：不能进入管理页（认证页 public，不受此限制）
  if (to.meta.requiresAuth && auth.isLoggedIn && !auth.canEnterConsole) {
    return { path: '/no-console' }
  }

  // 已登录访问认证页：登录/注册模式进入管理页或提示页；授权模式（带 client_id）放行展示授权确认
  if (to.path === '/auth' && auth.isLoggedIn) {
    const isAuthorize = !!to.query.client_id
    if (!isAuthorize) {
      return auth.canEnterConsole ? { path: '/accounts' } : { path: '/no-console' }
    }
  }

  return true
})

export default router
