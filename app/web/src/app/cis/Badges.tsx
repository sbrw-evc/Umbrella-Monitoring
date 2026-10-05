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

// MonitoringCell: the systems that cover the item, or a warning when none does.
export function MonitoringCell({ ci }: { ci: CI }) {
  const t = useT(strings)
  if (ci.monitoring.length === 0) return ci.not_monitored ? <span className="pill pill-warn">{t('ci.mon.not')}</span> : <span className="muted">{t('ci.none')}</span>
  const names = [...new Set(ci.monitoring.map((m) => m.source_name))]
  const tone = ci.monitoring.some((m) => m.state === 'down') ? 'error' : ci.monitoring.some((m) => m.state === 'partial') ? 'warn' : 'ok'
  return <span className={`pill pill-${tone}`}>{names.join(', ')}</span>
}

export function MonitorState({ state }: { state: string }) {
  const t = useT(strings)
  const tone = state === 'up' ? 'ok' : state === 'down' ? 'error' : state === 'partial' ? 'warn' : 'off'
  return <span className={`pill pill-${tone}`}>{t(`ci.mon.state.${state}`)}</span>
}
