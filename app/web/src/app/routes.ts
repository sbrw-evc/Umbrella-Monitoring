import type { User } from './types'

export type Page = 'status' | 'settings' | 'profile'

export const PATHS: Record<Page, string> = { status: '/', settings: '/settings', profile: '/profile' }

const ADMIN_PAGES: Page[] = ['status', 'settings']

export function matches(path: string, base: string) {
  return path === base || (base !== '/' && path.startsWith(base + '/'))
}

export function pageFor(path: string, user: User): Page | null {
  const page = (Object.keys(PATHS) as Page[]).find((p) => matches(path, PATHS[p])) ?? null
  if (page && ADMIN_PAGES.includes(page) && user.role !== 'admin') return page === 'status' ? 'profile' : null
  return page
}
