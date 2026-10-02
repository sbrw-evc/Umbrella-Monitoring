import { ExternalLink, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { CI_TYPE_LABEL, qs, type CI, type IncidentList, type Relation } from '../api'
import { useApp, useFetch, useLive } from '../context'
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
        <div className="side-list-title">Конфигурационные единицы</div>
        <div className="seg">
          {(['problem', 'all', 'services'] as Filter[]).map((f) => (
            <button key={f} className={`seg-btn ${filter === f ? 'seg-active' : ''}`} onClick={() => setFilter(f)}>
              {f === 'problem' ? `Проблемные · ${problems}` : f === 'all' ? `Все · ${items.length}` : 'Сервисы'}
            </button>
          ))}
        </div>
        <div className="search search-sm">
          <Search size={14} />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Название КЕ" />
        </div>
        <button className={`ci-row ${sel === '' ? 'ci-row-active' : ''}`} onClick={() => setSel('')}>
          <span className="ci-row-name">Вся модель</span>
        </button>
        {list.map((c) => (
          <button key={c.id} className={`ci-row ${sel === c.id ? 'ci-row-active' : ''}`} onClick={() => setSel(c.id)} onDoubleClick={() => setOpenCi(c.id)}>
            <SevDot sev={c.status} />
            <CiIcon type={c.type} size={14} />
            <span className="ci-row-name">{c.name}</span>
            {c.open_alerts > 0 && <span className="side-count">{c.open_alerts}</span>}
          </button>
        ))}
        {list.length === 0 && <div className="side-hint">Нет КЕ по фильтру</div>}
      </div>

      <div className="page-main">
        <div className="page-header">
          <div>
            <h1>{selCi ? selCi.name : 'Оперативный центр'}</h1>
            <div className="page-sub">
              {selCi ? (
                <>
                  {CI_TYPE_LABEL[selCi.type]} · <SevBadge sev={selCi.status} />{' '}
                  <button className="link" onClick={() => setOpenCi(selCi.id)}>
                    карточка КЕ <ExternalLink size={12} />
                  </button>
                </>
              ) : (
                'Состояние бизнес-услуг и ИТ-сервисов по открытым тревогам'
              )}
            </div>
          </div>
        </div>

        {!selCi && (
          <div className="svc-tiles">
            {services.map((s) => (
              <button key={s.id} className={`svc-tile svc-${s.status || 'ok'}`} onClick={() => setSel(s.id)}>
                <div className="svc-tile-name">{s.name}</div>
                <div className="svc-tile-state">{s.status ? `проблема: ${s.status}` : 'работает штатно'}</div>
              </button>
            ))}
            <div className="svc-tile svc-summary">
              <div className="svc-tile-name">{incidents.data?.counts.total ?? 0}</div>
              <div className="svc-tile-state">открытых инцидентов</div>
            </div>
          </div>
        )}

        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'graph', title: 'Граф РСМ' },
            { id: 'incidents', title: `Инциденты · ${incidents.data?.total ?? 0}` },
          ]}
        />
        {tab === 'graph' &&
          (graph.data && graph.data.nodes.length > 0 ? (
            <div className="card card-flush graph-card">
              <CmdbGraph nodes={graph.data.nodes} edges={graph.data.edges} selected={sel} onSelect={setSel} />
            </div>
          ) : (
            <Empty>Нет данных карты</Empty>
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
