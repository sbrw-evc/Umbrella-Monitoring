import { Database, KeyRound, ShieldCheck, Users, type LucideIcon } from 'lucide-react'
import type { ComponentType } from 'react'
import { PATHS } from '../routes'
import { DirectorySettings } from './directory/DirectorySettings'
import { OpenBaoSettings } from './openbao/OpenBaoSettings'
import { PolicySettings } from './policy/PolicySettings'
import { PostgresSettings } from './postgres/PostgresSettings'

export type Section = { id: string; label: string; icon: LucideIcon; Component: ComponentType }

export const SECTIONS: Section[] = [
  { id: 'postgresql', label: 'settings.postgres', icon: Database, Component: PostgresSettings },
  { id: 'openbao', label: 'settings.openbao', icon: KeyRound, Component: OpenBaoSettings },
  { id: 'ldap', label: 'settings.directory', icon: Users, Component: DirectorySettings },
  { id: 'password-policy', label: 'settings.policy', icon: ShieldCheck, Component: PolicySettings },
]

export function sectionPath(s: Section) {
  return `${PATHS.settings}/${s.id}`
}

export function sectionFor(path: string): Section | null {
  return SECTIONS.find((s) => sectionPath(s) === path) ?? null
}
