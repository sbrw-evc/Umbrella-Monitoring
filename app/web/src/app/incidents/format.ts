import { SEVERITY_TONE, severityText, type Entry, type Severity } from './types'

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
  return SEVERITY_TONE[s as Severity] ?? SEVERITY_TONE.info
}

// minutes renders a wait in seconds: "30 s", "2 min".
function minutes(t: T, s: number) {
  if (!Number.isFinite(s)) return ''
  if (s % 60 === 0) return t('inc.dur.m', { n: s / 60 })
  return t('inc.dur.s', { n: s })
}

// entryText renders a timeline line from its code and arguments; unknown codes show as they are.
export function entryText(t: T, e: Entry, connectors: Record<string, string>) {
  const a = { ...(e.args ?? {}) }
  if (a.connector) a.connector = connectors[a.connector] ?? a.connector
  if (a.severity) a.severity = severityText(t, a.severity)
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
    case 'priority_set':
      if (a.event) a.event = severityText(t, a.event)
      if (a.from) a.from = severityText(t, a.from)
      if (a.impact) a.impact = t(`inc.impact.${a.impact}`)
      if (a.rule) key = 'tl.priority_set.rule'
      break
    case 'severity_raised':
      a.from = severityText(t, a.from)
      a.to = severityText(t, a.to)
      break
    case 'fallback':
      if (a.after_s !== undefined) a.after = minutes(t, Number(a.after_s))
      if (a.reason) {
        key = `tl.fallback.${a.reason}`
        if (a.reason === 'pd_not_taken' && a.after_s === '0') key += '.now'
        else if (a.reason === 'pd_off' && a.after_s && a.after_s !== '0') key += '.later'
      }
      break
    case 'fallback_cancelled':
      a.status = t(`inc.status.${a.status}`).toLowerCase()
      break
    case 'notify_followup':
    case 'notify_followup_failed':
      a.event = t(`fu.${a.event}`)
      break
    case 'pd_skipped':
      if (a.code === 'test') key = 'tl.pd_skipped.test'
      else if (a.code === 'below_threshold') {
        key = 'tl.pd_skipped.below_threshold'
        a.min = a.min ? severityText(t, a.min) : '—'
      }
      break
    case 'pd_failed':
      // Delivery errors are codes; old lines keep their text.
      if (a.code) {
        key = a.detail && a.code !== 'no_key' && a.code !== 'queue_full' && a.code !== 'breaker' ? 'tl.pd_failed.detail' : 'tl.pd_failed.code'
        const reason = t(`pd.err.${a.code}`)
        a.reason = reason === `pd.err.${a.code}` ? t('pd.err.error') : reason
      }
      if (a.action) a.action = t(`pd.action.${a.action}`)
      break
    case 'pd_accepted':
      if (a.action) a.action = t(`pd.action.${a.action}`)
      break
  }
  const out = t(key, a)
  return out === key ? [e.code, ...Object.entries(e.args ?? {}).map(([k, v]) => `${k}=${v}`)].join(' ') : out
}
