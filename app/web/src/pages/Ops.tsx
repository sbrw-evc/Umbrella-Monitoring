import { ExternalLink, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ciTypeLabel, qs, type CI, type IncidentList, type Relation } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { CiDrawer } from '../components/CiDrawer'
import { CiIcon, CmdbGraph } from '../components/CmdbGraph'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { IncidentTable } from '../components/IncidentTable'
import { Empty, SevBadge, SevDot, Tabs } from '../components/ui'

type Filter = 'all' | 'problem' | 'services'
type Tab = 'graph' | 'incidents'

// Operations center: CI list on the left, the selected CI (or the whole
// model) in the middle with its graph and incidents.
export function OpsPage() {
  const { team } = useApp()
  const [filter, setFilter] = useState<Filter>('problem')
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<string>('')
  const [tab, setTab] = useState<Tab>('graph')
  const [openInc, setOpenInc] = useState<string | null>(null)
  const [openCi, setOpenCi] = useState<string | null>(null)

  const cis = useFetch<{ items: CI[] }>(`/api/cis${qs({ team })}`)
  const graph = useFetch<{ nodes: CI[]; edges: Relation[] }>(`/api/cmdb/graph${qs({ root: sel, depth: sel ? 2 : undefined })}`)
  const selCi = cis.data?.items.find((c) => c.id === sel)
  const incUrl = `/api/incidents${qs({
    team,
    view: 'open',
    sort: 'severity',
    service: selCi?.type === 'it_service' ? selCi.name : undefined,
    ci: selCi && selCi.type !== 'it_service' && selCi.type !== 'business_service' ? selCi.id : undefined,
  })}`
  const incidents = useFetch<IncidentList>(incUrl)
  useLive(['alert'], () => {
    cis.reload()
    graph.reload()
    incidents.reload()
  }, 1200)

  const list = useMemo(() => {
    let l = cis.data?.items ?? []
    if (filter === 'problem') l = l.filter((c) => c.status)
    if (filter === 'services') l = l.filter((c) => c.type === 'business_service' || c.type === 'it_service')
    if (q) l = l.filter((c) => c.name.toLowerCase().includes(q.toLowerCase()))
    return l
  }, [cis.data, filter, q])

  const items = cis.data?.items ?? []
  const problems = items.filter((c) => c.status).length
  const services = items.filter((c) => c.type === 'business_service')

  return (
    <div className="page page-with-side">
      <div className="side-list side-list-wide">
        <div className="side-list-title">{t('ops.side.title')}</div>
        <div className="seg">
          {(['problem', 'all', 'services'] as Filter[]).map((f) => (
            <button key={f} className={`seg-btn ${filter === f ? 'seg-active' : ''}`} onClick={() => setFilter(f)}>
              {f === 'problem' ? t('ops.side.problem', { n: problems }) : f === 'all' ? t('ops.side.all', { n: items.length }) : t('ops.side.services')}
            </button>
          ))}
        </div>
        <div className="search search-sm">
          <Search size={14} />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('ops.side.searchPlaceholder')} />
        </div>
        <button className={`ci-row ${sel === '' ? 'ci-row-active' : ''}`} onClick={() => setSel('')}>
          <span className="ci-row-name">{t('ops.side.wholeModel')}</span>
        </button>
        {list.map((c) => (
          <button key={c.id} className={`ci-row ${sel === c.id ? 'ci-row-active' : ''}`} onClick={() => setSel(c.id)} onDoubleClick={() => setOpenCi(c.id)}>
            <SevDot sev={c.status} />
            <CiIcon type={c.type} size={14} />
            <span className="ci-row-name">{c.name}</span>
            {c.open_alerts > 0 && <span className="side-count">{c.open_alerts}</span>}
          </button>
        ))}
        {list.length === 0 && <div className="side-hint">{t('ops.side.emptyFilter')}</div>}
      </div>

      <div className="page-main">
        <div className="page-header">
          <div>
            <h1>{selCi ? selCi.name : t('ops.header.title')}</h1>
            <div className="page-sub">
              {selCi ? (
                <>
                  {ciTypeLabel(selCi.type)} · <SevBadge sev={selCi.status} />{' '}
                  <button className="link" onClick={() => setOpenCi(selCi.id)}>
                    {t('ops.header.ciCard')} <ExternalLink size={12} />
                  </button>
                </>
              ) : (
                t('ops.header.sub')
              )}
            </div>
          </div>
        </div>

        {!selCi && (
          <div className="svc-tiles">
            {services.map((s) => (
              <button key={s.id} className={`svc-tile svc-${s.status || 'ok'}`} onClick={() => setSel(s.id)}>
                <div className="svc-tile-name">{s.name}</div>
                <div className="svc-tile-state">{s.status ? t('ops.tiles.problem', { status: s.status }) : t('ops.tiles.ok')}</div>
              </button>
            ))}
            <div className="svc-tile svc-summary">
              <div className="svc-tile-name">{incidents.data?.counts.total ?? 0}</div>
              <div className="svc-tile-state">{t('ops.tiles.openIncidents')}</div>
            </div>
          </div>
        )}

        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'graph', title: t('ops.tabs.graph') },
            { id: 'incidents', title: t('ops.tabs.incidents', { n: incidents.data?.total ?? 0 }) },
          ]}
        />
        {tab === 'graph' &&
          (graph.data && graph.data.nodes.length > 0 ? (
            <div className="card card-flush graph-card">
              <CmdbGraph nodes={graph.data.nodes} edges={graph.data.edges} selected={sel} onSelect={setSel} />
            </div>
          ) : (
            <Empty>{t('ops.empty.graph')}</Empty>
          ))}
        {tab === 'incidents' && (
          <div className="card card-flush">
            <IncidentTable items={incidents.data?.items ?? []} onOpen={setOpenInc} compact />
          </div>
        )}
      </div>
      {openInc && <IncidentDrawer id={openInc} onClose={() => setOpenInc(null)} onOpen={setOpenInc} />}
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
    </div>
  )
}
