import type { User } from './types'

export type Page = 'status' | 'profile'

export const PATHS: Record<Page, string> = { status: '/', profile: '/profile' }

export function pageFor(path: string, user: User): Page | null {
  if (path === PATHS.profile) return 'profile'
  if (path === PATHS.status) return user.role === 'admin' ? 'status' : 'profile'
  return null
}
