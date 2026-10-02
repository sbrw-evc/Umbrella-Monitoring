// Typed client for the Umbrella Core API.

import { dateLocale, t } from './i18n'

export type Severity = 'critical' | 'error' | 'warning' | 'info'
export const SEVERITIES: Severity[] = ['critical', 'error', 'warning', 'info']
export type AlertStatus = 'open' | 'acknowledged' | 'resolved'
export type PDState = 'pending' | 'accepted' | 'acked' | 'failed' | 'skipped'
export type Method = 'red' | 'use' | 'other'

export interface TimelineEntry {
  at: string
  kind: string
  text: string
  author?: string
}

export interface Incident {
  id: string
  dedup_key: string
  title: string
  ci_id?: string
  ci_name: string
  ci_type?: string
  service?: string
  team?: string
  signal: string
  method: Method
  severity: Severity
  status: AlertStatus
  sources: Record<string, string>
  count: number
  first_seen: string
  last_seen: string
  resolved_at?: string
  acked_by?: string
  suppressed: boolean
  pd_state: PDState
  pd_dedup_key: string
  pd_error?: string
  fallback: boolean
  related_id?: string
  timeline?: TimelineEntry[]
}

export interface EventItem {
  id: string
  connector_id: string
  source: string
  external_id: string
  ci_name: string
  ci_id?: string
  signal: string
  method: Method
  severity: Severity
  status: 'firing' | 'resolved'
  title: string
  value?: string
  labels?: Record<string, string>
  raw: string
  received_at: string
  alert_id?: string
  suppressed?: boolean
}

export interface Identity {
  kind: string
  value: string
  since: string
  until?: string
}

export interface CI {
  id: string
  name: string
  type: string
  team: string
  description?: string
  logical_group?: string
  labels?: Record<string, string>
  identities: Identity[]
  origin: string
  created_at: string
  status: Severity | ''
  own_status: Severity | ''
  open_alerts: number
  maintenance: boolean
  children: number
  parents: number
}

export interface Relation {
  from: string
  to: string
  type: string
}

export interface FlowNode {
  id: string
  kind: string
  x: number
  y: number
  config: Record<string, string>
  disabled?: boolean
}

export interface FlowEdge {
  id: string
  source: string
  target: string
}

export interface Graph {
  nodes: FlowNode[]
  edges: FlowEdge[]
}

export interface Connector {
  id: string
  name: string
  description?: string
  team: string
  status: 'running' | 'stopped'
  version: number
  draft: Graph
  published?: Graph
  draft_dirty: boolean
  sample_input?: string
  updated_at: string
  updated_by: string
  events_total: number
  errors_total: number
  last_event_at?: string
}

export interface FieldSpec {
  key: string
  label: string
  type: 'text' | 'textarea' | 'select' | 'secret'
  options?: string[]
  default?: string
  placeholder?: string
  help?: string
}

export interface BlockSpec {
  kind: string
  category: string
  title: string
  description: string
  fields: FieldSpec[]
}

export interface StepTrace {
  node_id: string
  kind: string
  in: number
  out: number
  sample?: unknown
  error?: string
  ms: number
}

export interface DraftEvent {
  title: string
  ci: string
  signal: string
  method: Method
  severity: Severity
  status: string
  external_id: string
  value?: string
  labels?: Record<string, string>
}

export interface DryRunResult {
  events: DraftEvent[]
  trace: StepTrace[]
  errors: { node_id: string; kind: string; error: string }[]
  error?: string
  cis: { input: string; ci_id?: string; ci_name?: string; found: boolean }[]
}

export interface ParseError {
  id: string
  connector_id: string
  connector: string
  block: string
  error: string
  raw: string
  at: string
}

export interface Maintenance {
  id: string
  title: string
  ci_id: string
  ci_name: string
  start: string
  end: string
  author: string
  created_at: string
}

export interface Rule {
  id: string
  method: Method
  signal: string
  name: string
  condition: string
  applies_to: string
  severity: Severity
  enabled: boolean
}

export interface Team {
  id: string
  name: string
}

export interface Meta {
  teams: Team[]
  users: string[]
  grafana: boolean
  pd_mode: string
  version: string
}

export interface IncidentList {
  items: Incident[]
  total: number
  counts: Record<string, number>
  hourly: Record<string, number>[]
}

let currentUser = 'Дежурный инженер'
export function setUser(u: string) {
  currentUser = u
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-Umbrella-User': encodeURIComponent(currentUser),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data as T
}

export const api = {
  get: <T,>(url: string) => request<T>('GET', url),
  post: <T,>(url: string, body?: unknown) => request<T>('POST', url, body ?? {}),
  put: <T,>(url: string, body: unknown) => request<T>('PUT', url, body),
  del: (url: string) => request<void>('DELETE', url),
}

export function qs(params: Record<string, string | number | undefined | null>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') p.set(k, String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

// ---- formatting ----
// Labels and time formats follow the active language (see i18n.ts).

export const sevLabel = (s: Severity | '' | 'ok') => t(`common.severity.${s || 'ok'}`)
export const statusLabel = (s: AlertStatus) => t(`common.status.${s}`)
export const pdLabel = (s: PDState) => t(`common.pd.${s}`)
export const methodLabel = (m: Method) => t(`common.method.${m}`)
export function ciTypeLabel(type?: string): string {
  if (!type) return ''
  const v = t(`common.ciType.${type}`)
  return v === `common.ciType.${type}` ? type : v
}

export function fmtTime(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  const now = new Date()
  const loc = dateLocale()
  const sameDay = d.toDateString() === now.toDateString()
  const tm = d.toLocaleTimeString(loc, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return sameDay ? tm : `${d.toLocaleDateString(loc, { day: '2-digit', month: '2-digit' })} ${tm}`
}

export function fmtDateTime(s?: string): string {
  if (!s) return '—'
  return new Date(s).toLocaleString(dateLocale(), { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export function fmtDuration(from: string, to?: string): string {
  const ms = (to ? new Date(to).getTime() : Date.now()) - new Date(from).getTime()
  const m = Math.max(0, Math.floor(ms / 60000))
  if (m < 1) return t('common.time.lessMinute')
  if (m < 60) return t('common.time.min', { m })
  const h = Math.floor(m / 60)
  if (h < 24) return t('common.time.hourMin', { h, m: m % 60 })
  return t('common.time.dayHour', { d: Math.floor(h / 24), h: h % 24 })
}
