// Typed client for the Umbrella Core API.

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

export const SEV_LABEL: Record<Severity, string> = {
  critical: 'Critical',
  error: 'Error',
  warning: 'Warning',
  info: 'Info',
}

export const STATUS_LABEL: Record<AlertStatus, string> = {
  open: 'Открыта',
  acknowledged: 'Подтверждена',
  resolved: 'Решена',
}

export const PD_LABEL: Record<PDState, string> = {
  pending: 'В очереди',
  accepted: 'Принят',
  acked: 'Ack',
  failed: 'Не принят',
  skipped: 'Не отправлялся',
}

export const CI_TYPE_LABEL: Record<string, string> = {
  business_service: 'Бизнес-услуга',
  it_service: 'ИТ-сервис',
  host: 'Хост',
  database: 'База данных',
  cloud_group: 'Облачная группа',
  deployment: 'Deployment',
  network: 'Сетевое устройство',
}

export const METHOD_LABEL: Record<Method, string> = { red: 'RED', use: 'USE', other: '—' }

export function fmtTime(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  const t = d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return sameDay ? t : `${d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' })} ${t}`
}

export function fmtDuration(from: string, to?: string): string {
  const ms = (to ? new Date(to).getTime() : Date.now()) - new Date(from).getTime()
  const m = Math.max(0, Math.floor(ms / 60000))
  if (m < 1) return '< 1 мин'
  if (m < 60) return `${m} мин`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} ч ${m % 60} мин`
  return `${Math.floor(h / 24)} д ${h % 24} ч`
}
