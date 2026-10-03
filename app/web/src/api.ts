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
  pd_route?: string
  pd_incident_id?: string
  pd_incident_url?: string
  pd_retry?: string
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
  owners?: Owner[]
  origin: string
  source?: string
  external_url?: string
  created_at: string
  updated_at?: string
  status: Severity | ''
  own_status: Severity | ''
  open_alerts: number
  maintenance: boolean
  children: number
  parents: number
}

export interface Owner {
  name: string
  email?: string
  phone?: string
  role?: string
  from?: string
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

export interface RuleSeries {
  ci: string
  labels: Record<string, string>
  value: number
  since: string
  firing: boolean
  fired_at?: string
  last_seen: string
}

export interface Rule {
  id: string
  name: string
  description?: string
  method: Method
  signal: string
  source_id: string
  query: string
  ci_label: string
  service_label?: string
  op: string
  threshold: number
  for: string
  interval: string
  severity: Severity
  title: string
  team?: string
  enabled: boolean
  created_at?: string
  updated_at?: string
  updated_by?: string
  last_eval_at?: string
  last_error?: string
  series?: number
  pending?: number
  firing?: number
  state?: Record<string, RuleSeries>
}

export interface IntegrationField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'select'
  options?: string[]
  default?: string
  placeholder?: string
  help?: string
  required?: boolean
}

export interface IntegrationType {
  id: string
  title: string
  description: string
  mode: 'push' | 'pull' | 'metrics' | 'inventory'
  url_label: string
  url_placeholder: string
  url_required: boolean
  auth: string[]
  secret_label: string
  params: IntegrationField[] | null
  setup: boolean
  setup_help?: string
}

export interface Integration {
  id: string
  name: string
  type: string
  team: string
  url: string
  auth_type: string
  username?: string
  secret_ref?: string
  webhook_token_ref?: string
  tls_skip_verify: boolean
  params: Record<string, string>
  connector_id: string
  last_check_at?: string
  last_check_ok: boolean
  last_check?: string
  setup_at?: string
  setup_info?: string
  synced_at?: string
  sync_ok: boolean
  sync_info?: string
  created_at: string
  updated_at: string
  updated_by: string
  mode: IntegrationType['mode']
  type_title: string
  slug?: string
  connector_status?: string
  connector_version?: number
  connector_edited: boolean
  events_total: number
  errors_total: number
  last_event_at?: string
  ingest_url?: string
  token_header?: string
  snippet?: string
  setup_help?: string
  can_setup: boolean
  can_sync: boolean
  secret_set: boolean
}

export interface CheckResult {
  ok: boolean
  message: string
  at?: string
}

export interface PDRoute {
  id: string
  name: string
  team?: string
  service?: string
  routing_key_ref: string
  service_id?: string
  service_name?: string
}

export interface PDSettings {
  enabled: boolean
  region: 'us' | 'eu'
  events_url?: string
  api_url?: string
  routing_key_ref?: string
  service_id?: string
  service_name?: string
  api_token_ref?: string
  webhook_secret_ref?: string
  webhook_subscription_id?: string
  min_severity: Severity
  escalation_policies: string[]
  routes: PDRoute[]
  updated_at?: string
  updated_by?: string
}

export interface PDStatus {
  enabled: boolean
  configured: boolean
  region: string
  api_token: boolean
  webhook_secret: boolean
  routes: number
  breaker_open: boolean
  consecutive_failures: number
  sent: number
  failed: number
  queue: number
  last_success_at?: string
  last_error?: string
  last_error_at?: string
  last_webhook_at?: string
  oncall_synced_at?: string
  oncall_error?: string
}

export interface OnCallEntry {
  policy_id: string
  policy_name: string
  level: number
  user_name: string
  email?: string
  schedule?: string
  start?: string
  end?: string
}

export interface PDView {
  settings: PDSettings
  status: PDStatus
  oncall: { entries: OnCallEntry[]; synced_at?: string; error?: string }
  webhook_url: string
  events_url: string
  api_url: string
  openbao: boolean
}

export interface OpenBaoStatus {
  configured: boolean
  addr?: string
  mount?: string
  auth?: string
  reachable: boolean
  initialized: boolean
  sealed: boolean
  version?: string
  cluster_name?: string
  token_ok: boolean
  token_expires?: string
  policies?: string[]
  mount_ok: boolean
  error?: string
  last_error?: string
  last_error_at?: string
}

export interface Team {
  id: string
  name: string
  description?: string
  email?: string
  chat?: string
  members?: string[]
  leads?: string[]
  updated_at?: string
  updated_by?: string
}

export interface TeamView extends Team {
  managed: boolean
  cis: number
  open_alerts: number
  connectors: number
  rules: number
}

export interface StorageView {
  kind: 'file' | 'postgres'
  where: string
  enabled: boolean
  data_dir?: string
  postgres?: { host: string; port: number; database: string; user: string; sslmode: string }
  password_set?: boolean
}

export interface SetupStatus {
  required: boolean
  admin_exists: boolean
  token_required: boolean
  openbao: boolean
  themes: string[]
  locales: string[]
  defaults: { theme: string; locale: string }
  storage?: StorageView
  version: string
}

export interface Meta {
  teams: Team[]
  grafana: boolean
  pagerduty: boolean
  openbao: boolean
  version: string
}

export interface IncidentList {
  items: Incident[]
  total: number
  counts: Record<string, number>
  hourly: Record<string, number>[]
}

// ---- access ----

export type Perm =
  | 'incidents.view'
  | 'incidents.act'
  | 'cmdb.view'
  | 'cmdb.edit'
  | 'connectors.view'
  | 'connectors.edit'
  | 'events.view'
  | 'maintenance.edit'
  | 'rules.view'
  | 'rules.edit'
  | 'notify.edit'
  | 'integrations.edit'
  | 'selfcheck.view'
  | 'audit.view'
  | 'users.admin'

export interface User {
  id: string
  username: string
  name?: string
  email?: string
  roles: string[]
  business_services?: string[]
  disabled?: boolean
  service?: boolean
  must_change_password?: boolean
  created_at: string
  last_login_at?: string
  password_changed_at?: string
  locked_until?: string
  sessions?: number
  tokens?: number
}

export interface Role {
  id: string
  name: string
  description?: string
  permissions: Perm[]
  all_services: boolean
  built_in?: boolean
  updated_at?: string
}

export interface APIToken {
  id: string
  user_id: string
  name: string
  prefix: string
  created_at: string
  expires_at?: string
  last_used_at?: string
}

export interface Me {
  user: User
  permissions: Perm[]
  all_services: boolean
  business_services: { id: string; name: string }[]
  csrf: string
}

export type ChannelType = 'teams' | 'zoom'
export type NotifyEvent = 'open' | 'escalate' | 'ack' | 'resolve' | 'fallback'

export interface Channel {
  id: string
  name: string
  type: ChannelType
  mode: 'always' | 'fallback'
  enabled: boolean
  min_severity: Severity
  events: NotifyEvent[]
  services?: string[]
  url_ref?: string
  token_ref?: string
  url_hint?: string
  sent: number
  failed: number
  last_status?: number
  last_error?: string
  last_at?: string
  updated_at: string
  updated_by: string
}

export interface Delivery {
  id: string
  channel_id: string
  channel: string
  alert_id: string
  event: string
  ok: boolean
  status: number
  attempts: number
  error?: string
  at: string
}

// The CSRF token of the cookie session, sent on every changing request.
let csrf = ''
export function setCSRF(v: string) {
  csrf = v
}

export class ApiError extends Error {
  status: number
  code?: string
  constructor(message: string, status: number, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

// AUTH_EVENT fires when the server says the session is gone or the password
// must be changed; the app then shows the sign-in screen.
export const AUTH_EVENT = 'umb:auth'

async function request<T>(method: string, url: string, body?: unknown, extra?: Record<string, string>): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...extra }
  if (method !== 'GET' && csrf) headers['X-Umbrella-CSRF'] = csrf
  const res = await fetch(url, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const auth = res.status === 401 || data.code === 'password_change_required'
    if (auth && !url.startsWith('/api/auth/')) window.dispatchEvent(new CustomEvent(AUTH_EVENT, { detail: data.code ?? 'unauthorized' }))
    throw new ApiError(data.error || `HTTP ${res.status}`, res.status, data.code)
  }
  return data as T
}

export const api = {
  get: <T,>(url: string) => request<T>('GET', url),
  post: <T,>(url: string, body?: unknown) => request<T>('POST', url, body ?? {}),
  postWith: <T,>(url: string, body: unknown, headers: Record<string, string>) => request<T>('POST', url, body, headers),
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

// dateLocaleTime is a short axis label: "14:00", or "02.10 14:00" with the day.
export function dateLocaleTime(s: string, withDay: boolean): string {
  const d = new Date(s)
  const tm = d.toLocaleTimeString(dateLocale(), { hour: '2-digit', minute: '2-digit' })
  return withDay ? `${d.toLocaleDateString(dateLocale(), { day: '2-digit', month: '2-digit' })} ${tm}` : tm
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
