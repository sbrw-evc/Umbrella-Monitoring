import type { Entry, Severity } from './types'

type T = (key: string, vars?: Record<string, string | number>) => string

export function ago(t: T, v: string | undefined, now: number) {
  if (!v) return ''
  const s = Math.max(0, Math.round((now - new Date(v).getTime()) / 1000))
  if (s < 60) return t('inc.ago.s')
  if (s < 3600) return t('inc.ago.m', { n: Math.floor(s / 60) })
  if (s < 86400) return t('inc.ago.h', { n: Math.floor(s / 3600) })
  return t('inc.ago.d', { n: Math.floor(s / 86400) })
}

export function severityTone(s: Severity | string) {
  return s === 'critical' ? 'critical' : s === 'error' ? 'error' : s === 'warning' ? 'warn' : 'info'
}

// entryText renders a timeline line from its code and arguments; unknown codes show as they are.
export function entryText(t: T, e: Entry, connectors: Record<string, string>) {
  const a = { ...(e.args ?? {}) }
  if (a.connector) a.connector = connectors[a.connector] ?? a.connector
  if (a.severity) a.severity = t(`inc.sev.${a.severity}`)
  if (a.status && e.code === 'event') a.status = t(`inc.src.${a.status}`)
  let key = `tl.${e.code}`
  switch (e.code) {
    case 'resolved':
      if (a.why) key = `tl.resolved.${a.why}`
      break
    case 'event':
      if (a.value) key = 'tl.event.value'
      break
    case 'routed':
      if (a.via === 'ci_owners') key = 'tl.routed.owners'
      else if (a.via === 'none') key = 'tl.routed.none'
      else if (a.services) key = 'tl.routed.services'
      break
    case 'severity_raised':
      a.from = t(`inc.sev.${a.from}`)
      a.to = t(`inc.sev.${a.to}`)
      break
  }
  const out = t(key, a)
  return out === key ? [e.code, ...Object.entries(e.args ?? {}).map(([k, v]) => `${k}=${v}`)].join(' ') : out
}
