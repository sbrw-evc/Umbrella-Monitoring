import { Pause, Play, Plus, Search, Trash2 } from 'lucide-react'
import { Fragment, type ReactNode, useState } from 'react'
import { api, fmtTime, methodLabel, qs, type CI, type EventItem, type Maintenance, type ParseError, type Rule } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { maintStateLabel } from '../components/CiDrawer'
import { t } from '../i18n'
import { Empty, Field, Modal, PageHeader, SevBadge } from '../components/ui'

export function EventsPage() {
  const [q, setQ] = useState('')
  const [paused, setPaused] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  const { data, reload } = useFetch<{ items: EventItem[] }>(`/api/events${qs({ q, limit: 300 })}`)
  useLive(['event'], () => !paused && reload(), 700)
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('events.header.title')}
          sub={t('events.header.sub')}
          actions={
            <button className="btn" onClick={() => setPaused((p) => !p)}>
              {paused ? <Play size={14} /> : <Pause size={14} />} {paused ? t('events.header.resume') : t('events.header.pause')}
            </button>
          }
        />
        <div className="filterbar">
          <div className="search">
            <Search size={15} />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('events.filters.search')} />
          </div>
        </div>
        <div className="card card-flush">
          <table className="table table-compact">
            <thead>
              <tr>
                <th>{t('events.table.time')}</th>
                <th>{t('events.table.severity')}</th>
                <th>{t('events.table.status')}</th>
                <th>{t('events.table.event')}</th>
                <th>{t('events.table.ci')}</th>
                <th>{t('events.table.signal')}</th>
                <th>{t('events.table.source')}</th>
                <th>{t('events.table.incident')}</th>
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map((e) => (
                <Fragment key={e.id}>
                  <tr onClick={() => setOpen(open === e.id ? null : e.id)}>
                    <td className="nowrap">{fmtTime(e.received_at)}</td>
                    <td>
                      <SevBadge sev={e.severity} />
                    </td>
                    <td>{e.status === 'resolved' ? t('events.row.resolved') : t('events.row.active')}</td>
                    <td>
                      {e.title}
                      {e.suppressed && <span className="tag">{t('events.row.suppressed')}</span>}
                    </td>
                    <td className="nowrap">
                      {e.ci_name || '—'}
                      {!e.ci_id && <span className="tag tag-warn">{t('events.row.noCi')}</span>}
                    </td>
                    <td className="mono">
                      {e.signal} <span className="tag">{methodLabel(e.method)}</span>
                    </td>
                    <td>{e.source}</td>
                    <td className="mono">{e.alert_id ? <a href={`/incidents?id=${e.alert_id}&view=all`}>{e.alert_id}</a> : '—'}</td>
                  </tr>
                  {open === e.id && (
                    <tr className="row-detail">
                      <td colSpan={8}>
                        <pre className="json">{e.raw}</pre>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

export function ParseErrorsPage() {
  const { data, reload } = useFetch<{ items: ParseError[] }>('/api/parse-errors')
  useLive(['parse_error'], reload)
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('parseErrors.header.title')} sub={t('parseErrors.header.sub')} />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('parseErrors.empty.none')}</Empty>
          ) : (
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>{t('parseErrors.table.time')}</th>
                  <th>{t('parseErrors.table.connector')}</th>
                  <th>{t('parseErrors.table.block')}</th>
                  <th>{t('parseErrors.table.error')}</th>
                  <th>{t('parseErrors.table.raw')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((p) => (
                  <tr key={p.id}>
                    <td className="nowrap">{fmtTime(p.at)}</td>
                    <td>
                      <a href={`/connectors/${p.connector_id}`}>{p.connector}</a>
                    </td>
                    <td className="mono">{p.block}</td>
                    <td className="text-danger">{p.error}</td>
                    <td>
                      <div className="raw">{p.raw || '—'}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

export function RulesPage() {
  const { data } = useFetch<{ items: Rule[] }>('/api/rules')
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('rules.header.title')} sub={t('rules.header.sub')} />
        <div className="card card-flush">
          <table className="table">
            <thead>
              <tr>
                <th>{t('rules.table.method')}</th>
                <th>{t('rules.table.signal')}</th>
                <th>{t('rules.table.rule')}</th>
                <th>{t('rules.table.condition')}</th>
                <th>{t('rules.table.appliesTo')}</th>
                <th>{t('rules.table.severity')}</th>
                <th>{t('rules.table.enabled')}</th>
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map((r) => (
                <tr key={r.id}>
                  <td>
                    <span className={`tag tag-${r.method}`}>{methodLabel(r.method)}</span>
                  </td>
                  <td className="mono">{r.signal}</td>
                  <td>{r.name}</td>
                  <td className="mono">{r.condition}</td>
                  <td>{r.applies_to}</td>
                  <td>
                    <SevBadge sev={r.severity} />
                  </td>
                  <td>{r.enabled ? t('common.words.yes') : t('common.words.no')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function localInput(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

export function MaintenancePage() {
  const { toast } = useApp()
  const { data, reload } = useFetch<{ items: { maintenance: Maintenance; state: string }[] }>('/api/maintenance')
  const cis = useFetch<{ items: CI[] }>('/api/cis')
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(() => ({ title: '', ci_id: '', start: localInput(new Date()), end: localInput(new Date(Date.now() + 3600e3)) }))

  const submit = async () => {
    try {
      await api.post('/api/maintenance', { ...form, start: new Date(form.start).toISOString(), end: new Date(form.end).toISOString() })
      setCreating(false)
      reload()
      toast(t('maintenance.toasts.created'))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const remove = async (id: string) => {
    if (!window.confirm(t('maintenance.confirm.delete'))) return
    await api.del(`/api/maintenance/${id}`)
    reload()
  }
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('maintenance.header.title')}
          sub={t('maintenance.header.sub')}
          actions={
            <button className="btn btn-primary" onClick={() => setCreating(true)}>
              <Plus size={14} /> {t('maintenance.header.create')}
            </button>
          }
        />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('maintenance.empty.none')}</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('maintenance.table.id')}</th>
                  <th>{t('maintenance.table.window')}</th>
                  <th>{t('maintenance.table.ci')}</th>
                  <th>{t('maintenance.table.state')}</th>
                  <th>{t('maintenance.table.start')}</th>
                  <th>{t('maintenance.table.end')}</th>
                  <th>{t('maintenance.table.author')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map(({ maintenance: m, state }) => (
                  <tr key={m.id}>
                    <td className="mono nowrap">{m.id}</td>
                    <td>{m.title}</td>
                    <td>{m.ci_name}</td>
                    <td>
                      <span className={`pill ${state === 'active' ? 'pill-acknowledged' : state === 'planned' ? 'pill-open' : 'pill-muted'}`}>{maintStateLabel(state)}</span>
                    </td>
                    <td className="nowrap">{fmtTime(m.start)}</td>
                    <td className="nowrap">{fmtTime(m.end)}</td>
                    <td>{m.author}</td>
                    <td>
                      <button className="icon-btn" onClick={() => remove(m.id)}>
                        <Trash2 size={15} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {creating && (
        <Modal
          title={t('maintenance.modal.title')}
          onClose={() => setCreating(false)}
          footer={
            <>
              <button className="btn" onClick={() => setCreating(false)}>
                {t('common.actions.cancel')}
              </button>
              <button className="btn btn-primary" disabled={!form.title || !form.ci_id} onClick={submit}>
                {t('common.actions.create')}
              </button>
            </>
          }
        >
          <Field label={t('maintenance.modal.name')}>
            <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder={t('maintenance.modal.namePlaceholder')} />
          </Field>
          <Field label={t('maintenance.modal.ci')} help={t('maintenance.modal.ciHelp')}>
            <select value={form.ci_id} onChange={(e) => setForm({ ...form, ci_id: e.target.value })}>
              <option value="">{t('maintenance.modal.ciPlaceholder')}</option>
              {(cis.data?.items ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          <div className="row2">
            <Field label={t('maintenance.modal.start')}>
              <input type="datetime-local" value={form.start} onChange={(e) => setForm({ ...form, start: e.target.value })} />
            </Field>
            <Field label={t('maintenance.modal.end')}>
              <input type="datetime-local" value={form.end} onChange={(e) => setForm({ ...form, end: e.target.value })} />
            </Field>
          </div>
        </Modal>
      )}
    </div>
  )
}

interface SelfCheck {
  uptime_s: number
  events: number
  alerts: number
  active_alerts: number
  parse_errors: number
  connectors: number
  connectors_running: number
  last_event_at?: string
  store: string
  bus: string
  pagerduty: {
    mode: string
    breaker_open: boolean
    simulated_outage: boolean
    consecutive_failures: number
    sent: number
    failed: number
    queue: number
    last_success_at?: string
    last_error?: string
  }
}

export function SelfCheckPage() {
  const { toast } = useApp()
  const { data, reload } = useFetch<SelfCheck>('/api/selfcheck')
  useLive(['alert', 'event'], reload, 2000)
  if (!data) return <Empty>{t('common.words.loading')}</Empty>
  const pd = data.pagerduty
  const outage = async (on: boolean) => {
    await api.post('/api/selfcheck/pd-outage', { on })
    toast(on ? t('selfcheck.toasts.outageOn') : t('selfcheck.toasts.outageOff'))
    reload()
  }
  const tile = (title: string, value: ReactNode, sub?: string, cls = '') => (
    <div className={`stat ${cls}`}>
      <div className="stat-title">{title}</div>
      <div className="stat-value">{value}</div>
      {sub && <div className="stat-sub">{sub}</div>}
    </div>
  )
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('selfcheck.header.title')} sub={t('selfcheck.header.sub')} />
        <div className="stats">
          {tile(t('selfcheck.tiles.uptime'), t('selfcheck.tiles.uptimeValue', { m: Math.floor(data.uptime_s / 60) }), t('selfcheck.tiles.uptimeSub', { store: data.store, bus: data.bus }))}
          {tile(t('selfcheck.tiles.connectors'), `${data.connectors_running} / ${data.connectors}`, t('selfcheck.tiles.connectorsSub'))}
          {tile(t('selfcheck.tiles.events'), data.events, t('selfcheck.tiles.eventsSub', { time: fmtTime(data.last_event_at) }))}
          {tile(t('selfcheck.tiles.parseErrors'), data.parse_errors, undefined, data.parse_errors ? 'stat-warn' : '')}
          {tile(t('selfcheck.tiles.alerts'), `${data.active_alerts} / ${data.alerts}`, t('selfcheck.tiles.alertsSub'))}
        </div>
        <div className="card">
          <h3>PagerDuty Gateway</h3>
          <div className="stats">
            {tile(t('selfcheck.pd.mode'), pd.mode === 'live' ? 'Events API v2' : 'dry-run', pd.mode === 'live' ? t('selfcheck.pd.modeLive') : t('selfcheck.pd.modeDry'))}
            {tile(t('selfcheck.pd.breaker'), pd.breaker_open ? t('selfcheck.pd.breakerOpen') : t('selfcheck.pd.breakerClosed'), t('selfcheck.pd.breakerSub', { n: pd.consecutive_failures }), pd.breaker_open ? 'stat-bad' : '')}
            {tile(t('selfcheck.pd.sent'), pd.sent, t('selfcheck.pd.sentSub', { failed: pd.failed, queue: pd.queue }))}
            {tile(t('selfcheck.pd.lastSuccess'), fmtTime(pd.last_success_at), pd.last_error ? t('selfcheck.pd.lastError', { error: pd.last_error }) : undefined)}
          </div>
          <div className="outage">
            <div>
              <b>{t('selfcheck.outage.title')}</b> {t('selfcheck.outage.text')}
            </div>
            {pd.simulated_outage ? (
              <button className="btn btn-primary" onClick={() => outage(false)}>
                {t('selfcheck.outage.restore')}
              </button>
            ) : (
              <button className="btn btn-danger" onClick={() => outage(true)}>
                {t('selfcheck.outage.simulate')}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export function AuditPage() {
  const { data } = useFetch<{ items: { at: string; actor: string; action: string; object: string }[] }>('/api/audit')
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('audit.header.title')} sub={t('audit.header.sub')} />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('audit.empty.none')}</Empty>
          ) : (
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>{t('audit.table.time')}</th>
                  <th>{t('audit.table.actor')}</th>
                  <th>{t('audit.table.action')}</th>
                  <th>{t('audit.table.object')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((a, i) => (
                  <tr key={i}>
                    <td className="nowrap">{fmtTime(a.at)}</td>
                    <td>{a.actor}</td>
                    <td className="mono">{a.action}</td>
                    <td className="mono">{a.object}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

// Role keys under roles.list; each has a matching "<key>Perms" entry.
const ROLES = ['viewer', 'oncall', 'monitoring', 'owner', 'auditor', 'admin']

export function RolesPage() {
  const { meta } = useApp()
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('roles.header.title')} sub={t('roles.header.sub')} />
        <div className="card card-flush">
          <table className="table">
            <thead>
              <tr>
                <th>{t('roles.table.role')}</th>
                <th>{t('roles.table.perms')}</th>
              </tr>
            </thead>
            <tbody>
              {ROLES.map((r) => (
                <tr key={r}>
                  <td>{t(`roles.list.${r}`)}</td>
                  <td>{t(`roles.list.${r}Perms`)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="card">
          <h3>{t('roles.teams.title')}</h3>
          <div className="tags">
            {meta?.teams.map((team) => (
              <span key={team.id} className="tag">
                {team.name} · <code>{team.id}</code>
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
