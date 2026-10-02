import { Clock, LayoutGrid, Wrench } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ciTypeLabel, dateLocaleTime, qs, sevLabel, SEVERITIES, type Severity } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { CiDrawer } from '../components/CiDrawer'
import { CiIcon } from '../components/CmdbGraph'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { Empty, PageHeader, SevDot, Tabs } from '../components/ui'

interface HeatCell {
  n: number
  sev?: Severity
}
interface HeatRow {
  ci_id?: string
  ci_name: string
  ci_type?: string
  status: Severity | ''
  open: number
  total: number
  maintenance: boolean
  cells: HeatCell[]
}
interface HeatGroup {
  key: string
  name: string
  status: Severity | ''
  total: number
  rows: HeatRow[]
}
interface Heatmap {
  hours: number
  bucket_minutes: number
  from: string
  to: string
  group: string
  columns: string[]
  max: number
  totals: HeatCell[]
  groups: HeatGroup[]
}

type View = 'time' | 'state'
type GroupBy = 'service' | 'team' | 'type'
const PERIODS = [6, 24, 72, 168]

function load<T extends string>(key: string, def: T, allowed: readonly T[]): T {
  try {
    const v = localStorage.getItem(key) as T | null
    return v && allowed.includes(v) ? v : def
  } catch {
    return def
  }
}

function save(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* ignore */
  }
}

// Cell opacity grows with the number of incidents, so a busy hour stands out
// while one incident is still visible.
function cellStyle(c: HeatCell, max: number) {
  if (!c.n || !c.sev) return undefined
  const k = max > 1 ? (c.n - 1) / (max - 1) : 1
  return { background: `var(--severity-${c.sev})`, opacity: 0.45 + 0.55 * k }
}

// Heatmap page: CIs against time (when and where incidents were active) and
// a tile map of the current state, grouped by IT service, team or CI type.
export function HeatmapPage() {
  const { team, meta } = useApp()
  const nav = useNavigate()
  const [view, setView] = useState<View>(() => load('umb.heat.view', 'time', ['time', 'state'] as const))
  const [group, setGroup] = useState<GroupBy>(() => load('umb.heat.group', 'service', ['service', 'team', 'type'] as const))
  const [hours, setHours] = useState(() => Number(load('umb.heat.hours', '24', ['6', '24', '72', '168'] as const)))
  const [problems, setProblems] = useState(() => load('umb.heat.problems', '1', ['0', '1'] as const) === '1')
  const [openCi, setOpenCi] = useState<string | null>(null)
  const [openInc, setOpenInc] = useState<string | null>(null)

  const { data, reload } = useFetch<Heatmap>(`/api/heatmap${qs({ hours, group, team, state: problems ? 'problem' : undefined })}`)
  useLive(['alert'], reload, 1500)

  const groupName = (g: HeatGroup) => {
    if (!g.key) return t(`heatmap.group.none.${group}`)
    if (g.key === 'business') return t('heatmap.group.business')
    if (group === 'type') return ciTypeLabel(g.key)
    if (group === 'team') return meta?.teams.find((x) => x.id === g.key)?.name ?? g.name
    return g.name
  }

  // A click on a row or cell opens the CI's incidents for the period.
  const openIncidents = (row: HeatRow) => {
    const h = hours <= 24 ? String(hours) : '168'
    if (row.ci_id) nav(`/incidents${qs({ view: 'all', ci: row.ci_id, hours: h })}`)
    else nav(`/incidents${qs({ view: 'all', q: row.ci_name, hours: h })}`)
  }

  const rowsTotal = data?.groups.reduce((n, g) => n + g.rows.length, 0) ?? 0

  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('heatmap.header.title')}
          sub={t('heatmap.header.sub')}
          actions={
            <>
              <div className="seg" title={t('heatmap.controls.period')}>
                {PERIODS.map((p) => (
                  <button
                    key={p}
                    className={`seg-btn ${hours === p ? 'seg-active' : ''}`}
                    onClick={() => {
                      setHours(p)
                      save('umb.heat.hours', String(p))
                    }}
                  >
                    {t(`heatmap.periods.h${p}`)}
                  </button>
                ))}
              </div>
              <label className="inline-field">
                <span>{t('heatmap.controls.groupBy')}</span>
                <select
                  value={group}
                  onChange={(e) => {
                    setGroup(e.target.value as GroupBy)
                    save('umb.heat.group', e.target.value)
                  }}
                >
                  <option value="service">{t('heatmap.groupBy.service')}</option>
                  <option value="team">{t('heatmap.groupBy.team')}</option>
                  <option value="type">{t('heatmap.groupBy.type')}</option>
                </select>
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={problems}
                  onChange={(e) => {
                    setProblems(e.target.checked)
                    save('umb.heat.problems', e.target.checked ? '1' : '0')
                  }}
                />
                {t('heatmap.controls.problemsOnly')}
              </label>
            </>
          }
        />
        <Tabs
          tabs={[
            { id: 'time', title: t('heatmap.tabs.time'), icon: <Clock size={16} /> },
            { id: 'state', title: t('heatmap.tabs.state'), icon: <LayoutGrid size={16} /> },
          ]}
          value={view}
          onChange={(v) => {
            setView(v)
            save('umb.heat.view', v)
          }}
        />
        {!data ? (
          <Empty>{t('common.words.loading')}</Empty>
        ) : rowsTotal === 0 ? (
          <div className="card">
            <Empty>{problems ? t('heatmap.empty.problems') : t('heatmap.empty.all')}</Empty>
          </div>
        ) : view === 'time' ? (
          <TimeMatrix data={data} groupName={groupName} onRow={openIncidents} onCi={setOpenCi} />
        ) : (
          <StateTiles data={data} groupName={groupName} onCi={(row) => (row.ci_id ? setOpenCi(row.ci_id) : openIncidents(row))} />
        )}
        <Legend matrix={view === 'time'} max={data?.max ?? 0} />
      </div>
      {openCi && (
        <CiDrawer
          id={openCi}
          onClose={() => setOpenCi(null)}
          onOpenCi={setOpenCi}
          onOpenIncident={(id) => {
            setOpenCi(null)
            setOpenInc(id)
          }}
        />
      )}
      {openInc && <IncidentDrawer id={openInc} onClose={() => setOpenInc(null)} onOpen={setOpenInc} />}
    </div>
  )
}

function TimeMatrix({
  data,
  groupName,
  onRow,
  onCi,
}: {
  data: Heatmap
  groupName: (g: HeatGroup) => string
  onRow: (r: HeatRow) => void
  onCi: (id: string) => void
}) {
  const multiDay = data.hours > 24
  const label = (s: string) => dateLocaleTime(s, multiDay)
  const range = (i: number) => {
    const start = data.columns[i]
    const end = new Date(new Date(start).getTime() + data.bucket_minutes * 60000).toISOString()
    return `${label(start)} – ${label(end)}`
  }
  const cellTitle = (name: string, c: HeatCell, i: number) =>
    c.n ? `${name} · ${range(i)}\n${t('heatmap.cell.active', { n: c.n })} · ${t('heatmap.cell.worst')}: ${sevLabel(c.sev ?? '')}` : `${name} · ${range(i)}\n${t('heatmap.cell.none')}`
  // Label every fourth column so the axis stays readable.
  return (
    <div className="card card-flush heat-card">
      <div className="heat" style={{ gridTemplateColumns: `minmax(220px, 280px) repeat(${data.columns.length}, minmax(22px, 1fr)) 64px` }}>
        <div className="heat-corner">{t('heatmap.matrix.ci')}</div>
        {data.columns.map((c, i) => (
          <div key={c} className="heat-axis">
            {i % 4 === 0 ? label(c) : ''}
          </div>
        ))}
        <div className="heat-axis heat-axis-total">{t('heatmap.matrix.total')}</div>

        {data.groups.map((g) => (
          <Fragment key={g.key || '-'}>
            <div className="heat-group" style={{ gridColumn: `1 / span ${data.columns.length + 2}` }}>
              <SevDot sev={g.status} />
              <span>{groupName(g)}</span>
              <span className="muted">{t('heatmap.matrix.groupStat', { rows: g.rows.length, n: g.total })}</span>
            </div>
            {g.rows.map((r) => (
              <Fragment key={r.ci_id ?? r.ci_name}>
                <button className="heat-label" onClick={() => (r.ci_id ? onCi(r.ci_id) : onRow(r))} title={t('heatmap.matrix.openCi')}>
                  <SevDot sev={r.status} />
                  <span className="heat-icon">
                    <CiIcon type={r.ci_type ?? ''} size={15} />
                  </span>
                  <span className="heat-name">{r.ci_name}</span>
                  {r.maintenance && <Wrench size={13} className="muted" />}
                  {!r.ci_id && <span className="tag tag-warn">{t('heatmap.matrix.noCi')}</span>}
                  {r.open > 0 && <span className="side-count">{r.open}</span>}
                </button>
                {r.cells.map((c, i) => (
                  <button key={i} className={`heat-cell ${c.n ? 'heat-cell-on' : ''}`} title={cellTitle(r.ci_name, c, i)} onClick={() => onRow(r)}>
                    <span style={cellStyle(c, data.max)}>{c.n > 1 ? c.n : ''}</span>
                  </button>
                ))}
                <div className="heat-total">{r.total || ''}</div>
              </Fragment>
            ))}
          </Fragment>
        ))}

        <div className="heat-label heat-label-total">{t('heatmap.matrix.allCis')}</div>
        {data.totals.map((c, i) => (
          <div key={i} className="heat-cell heat-cell-total" title={cellTitle(t('heatmap.matrix.allCis'), c, i)}>
            <span>{c.n || ''}</span>
          </div>
        ))}
        <div className="heat-total" />
      </div>
    </div>
  )
}

function StateTiles({ data, groupName, onCi }: { data: Heatmap; groupName: (g: HeatGroup) => string; onCi: (r: HeatRow) => void }) {
  return (
    <div className="tile-groups">
      {data.groups.map((g) => (
        <section key={g.key || '-'} className="card tile-group">
          <div className="tile-group-head">
            <SevDot sev={g.status} />
            <b>{groupName(g)}</b>
            <span className="muted">{t('heatmap.tiles.count', { n: g.rows.length })}</span>
          </div>
          <div className="tiles">
            {g.rows.map((r) => (
              <button key={r.ci_id ?? r.ci_name} className={`tile tile-${r.status || 'ok'}`} onClick={() => onCi(r)} title={`${r.ci_name}: ${sevLabel(r.status)}`}>
                <span className="tile-top">
                  <CiIcon type={r.ci_type ?? ''} size={15} />
                  {r.maintenance && <Wrench size={13} />}
                  {r.open > 0 && <b className="tile-open">{r.open}</b>}
                </span>
                <span className="tile-name">{r.ci_name}</span>
                <span className="tile-sub">
                  {r.open > 0 ? t('heatmap.tiles.open', { n: r.open }) : r.status ? t('heatmap.tiles.below') : t('heatmap.tiles.ok')}
                  {r.total > 0 && ` · ${t('heatmap.tiles.period', { n: r.total })}`}
                </span>
              </button>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function Legend({ matrix, max }: { matrix: boolean; max: number }) {
  return (
    <div className="heat-legend">
      {SEVERITIES.map((s) => (
        <span key={s} className="heat-legend-item">
          <i style={{ background: `var(--severity-${s})` }} />
          {sevLabel(s)}
        </span>
      ))}
      <span className="heat-legend-item">
        <i className={matrix ? 'heat-legend-empty' : undefined} style={matrix ? undefined : { background: 'var(--severity-ok)' }} />
        {matrix ? t('heatmap.legend.none') : t('heatmap.tiles.ok')}
      </span>
      {matrix && max > 1 && <span className="muted">{t('heatmap.legend.intensity', { max })}</span>}
    </div>
  )
}
