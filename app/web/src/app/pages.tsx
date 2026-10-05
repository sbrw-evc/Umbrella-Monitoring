import type { ComponentType } from 'react'
import { Activity, Briefcase, Database, KeyRound, Network, ShieldCheck, UserCog, Users, UsersRound, type LucideIcon } from 'lucide-react'
import { RolesPage } from './roles/RolesPage'
import { ServicesPage } from './services/ServicesPage'
import { DirectorySettings } from './settings/directory/DirectorySettings'
import { OpenBaoSettings } from './settings/openbao/OpenBaoSettings'
import { PolicySettings } from './settings/policy/PolicySettings'
import { PostgresSettings } from './settings/postgres/PostgresSettings'
import { SystemStatus } from './SystemStatus'
import { TeamsPage } from './teams/TeamsPage'
import { UsersPage } from './users/UsersPage'

export type Group = 'main' | 'org' | 'settings'

export type PageDef = {
  id: string
  path: string
  group: Group
  icon: LucideIcon
  Component: ComponentType
  subtitle?: string
}

export const HOME_PATH = '/'
export const PROFILE_PATH = '/profile'
export const SETTINGS_PATH = '/settings'

export const PAGES: PageDef[] = [
  { id: 'status', path: '/status', group: 'main', icon: Activity, Component: SystemStatus },
  { id: 'users', path: '/users', group: 'org', icon: Users, Component: UsersPage, subtitle: 'page.users.subtitle' },
  { id: 'roles', path: '/roles', group: 'org', icon: UserCog, Component: RolesPage, subtitle: 'page.roles.subtitle' },
  { id: 'teams', path: '/teams', group: 'org', icon: UsersRound, Component: TeamsPage, subtitle: 'page.teams.subtitle' },
  { id: 'services', path: '/services', group: 'org', icon: Briefcase, Component: ServicesPage, subtitle: 'page.services.subtitle' },
  { id: 'settings.postgres', path: '/settings/postgresql', group: 'settings', icon: Database, Component: PostgresSettings, subtitle: 'page.settings.subtitle' },
  { id: 'settings.openbao', path: '/settings/openbao', group: 'settings', icon: KeyRound, Component: OpenBaoSettings, subtitle: 'page.settings.subtitle' },
  { id: 'settings.ldap', path: '/settings/ldap', group: 'settings', icon: Network, Component: DirectorySettings, subtitle: 'page.settings.subtitle' },
  {
    id: 'settings.policy',
    path: '/settings/password-policy',
    group: 'settings',
    icon: ShieldCheck,
    Component: PolicySettings,
    subtitle: 'page.settings.subtitle',
  },
]

export const GROUPS: Group[] = ['main', 'org', 'settings']

export type Can = (perm: string) => boolean

export const viewPerm = (p: PageDef) => `${p.id}:view`

export function visiblePages(can: Can) {
  return PAGES.filter((p) => can(viewPerm(p)))
}

export type Resolved = { kind: 'page'; page: PageDef } | { kind: 'profile' } | { kind: 'redirect'; to: string }

export function resolve(path: string, can: Can): Resolved {
  if (path === PROFILE_PATH) return { kind: 'profile' }
  const visible = visiblePages(can)
  const page = visible.find((p) => p.path === path)
  if (page) return { kind: 'page', page }
  if (path === SETTINGS_PATH || path.startsWith(SETTINGS_PATH + '/')) {
    const first = visible.find((p) => p.group === 'settings')
    if (first) return { kind: 'redirect', to: first.path }
  }
  return { kind: 'redirect', to: visible[0]?.path ?? PROFILE_PATH }
}
