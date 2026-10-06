import { useCallback, useEffect, useState } from 'react'
import { ExternalLink, Plus, RefreshCw, Search } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { mergeDicts } from '../../connections/connectionStrings'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Banner, Button, Field, formatDate, Input, Modal, Segmented, Select, Stepper, Switch } from '../../ui'
import { strings as ciStrings } from '../cis/strings'
import type { CI, CIList } from '../cis/types'
import { CreateHostsDialog, type HostKey } from '../bulk/BulkDialogs'
import { strings as bulkStrings } from '../bulk/strings'
import { useSession } from '../session'
import { strings } from './strings'
import '../services/services.css'
import '../connectors/connectors.css'
import '../cis/cis.css'
import '../rules/rules.css'
import './monitoring.css'
import '../bulk/bulk.css'

type Kind = 'zabbix' | 'prometheus'
type Sync = { started_at: string; finished_at: string; ok: boolean; error?: string; actor: string; hosts: number; version?: string }
type Source = {
  id: string
  name: string
  kind: Kind
  url: string
  credential_id?: string
  credential_name?: string
  skip_verify: boolean
  enabled: boolean
  sync_minutes: number
  query?: string
  host_label?: string
  sync: Sync
  hosts: number
  matched: number
  unmatched: number
  running: boolean
  next_sync_at?: string
}
type View = { sources: Source[]; defaults: { query: string; host_label: string } }
type Ref = { id: string; name: string }
type Host = {
  key: string
  host: string
  name: string
  // Lists may come as null from an older server (hosts read back from its snapshot).
  ips: string[] | null
  dns: string[] | null
  groups: string[] | null
  endpoints?: string[] | null
  state: string
  url?: string
  source_id: string
  source_name: string
  kind: Kind
  ci?: Ref
  match: string
  candidates: Ref[] | null
  also_in: { source_id: string; source_name: string; key: string; host: string }[] | null
}
type Summary = { total: number; matched: number; unmatched: number; ambiguous: number; excluded: number }
type HostList = { items: Host[]; summary: Summary; limited: boolean }
type Credential = { id: string; name: string; type: string }
type Report = { ok: boolean; error?: string; version?: string; hosts: number; sample: Host[] | null }

const MATCHES = ['matched', 'unmatched', 'ambiguous', 'excluded'] as const
const ran = (at?: string) => !!at && !at.startsWith('0001-')
const createStrings = mergeDicts(ciStrings, strings)
const hostStrings = mergeDicts(strings, bulkStrings)
const hostID = (h: { source_id: string; key: string }) => h.source_id + '/' + h.key
// creatable: a host an item can be made of.
const creatable = (h: Host) => !h.ci && h.match !== 'excluded'

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return v
}

export function MonitoringPage() {
  const t = useT(strings)
  const { can } = useSession()
  const [tab, setTab] = useState<'hosts' | 'sources'>('hosts')
  const [epoch, setEpoch] = useState(0)
  const view = useResource<View>('/api/monitoring', epoch)
  const [editing, setEditing] = useState<Source | 'new' | null>(null)
  const reload = useCallback(() => setEpoch((e) => e + 1), [])
  const v = view.data
  useEffect(() => {
    if (v && v.sources.length === 0) setTab('sources')
  }, [v])
  return (
    <div className="stack">
      <div className="row rl-head">
        <Segmented
          label={t('mon.tab.hosts')}
          value={tab}
          onChange={setTab}
          options={[
            { value: 'hosts', label: t('mon.tab.hosts') },
            { value: 'sources', label: `${t('mon.tab.sources')} (${v?.sources.length ?? 0})` },
          ]}
        />
        {can('monitoring:edit') && tab === 'sources' && (
          <Button variant="primary" onClick={() => setEditing('new')}>
            <Plus size={16} aria-hidden />
            {t('mon.new')}
          </Button>
        )}
      </div>
      <ErrorBanner error={view.error} strings={strings} />
      {v && tab === 'sources' && <SourcesTable v={v} onOpen={setEditing} onChanged={reload} />}
      {v && tab === 'hosts' && <HostsTab sources={v.sources} epoch={epoch} onChanged={reload} />}
      {v && <SourceEditor value={editing} defaults={v.defaults} onClose={() => setEditing(null)} onSaved={() => (setEditing(null), reload())} />}
    </div>
  )
}

function When({ at }: { at?: string }) {
  const { locale } = useLocale()
  const { timezone } = useSession()
  return <>{ran(at) ? formatDate(at, locale, timezone) : '—'}</>
}

function SourcesTable({ v, onOpen, onChanged }: { v: View; onOpen: (s: Source) => void; onChanged: () => void }) {
  const t = useT(strings)
  const { can } = useSession()
  const syncer = useAction()
  const [busy, setBusy] = useState('')
  const [done, setDone] = useState('')
  const read = (s: Source) => {
    setBusy(s.id)
    setDone('')
    void syncer
      .run(async () => {
        const st = await api<Sync>('POST', `/api/monitoring/sources/${s.id}/sync`)
        if (st.ok) setDone(t('mon.sync.done', { n: st.hosts, name: s.name }))
      })
      .finally(() => {
        setBusy('')
        onChanged()
      })
  }
  if (v.sources.length === 0) return <p className="muted card rl-empty">{t('mon.empty.sources')}</p>
  return (
    <>
      {done && <Banner kind="ok" title={done} />}
      <ErrorBanner error={syncer.error} strings={strings} />
      <div className="card cn-table-wrap">
        <table className="cn-table rl-table">
          <thead>
            <tr>
              <th>{t('mon.col.name')}</th>
              <th>{t('mon.col.url')}</th>
              <th>{t('mon.col.hosts')}</th>
              <th>{t('mon.col.sync')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {v.sources.map((s) => (
              <tr key={s.id}>
                <td className="rl-name">
                  {can('monitoring:edit') ? (
                    <button type="button" className="cn-link cn-name" onClick={() => onOpen(s)}>
                      {s.name}
                    </button>
                  ) : (
                    <span className="cn-name">{s.name}</span>
                  )}
                  <div className="rl-sub">
                    <span className={`pill mon-kind mon-kind-${s.kind}`}>{t(`mon.kind.${s.kind}`)}</span>
                    {!s.enabled && <span className="pill pill-off">{t('mon.off')}</span>}
                    {s.sync.version && <span className="muted"> {s.sync.version}</span>}
                  </div>
                </td>
                <td className="mon-url">
                  <code>{s.url}</code>
                  {s.credential_name && <div className="muted rl-sub">{s.credential_name}</div>}
                </td>
                <td>
                  <span className="cn-name">{s.hosts}</span>
                  <div className="rl-sub">
                    <span className="muted">{t('mon.col.matched')}: {s.matched}</span>
                    <br />
                    <span className={s.unmatched > 0 ? 'mon-warn' : 'muted'}>
                      {t('mon.col.unmatched')}: {s.unmatched}
                    </span>
                  </div>
                </td>
                <td>
                  {ran(s.sync.started_at) ? (
                    <>
                      <When at={s.sync.finished_at || s.sync.started_at} />
                      {!s.sync.ok && (
                        <div className="mon-error" title={s.sync.error}>
                          <span className="pill pill-error">{t('mon.sync.failed')}</span> {s.sync.error}
                        </div>
                      )}
                    </>
                  ) : (
                    <span className="muted">{t('mon.sync.never')}</span>
                  )}
                  <div className="muted rl-sub">{s.sync_minutes > 0 ? t('mon.sync.every', { n: s.sync_minutes }) : t('mon.sync.manual')}</div>
                </td>
                <td className="num mon-sync">
                  {can('monitoring:sync') && (
                    <Button busy={busy === s.id || s.running} onClick={() => read(s)}>
                      <RefreshCw size={15} aria-hidden />
                      {busy === s.id || s.running ? t('mon.sync.running') : t('mon.sync.now')}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

function StatePill({ state }: { state: string }) {
  const t = useT(strings)
  const tone = state === 'up' ? 'ok' : state === 'down' ? 'error' : state === 'partial' ? 'warn' : 'off'
  return <span className={`pill pill-${tone}`}>{t(`mon.state.${state}`)}</span>
}

function HostsTab({ sources, epoch, onChanged }: { sources: Source[]; epoch: number; onChanged: () => void }) {
  const t = useT(hostStrings)
  const { can } = useSession()
  const [match, setMatch] = useState('')
  const [source, setSource] = useState('')
  const [q, setQ] = useState('')
  const dq = useDebounced(q, 250)
  const [own, setOwn] = useState(0)
  const params = new URLSearchParams()
  if (match) params.set('match', match)
  if (source) params.set('source', source)
  if (dq.trim()) params.set('q', dq.trim())
  const list = useResource<HostList>(`/api/monitoring/hosts?${params}`, epoch + own)
  const [linking, setLinking] = useState<Host | null>(null)
  const [creating, setCreating] = useState<Host | null>(null)
  const [checked, setChecked] = useState<Map<string, HostKey>>(new Map())
  const [bulkHosts, setBulkHosts] = useState<HostKey[] | null>(null)
  const changed = () => {
    setOwn((n) => n + 1)
    onChanged()
  }
  const canLink = can('monitoring:link')
  const canCreate = canLink && can('cis:edit')
  if (!list.data) return list.error ? <ErrorBanner error={list.error} strings={strings} /> : <p className="muted">{t('loading')}</p>
  const { items, summary } = list.data
  const filtered = match !== '' || source !== '' || q.trim() !== ''
  const eligible = canCreate ? items.filter(creatable) : []
  const allChecked = eligible.length > 0 && eligible.every((h) => checked.has(hostID(h)))
  const toggleAll = () =>
    setChecked((c) => {
      const n = new Map(c)
      for (const h of eligible) {
        if (allChecked) n.delete(hostID(h))
        else n.set(hostID(h), { source_id: h.source_id, key: h.key })
      }
      return n
    })
  const toggle = (h: Host) =>
    setChecked((c) => {
      const n = new Map(c)
      if (n.has(hostID(h))) n.delete(hostID(h))
      else n.set(hostID(h), { source_id: h.source_id, key: h.key })
      return n
    })
  return (
    <div className="ci-page">
      <div className="ci-tiles">
        <button type="button" className={`card ci-tile ci-tile-flag ${match === '' ? 'active' : ''}`} aria-pressed={match === ''} onClick={() => setMatch('')}>
          <span className="ci-tile-value">{summary.total}</span>
          <span className="muted">{t('mon.tile.total')}</span>
        </button>
        {MATCHES.map((m) => (
          <button
            key={m}
            type="button"
            className={`card ci-tile ci-tile-flag ${match === m ? 'active' : ''} ${m !== 'matched' && m !== 'excluded' && summary[m] > 0 ? 'warn' : ''}`}
            aria-pressed={match === m}
            onClick={() => setMatch(match === m ? '' : m)}
          >
            <span className="ci-tile-value">{summary[m]}</span>
            <span className="muted">{t(`mon.tile.${m}`)}</span>
          </button>
        ))}
      </div>
      <div className="card svc-toolbar">
        <div className="svc-toolbar-row">
          <label className="svc-search">
            <Search size={16} aria-hidden />
            <Input type="search" value={q} placeholder={t('mon.search')} aria-label={t('mon.search')} onChange={(e) => setQ(e.target.value)} />
          </label>
          {sources.length > 1 && (
            <label className="mon-source-filter">
              <span className="muted">{t('mon.filter.source')}</span>
              <Select value={source} onChange={(e) => setSource(e.target.value)}>
                <option value="">{t('mon.filter.any')}</option>
                {sources.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </Select>
            </label>
          )}
          <span className="muted svc-count">{t('mon.count', { shown: items.length, total: summary.total })}</span>
        </div>
        {canCreate && checked.size > 0 && (
          <div className="bulk-bar">
            <span className="muted">{t('bulk.selected', { n: checked.size })}</span>
            <Button variant="primary" onClick={() => setBulkHosts([...checked.values()])}>
              <Plus size={16} />
              {t('bulk.hosts.create')}
            </Button>
            <Button variant="ghost" onClick={() => setChecked(new Map())}>
              {t('bulk.clear')}
            </Button>
          </div>
        )}
      </div>
      {list.data.limited && <Banner kind="info" title={t('mon.limited', { n: items.length })} />}
      {items.length === 0 ? (
        <div className="card svc-empty">
          <p>{t(filtered || summary.total > 0 ? 'mon.empty.filtered' : 'mon.empty.hosts')}</p>
        </div>
      ) : (
        <div className="card cn-table-wrap">
          <table className="cn-table mon-table">
            <thead>
              <tr>
                {eligible.length > 0 && (
                  <th className="bulk-check">
                    <input type="checkbox" aria-label={t('bulk.selectAll')} checked={allChecked} onChange={toggleAll} />
                  </th>
                )}
                <th>{t('mon.h.host')}</th>
                <th>{t('mon.h.system')}</th>
                <th>{t('mon.h.addresses')}</th>
                <th>{t('mon.h.groups')}</th>
                <th>{t('mon.h.state')}</th>
                <th>{t('mon.h.ci')}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((h) => (
                <tr key={hostID(h)}>
                  {eligible.length > 0 && (
                    <td className="bulk-check">
                      {creatable(h) && (
                        <input type="checkbox" aria-label={t('bulk.selectOne', { name: h.name || h.host })} checked={checked.has(hostID(h))} onChange={() => toggle(h)} />
                      )}
                    </td>
                  )}
                  <td>
                    <div className="cn-name">
                      {h.url ? (
                        <a href={h.url} target="_blank" rel="noopener noreferrer" title={t('mon.open')}>
                          {h.name || h.host}
                          <ExternalLink size={12} aria-hidden />
                        </a>
                      ) : (
                        h.name || h.host
                      )}
                    </div>
                    {h.name && h.host && h.name !== h.host && <div className="muted cn-mono">{h.host}</div>}
                  </td>
                  <td>{h.source_name}</td>
                  <td className="cn-mono">{[...(h.ips ?? []), ...(h.dns ?? [])].join(', ') || <span className="muted">—</span>}</td>
                  <td>{h.groups?.length ? h.groups.join(', ') : <span className="muted">—</span>}</td>
                  <td>
                    <StatePill state={h.state} />
                  </td>
                  <td className="mon-ci">
                    <HostCI h={h} />
                    {(canLink || canCreate) && (
                      <div className="row mon-actions">
                        {canLink && (
                          <Button variant="ghost" onClick={() => setLinking(h)}>
                            {t(h.ci || h.match === 'excluded' ? 'mon.act.change' : 'mon.act.link')}
                          </Button>
                        )}
                        {canCreate && !h.ci && (
                          <Button variant="ghost" onClick={() => setCreating(h)}>
                            {t('mon.act.create')}
                          </Button>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <LinkDialog host={linking} onClose={() => setLinking(null)} onSaved={() => (setLinking(null), changed())} />
      <CreateDialog host={creating} onClose={() => setCreating(null)} onSaved={() => (setCreating(null), changed())} />
      <CreateHostsDialog
        hosts={bulkHosts}
        onClose={() => setBulkHosts(null)}
        onDone={() => {
          setChecked(new Map())
          changed()
        }}
      />
    </div>
  )
}

function HostCI({ h }: { h: Host }) {
  const t = useT(strings)
  if (h.ci)
    return (
      <div>
        <span className="cn-name">{h.ci.name}</span> <span className="muted">· {t(`mon.match.${h.match}`)}</span>
      </div>
    )
  if (h.match === 'excluded') return <span className="pill pill-off">{t('mon.match.excluded')}</span>
  if (h.match === 'ambiguous')
    return (
      <div>
        <span className="pill pill-warn">{t('mon.match.ambiguous')}</span>
        <div className="muted rl-sub">{(h.candidates ?? []).map((c) => c.name).join(', ')}</div>
      </div>
    )
  return (
    <div>
      <span className="pill pill-warn">{t('mon.match.none')}</span>
      {(h.also_in?.length ?? 0) > 0 && <div className="muted rl-sub">{t('mon.also', { list: (h.also_in ?? []).map((o) => `${o.source_name}: ${o.host}`).join(', ') })}</div>}
    </div>
  )
}

function LinkDialog({ host, onClose, onSaved }: { host: Host | null; onClose: () => void; onSaved: () => void }) {
  const t = useT(strings)
  const [q, setQ] = useState('')
  const [pick, setPick] = useState('')
  const saver = useAction()
  const dq = useDebounced(q, 250)
  const found = useResource<CIList>(host && dq.trim() ? `/api/cis?q=${encodeURIComponent(dq.trim())}` : '', 0)
  useEffect(() => {
    setQ(host ? host.host || host.name : '')
    setPick(host?.ci?.id ?? '')
    saver.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host])
  const send = (mode: 'ci' | 'none' | 'auto') =>
    host &&
    void saver.run(async () => {
      await api('POST', '/api/monitoring/link', { source_id: host.source_id, key: host.key, mode, ci_id: mode === 'ci' ? pick : '' })
      onSaved()
    })
  const results: Ref[] = (found.data?.items ?? []).slice(0, 20).map((c: CI) => ({ id: c.id, name: c.name }))
  const option = (r: Ref, extra?: string) => (
    <label key={r.id} className={`mon-option ${pick === r.id ? 'active' : ''}`}>
      <input type="radio" name="mon-ci" checked={pick === r.id} onChange={() => setPick(r.id)} />
      <span className="cn-name">{r.name}</span>
      <span className="muted cn-mono">{r.id}</span>
      {extra && <span className="muted">{extra}</span>}
    </label>
  )
  return (
    <Modal
      open={host !== null}
      title={host ? t('mon.link.title', { host: host.name || host.host }) : ''}
      onClose={onClose}
      footer={
        <>
          {host && (host.match === 'manual' || host.match === 'excluded') && (
            <Button variant="ghost" busy={saver.busy} onClick={() => send('auto')}>
              {t('mon.link.auto')}
            </Button>
          )}
          {host && host.match !== 'excluded' && (
            <Button busy={saver.busy} onClick={() => send('none')}>
              {t('mon.link.none')}
            </Button>
          )}
          <Button onClick={onClose}>{t('mon.cancel')}</Button>
          <Button variant="primary" busy={saver.busy} disabled={!pick || pick === host?.ci?.id} onClick={() => send('ci')}>
            {t('mon.link.save')}
          </Button>
        </>
      }
    >
      {host && (
        <div className="stack">
          <p className="muted">{t('mon.link.text')}</p>
          <p className="hint">{t('mon.link.none.hint')}</p>
          {(host.candidates?.length ?? 0) > 0 && (
            <div className="mon-options">
              <span className="nb-group-title">{t('mon.link.candidates')}</span>
              {(host.candidates ?? []).map((c) => option(c))}
            </div>
          )}
          <label className="svc-search mon-link-search">
            <Search size={16} aria-hidden />
            <Input type="search" value={q} placeholder={t('mon.link.search')} aria-label={t('mon.link.search')} onChange={(e) => setQ(e.target.value)} />
          </label>
          {dq.trim() !== '' && (
            <div className="mon-options">
              <span className="nb-group-title">{t('mon.link.results')}</span>
              {found.data && results.length === 0 && <p className="muted">{t('mon.link.nothing')}</p>}
              {results.map((r) => option(r))}
            </div>
          )}
          <ErrorBanner error={saver.error ?? found.error} strings={strings} />
        </div>
      )}
    </Modal>
  )
}

function CreateDialog({ host, onClose, onSaved }: { host: Host | null; onClose: () => void; onSaved: () => void }) {
  const t = useT(createStrings)
  const [kind, setKind] = useState('device')
  const [register, setRegister] = useState(false)
  const saver = useAction()
  useEffect(() => {
    setKind('device')
    setRegister(false)
    saver.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host])
  const registrable = kind === 'device' || kind === 'vm'
  const submit = () =>
    host &&
    void saver.run(async () => {
      await api('POST', '/api/monitoring/ci', { source_id: host.source_id, key: host.key, kind, register: register && registrable })
      onSaved()
    })
  return (
    <Modal
      open={host !== null}
      title={host ? t('mon.create.title', { host: host.name || host.host }) : ''}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>{t('mon.cancel')}</Button>
          <Button variant="primary" busy={saver.busy} onClick={submit}>
            {t('mon.create.save')}
          </Button>
        </>
      }
    >
      {host && (
        <div className="stack">
          <p className="muted">{t('mon.create.text', { name: host.host || host.name })}</p>
          {(host.also_in?.length ?? 0) > 0 && <p className="hint">{t('mon.create.also', { list: (host.also_in ?? []).map((o) => `${o.source_name}: ${o.host}`).join(', ') })}</p>}
          <Field label={t('mon.create.kind')}>
            {(id) => (
              <Select id={id} value={kind} onChange={(e) => setKind(e.target.value)}>
                {['device', 'vm', 'service', 'other'].map((k) => (
                  <option key={k} value={k}>
                    {t(`mon.kind.${k}`)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {registrable && <Switch checked={register} onChange={setRegister} label={t('mon.create.register')} />}
          <ErrorBanner error={saver.error} strings={createStrings} />
        </div>
      )}
    </Modal>
  )
}

type Draft = Omit<Source, 'id' | 'sync' | 'hosts' | 'matched' | 'unmatched' | 'running' | 'next_sync_at' | 'credential_name'> & {
  credential_id: string
  query: string
  host_label: string
}

function blank(kind: Kind = 'zabbix'): Draft {
  return { name: '', kind, url: '', credential_id: '', skip_verify: false, enabled: true, sync_minutes: 60, query: '', host_label: '' }
}

function SourceEditor({
  value,
  defaults,
  onClose,
  onSaved,
}: {
  value: Source | 'new' | null
  defaults: View['defaults']
  onClose: () => void
  onSaved: () => void
}) {
  const t = useT(strings)
  const { can } = useSession()
  const editing = value && value !== 'new' ? value : null
  const [d, setD] = useState<Draft>(blank())
  const [report, setReport] = useState<Report | null>(null)
  const creds = useResource<Credential[]>(value && can('credentials:view') ? '/api/credentials' : '', 0)
  const save = useAction()
  const test = useAction()
  useEffect(() => {
    setD(
      editing
        ? {
            name: editing.name,
            kind: editing.kind,
            url: editing.url,
            credential_id: editing.credential_id ?? '',
            skip_verify: editing.skip_verify,
            enabled: editing.enabled,
            sync_minutes: editing.sync_minutes,
            query: editing.query ?? '',
            host_label: editing.host_label ?? '',
          }
        : blank(),
    )
    setReport(null)
    save.clear()
    test.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])
  const set = (patch: Partial<Draft>) => {
    setD({ ...d, ...patch })
    setReport(null)
  }
  const allowed = d.kind === 'zabbix' ? ['bearer', 'basic'] : ['bearer', 'basic', 'header']
  const usable = (creds.data ?? []).filter((c) => allowed.includes(c.type))
  const submit = () =>
    save.run(async () => {
      const saved = await api<Source>(editing ? 'PUT' : 'POST', editing ? `/api/monitoring/sources/${editing.id}` : '/api/monitoring/sources', d)
      // A new or changed system shows its hosts right away.
      if (saved.enabled && can('monitoring:sync')) await api('POST', `/api/monitoring/sources/${saved.id}/sync`).catch(() => undefined)
      onSaved()
    })
  const check = () =>
    test.run(async () => {
      setReport(await api<Report>('POST', '/api/monitoring/test', d))
    })
  const remove = () =>
    editing &&
    window.confirm(t('mon.delete.confirm', { name: editing.name })) &&
    save.run(async () => {
      await api('DELETE', `/api/monitoring/sources/${editing.id}`)
      onSaved()
    })
  return (
    <Modal
      open={value !== null}
      title={editing ? t('mon.dialog.edit') : t('mon.dialog.new')}
      onClose={onClose}
      footer={
        <>
          {editing && (
            <Button variant="ghost" busy={save.busy} onClick={() => void remove()}>
              {t('mon.delete')}
            </Button>
          )}
          {can('monitoring:test') && (
            <Button busy={test.busy} onClick={() => void check()}>
              {t('mon.test')}
            </Button>
          )}
          <Button onClick={onClose}>{t('mon.cancel')}</Button>
          <Button variant="primary" busy={save.busy} onClick={() => void submit()}>
            {t('mon.save')}
          </Button>
        </>
      }
    >
      <div className="stack">
        <Segmented
          label={t('mon.kind')}
          value={d.kind}
          onChange={(kind) => set({ kind, credential_id: '', name: d.name || '' })}
          options={[
            { value: 'zabbix', label: t('mon.kind.zabbix') },
            { value: 'prometheus', label: t('mon.kind.prometheus') },
          ]}
        />
        <Field label={t('mon.name')}>
          {(id) => <Input id={id} value={d.name} placeholder={t(`mon.kind.${d.kind}`)} onChange={(e) => set({ name: e.target.value })} />}
        </Field>
        <Field label={t('mon.url')} hint={t(`mon.url.${d.kind}.hint`)}>
          {(id) => (
            <Input
              id={id}
              value={d.url}
              spellCheck={false}
              autoComplete="off"
              placeholder={d.kind === 'zabbix' ? 'https://zabbix.example.com' : 'http://prometheus:9090'}
              onChange={(e) => set({ url: e.target.value })}
            />
          )}
        </Field>
        <Field label={t('mon.cred')} hint={usable.length === 0 && creds.data ? t('mon.cred.missing') : t(`mon.cred.${d.kind}.hint`)}>
          {(id) => (
            <Select id={id} value={d.credential_id} onChange={(e) => set({ credential_id: e.target.value })}>
              <option value="">{t('mon.cred.none')}</option>
              {editing?.credential_id && !usable.some((c) => c.id === editing.credential_id) && (
                <option value={editing.credential_id}>{editing.credential_name ?? editing.credential_id}</option>
              )}
              {usable.map((c) => (
                <option key={c.id} value={c.id}>
                  {`${c.name} (${c.type})`}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {d.kind === 'prometheus' && (
          <div className="rl-grid">
            <Field label={t('mon.query')} hint={t('mon.query.hint')}>
              {(id) => <Input id={id} value={d.query} spellCheck={false} placeholder={defaults.query} onChange={(e) => set({ query: e.target.value })} />}
            </Field>
            <Field label={t('mon.label')} hint={t('mon.label.hint')}>
              {(id) => <Input id={id} value={d.host_label} spellCheck={false} placeholder={defaults.host_label} onChange={(e) => set({ host_label: e.target.value })} />}
            </Field>
          </div>
        )}
        {d.url.trim().startsWith('https') && <Switch checked={d.skip_verify} onChange={(skip_verify) => set({ skip_verify })} label={t('mon.skip')} />}
        <Switch checked={d.enabled} onChange={(enabled) => set({ enabled })} label={t('mon.enabled')} hint={t('mon.enabled.hint')} />
        <Field label={t('mon.interval')} hint={t('mon.interval.hint')}>
          {(id) => (
            <div className="nb-stepper">
              <Stepper id={id} value={d.sync_minutes} min={0} max={10080} suffix={t('mon.minutes')} onChange={(sync_minutes) => set({ sync_minutes })} />
            </div>
          )}
        </Field>
        {report &&
          (report.ok ? (
            <Banner kind="ok" title={t('mon.test.ok', { hosts: report.hosts })}>
              {report.version && <div>{t('mon.test.version', { version: report.version })}</div>}
              {(report.sample?.length ?? 0) > 0 && <div className="cn-mono">{(report.sample ?? []).map((h) => h.host || h.name).join(', ')}</div>}
            </Banner>
          ) : (
            <Banner kind="error" title={t('mon.test.fail')}>
              {report.error}
            </Banner>
          ))}
        <ErrorBanner error={test.error ?? save.error} strings={strings} />
      </div>
    </Modal>
  )
}
