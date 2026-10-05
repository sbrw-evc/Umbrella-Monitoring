export const SEVERITIES = ['critical', 'error', 'warning', 'info'] as const
export const STATUSES = ['active', 'open', 'acknowledged', 'resolved', 'all'] as const
export const METHODS = ['red', 'use', 'other'] as const

export type Severity = (typeof SEVERITIES)[number]

export type Source = {
  connector_id: string
  key: string
  status: 'firing' | 'resolved'
  severity: Severity
  title: string
  value?: string
  first_seen: string
  last_seen: string
}

export type Ref = { id: string; name: string }

export type Person = { user_id: string; name: string; email?: string; telegram?: string; role?: string }

export type Route = { services: Ref[]; team?: Ref; people: Person[]; owners: Person[]; via: 'service' | 'ci_owners' | 'none'; at: string }

export type PD = {
  state: 'pending' | 'accepted' | 'acked' | 'failed' | 'skipped'
  key: string
  route?: string
  error?: string
  retry?: string
  attempt_at?: string
  incident_id?: string
  incident_url?: string
}

export type Incident = {
  id: string
  title: string
  ci_id?: string
  ci_name: string
  ci_kind?: string
  signal: string
  method: string
  severity: Severity
  status: 'open' | 'acknowledged' | 'resolved'
  sources: Record<string, Source>
  labels: Record<string, string>
  count: number
  first_seen: string
  opened_at: string
  last_seen: string
  resolved_at?: string
  resolved_by?: string
  acked_by?: string
  acked_at?: string
  suppressed: boolean
  maintenance_id?: string
  route: Route
  pd: PD
  fallback: boolean
  fallback_at?: string
  related_id?: string
}

export type Counts = {
  active: number
  open: number
  acknowledged: number
  by_severity: Partial<Record<Severity, number>>
  pd_not_taken: number
  fallback: number
  suppressed: number
  unbound: number
}

export type Page = { alerts: Incident[]; counts: Counts; more: boolean }

export type Entry = { id: number; at: string; kind: string; code: string; args?: Record<string, string>; author?: string }

export type Detail = { alert: Incident; timeline: Entry[]; grafana_url?: string; connectors: Record<string, string> }

export type Flag = '' | 'pd' | 'fallback' | 'suppressed'

export type Filters = { q: string; status: string; severity: string; method: string; team: string; service: string; flag: Flag }

export const NO_FILTERS: Filters = { q: '', status: 'active', severity: '', method: '', team: '', service: '', flag: '' }

export function queryOf(f: Filters) {
  const p = new URLSearchParams()
  if (f.status && f.status !== 'all') p.set('status', f.status)
  if (f.severity) p.set('severity', f.severity)
  if (f.method) p.set('method', f.method)
  if (f.team) p.set('team', f.team)
  if (f.service) p.set('service', f.service)
  if (f.q.trim()) p.set('q', f.q.trim())
  if (f.flag === 'pd') p.set('pd', 'failed')
  if (f.flag === 'fallback') p.set('fallback', 'true')
  if (f.flag === 'suppressed') p.set('suppressed', 'true')
  const s = p.toString()
  return s ? `?${s}` : ''
}

// The filters live in the address, so a link (PagerDuty, a message) opens the same view.
export function filtersFromURL(search: string): Filters {
  const p = new URLSearchParams(search)
  const f = { ...NO_FILTERS }
  for (const k of ['q', 'status', 'severity', 'method', 'team', 'service'] as const) {
    const v = p.get(k)
    if (v !== null) f[k] = v
  }
  const flag = p.get('flag')
  if (flag === 'pd' || flag === 'fallback' || flag === 'suppressed') f.flag = flag
  return f
}

export function urlOf(f: Filters, id: string | null) {
  const p = new URLSearchParams()
  for (const k of ['q', 'severity', 'method', 'team', 'service', 'flag'] as const) if (f[k]) p.set(k, f[k])
  if (f.status !== NO_FILTERS.status) p.set('status', f.status)
  if (id) p.set('id', id)
  const s = p.toString()
  return `/incidents${s ? `?${s}` : ''}`
}
