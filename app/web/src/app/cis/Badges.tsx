import { useT } from '../../i18n'
import { strings } from './strings'
import type { CI } from './types'

export function StatusPill({ status }: { status: string }) {
  const t = useT(strings)
  const tone = status === 'active' ? 'ok' : status === 'failed' ? 'error' : status === 'offline' || status === 'decommissioning' ? 'warn' : 'off'
  const label = t(`ci.status.${status}`)
  return <span className={`pill pill-${tone}`}>{label.startsWith('ci.status.') ? status : label}</span>
}

export function SourcePill({ ci }: { ci: CI }) {
  const t = useT(strings)
  const key = ci.source === 'netbox' ? 'ci.source.netbox' : ci.netbox ? 'ci.source.local.registered' : 'ci.source.local'
  return <span className={`pill ci-source ci-source-${ci.source}`}>{t(key)}</span>
}
