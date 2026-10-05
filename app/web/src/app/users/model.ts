import { TreeIndex } from '../org/treeIndex'
import type { User } from '../types'

export type ManagedUser = User & {
  display_name: string
  team_name?: string
  disabled: boolean
  created_at: string
  password_changed_at?: string
}

export type RoleRef = { id: string; name: string; system: boolean }
export type TeamRef = { id: string; name: string; parent_id: string }
export type Refs = { roles: RoleRef[]; teams: TeamRef[] }

export type Filters = { q: string; source: string; role: string; team: string; status: string }

export const NO_FILTERS: Filters = { q: '', source: '', role: '', team: '', status: '' }

export type UserStatus = 'locked' | 'must_change' | 'expired' | 'active'

export function statusOf(u: ManagedUser): UserStatus {
  if (u.disabled) return 'locked'
  if (u.must_change_password) return 'must_change'
  if (u.password_expired) return 'expired'
  return 'active'
}

export function usersQuery(f: Filters) {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(f)) if (v.trim()) params.set(k, v.trim())
  const s = params.toString()
  return s ? `/api/users?${s}` : '/api/users'
}

export type TeamOption = { id: string; name: string; depth: number; path: string }

export function teamOptions(teams: TeamRef[]): TeamOption[] {
  const index = new TreeIndex(teams)
  return index.flat().map(({ item, depth }) => ({ id: item.id, name: item.name, depth, path: index.path(item.id) }))
}

export function adminOf(u: { role: string }) {
  return u.role === 'admin'
}
