import { BellOff, Bookmark, CheckCheck, CircleCheck, Download, RefreshCw, Search, Trash2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, qs, sevLabel, SEVERITIES, type IncidentList, type Severity } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { IncidentTable, type SortKey } from '../components/IncidentTable'
import { PageHeader, SideList } from '../components/ui'
import { t } from '../i18n'

// System maps: saved filters that cannot be edited, like "open" / "closed".
// Titles are translated at render time: incidents.views.<id>.
const SYSTEM_VIEWS: { id: string; params: Record<string, string> }[] = [
  { id: 'open', params: { view: 'open' } },
  { id: 'closed', params: { view: 'closed' } },
  { id: 'all', params: { view: 'all', hours: '24' } },
  { id: 'pd', params: { view: 'open', pd: 'pending,failed' } },
  { id: 'fallback', params: { view: 'all', fallback: 'yes' } },
  { id: 'red', params: { view: 'open', method: 'red' } },
  { id: 'use', params: { view: 'open', method: 'use' } },
]

const FILTER_KEYS = ['view', 'q', 'severity', 'method', 'pd', 'fallback', 'hours', 'sort', 'order', 'service']

interface SavedView {
  id: string
  title: string
  params: Record<string, string>
}

function loadViews(): SavedView[] {
  try {
    return JSON.parse(localStorage.getItem('umb.views') ?? '[]')
  } catch {
    return []
  }
}

export function IncidentsPage() {
  const { team, toast } = useApp()
  const [params, setParams] = useSearchParams()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [views, setViews] = useState<SavedView[]>(loadViews)
  const [q, setQ] = useState(params.get('q') ?? '')

  const filters = useMemo(() => {
    const f: Record<string, string> = {}
    for (const k of FILTER_KEYS) {
      const v = params.get(k)
      if (v) f[k] = v
    }
    if (!f.view) f.view = 'open'
    return f
  }, [params])

  const url = `/api/incidents${qs({ ...filters, team })}`
  const { data, reload, loading } = useFetch<IncidentList>(url)
  useLive(['alert'], reload, 800)

  const activeView =
    [...SYSTEM_VIEWS, ...views].find((v) => {
      const keys = new Set([...Object.keys(v.params), ...Object.keys(filters).filter((k) => k !== 'sort' && k !== 'order' && k !== 'q')])
      return [...keys].every((k) => (v.params[k] ?? (k === 'view' ? 'open' : '')) === (filters[k] ?? ''))
    })?.id ?? ''

  const update = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === '') next.delete(k)
      else next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  const applyView = (id: string) => {
    const v = [...SYSTEM_VIEWS, ...views].find((x) => x.id === id)
    if (!v) return
    const next = new URLSearchParams(v.params)
    setParams(next)
    setQ(v.params.q ?? '')
    setSelected(new Set())
  }

  const toggleSev = (s: Severity) => {
    const cur = new Set((filters.severity ?? '').split(',').filter(Boolean))
    if (cur.has(s)) cur.delete(s)
    else cur.add(s)
    update({ severity: [...cur].join(',') })
  }

  const onSort = (k: SortKey) => {
    if (filters.sort === k) update({ order: filters.order === 'asc' ? null : 'asc' })
    else update({ sort: k, order: null })
  }

  const saveView = () => {
    const title = window.prompt(t('incidents.views.prompt'))
    if (!title) return
    const p = { ...filters }
    delete p.sort
    delete p.order
    const next = [...views, { id: `u${Date.now()}`, title, params: p }]
    setViews(next)
    try {
      localStorage.setItem('umb.views', JSON.stringify(next))
    } catch {
      /* ignore */
    }
    toast(t('incidents.toasts.viewSaved'))
  }

  const removeView = (id: string) => {
    const next = views.filter((v) => v.id !== id)
    setViews(next)
    try {
      localStorage.setItem('umb.views', JSON.stringify(next))
    } catch {
      /* ignore */
    }
  }

  const bulk = async (action: 'ack' | 'resolve') => {
    try {
      const r = await api.post<{ done: number; failed: Record<string, string> }>('/api/incidents/bulk', { ids: [...selected], action })
      const failed = Object.keys(r.failed).length
      toast(t(action === 'ack' ? 'incidents.toasts.bulkAck' : 'incidents.toasts.bulkResolve', { n: r.done }) + (failed ? t('incidents.toasts.bulkSkipped', { n: failed }) : ''))
      setSelected(new Set())
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const exportCsv = () => {
    if (!data) return
    const rows = [['id', 'severity', 'status', 'title', 'ci', 'service', 'signal', 'method', 'pagerduty', 'fallback', 'count', 'firstSeen', 'lastSeen'].map((k) => t(`incidents.csv.${k}`))]
    for (const a of data.items) rows.push([a.id, a.severity, a.status, a.title, a.ci_name, a.service ?? '', a.signal, a.method, a.pd_state, String(a.fallback), String(a.count), a.first_seen, a.last_seen])
    const csv = rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(',')).join('\n')
    const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'incidents.csv'
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const openId = params.get('id')
  const openIncident = useCallback((id: string) => update({ id }), [params])
  const counts = data?.counts ?? {}
  const sevFilter = new Set((filters.severity ?? '').split(',').filter(Boolean))

  return (
    <div className="page page-with-side">
      <SideList
        title={t('incidents.views.title')}
        value={activeView}
        onChange={applyView}
        items={[
          ...SYSTEM_VIEWS.map((v) => ({ id: v.id, title: t(`incidents.views.${v.id}`), count: v.id === 'open' ? counts.total : v.id === 'pd' ? counts.pd_not_accepted : undefined })),
        ]}
        footer={
          <>
            <div className="side-list-title side-list-sub">{t('incidents.views.mine')}</div>
            {views.length === 0 && <div className="side-hint">{t('incidents.views.hint')}</div>}
            {views.map((v) => (
              <div key={v.id} className={`side-item side-item-row ${activeView === v.id ? 'side-item-active' : ''}`}>
                <button className="side-item-text" onClick={() => applyView(v.id)}>
                  <Bookmark size={13} /> {v.title}
                </button>
                <button className="icon-btn icon-btn-sm" onClick={() => removeView(v.id)} title={t('common.actions.delete')}>
                  <Trash2 size={13} />
                </button>
              </div>
            ))}
          </>
        }
      />
      <div className="page-main">
        <PageHeader
          title={t('incidents.header.title')}
          sub={t('incidents.header.sub')}
          actions={
            <>
              <button className="btn" onClick={saveView}>
                <Bookmark size={14} /> {t('incidents.header.saveView')}
              </button>
              <button className="btn" onClick={exportCsv}>
                <Download size={14} /> CSV
              </button>
              <button className="btn" onClick={reload} title={t('common.actions.refresh')}>
                <RefreshCw size={14} className={loading ? 'spin' : ''} />
              </button>
            </>
          }
        />

        <div className="indicators">
          {SEVERITIES.map((s) => (
            <button key={s} className={`indicator ind-${s} ${sevFilter.has(s) ? 'ind-active' : ''}`} onClick={() => toggleSev(s)}>
              <span className="ind-label">{sevLabel(s)}</span>
              <span className="ind-value">{counts[s] ?? 0}</span>
              <span className="ind-hint">{t('incidents.indicators.open')}</span>
            </button>
          ))}
          <button className={`indicator ind-pd ${filters.pd ? 'ind-active' : ''}`} onClick={() => update({ pd: filters.pd ? null : 'pending,failed', view: 'open' })}>
            <span className="ind-label">{t('incidents.indicators.pdNotAccepted')}</span>
            <span className="ind-value">{counts.pd_not_accepted ?? 0}</span>
            <span className="ind-hint">{t('incidents.indicators.pdHint')}</span>
          </button>
          <HourlyStrip hourly={data?.hourly ?? []} />
        </div>

        <div className="filterbar">
          <form
            className="search"
            onSubmit={(e) => {
              e.preventDefault()
              update({ q })
            }}
          >
            <Search size={15} />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('incidents.filters.searchPlaceholder')} />
          </form>
          <select value={filters.method ?? ''} onChange={(e) => update({ method: e.target.value })}>
            <option value="">{t('incidents.filters.methodAll')}</option>
            <option value="red">RED</option>
            <option value="use">USE</option>
            <option value="other">{t('incidents.filters.methodOther')}</option>
          </select>
          <select value={filters.pd ?? ''} onChange={(e) => update({ pd: e.target.value })}>
            <option value="">{t('incidents.filters.pdAll')}</option>
            <option value="accepted">{t('common.pd.accepted')}</option>
            <option value="acked">{t('common.pd.acked')}</option>
            <option value="pending,failed">{t('common.pd.failed')}</option>
            <option value="skipped">{t('common.pd.skipped')}</option>
          </select>
          <select value={filters.hours ?? ''} onChange={(e) => update({ hours: e.target.value })}>
            <option value="">{t('incidents.filters.periodAll')}</option>
            <option value="1">{t('incidents.filters.hour1')}</option>
            <option value="6">{t('incidents.filters.hours6')}</option>
            <option value="24">{t('incidents.filters.hours24')}</option>
            <option value="168">{t('incidents.filters.days7')}</option>
          </select>
          <div className="filterbar-spacer" />
          {selected.size > 0 && (
            <div className="bulk">
              <span>{t('incidents.bulk.selected', { n: selected.size })}</span>
              <button className="btn" onClick={() => bulk('ack')}>
                <CheckCheck size={14} /> {t('incidents.bulk.ack')}
              </button>
              <button className="btn" onClick={() => bulk('resolve')}>
                <CircleCheck size={14} /> {t('incidents.bulk.resolve')}
              </button>
              <button className="btn btn-ghost" onClick={() => setSelected(new Set())}>
                <BellOff size={14} /> {t('incidents.bulk.clear')}
              </button>
            </div>
          )}
          <span className="muted">{data ? t('incidents.filters.total', { n: data.total }) : ''}</span>
        </div>

        <div className="card card-flush">
          <IncidentTable
            items={data?.items ?? []}
            selected={selected}
            onSelect={setSelected}
            onOpen={openIncident}
            sort={(filters.sort as SortKey) ?? 'last_seen'}
            desc={filters.order !== 'asc'}
            onSort={onSort}
          />
        </div>
      </div>
      {openId && <IncidentDrawer id={openId} onClose={() => update({ id: null })} onOpen={openIncident} />}
    </div>
  )
}

// HourlyStrip is the mini timeline of incidents opened per hour (24 h).
function HourlyStrip({ hourly }: { hourly: Record<string, number>[] }) {
  const max = Math.max(1, ...hourly.map((h) => SEVERITIES.reduce((s, k) => s + (h[k] ?? 0), 0)))
  const now = new Date()
  return (
    <div className="hourly" title={t('incidents.indicators.hourlyTitle')}>
      <div className="ind-label">{t('incidents.indicators.hourlyLabel')}</div>
      <div className="hourly-bars">
        {hourly.map((h, i) => {
          const hour = (now.getHours() - 23 + i + 24) % 24
          const total = SEVERITIES.reduce((s, k) => s + (h[k] ?? 0), 0)
          return (
            <div key={i} className="hourly-col" title={`${String(hour).padStart(2, '0')}:00 · ${total}`}>
              {SEVERITIES.slice()
                .reverse()
                .map((s) =>
                  h[s] ? <div key={s} className={`hourly-seg bg-${s}`} style={{ height: `${(h[s] / max) * 100}%` }} /> : null,
                )}
            </div>
          )
        })}
      </div>
      <div className="hourly-axis">
        <span>{t('incidents.indicators.minus24')}</span>
        <span>{t('incidents.indicators.minus12')}</span>
        <span>{t('incidents.indicators.now')}</span>
      </div>
    </div>
  )
}
