import { Plus, Search, Table2, Workflow } from 'lucide-react'
import { useState } from 'react'
import { api, CI_TYPE_LABEL, fmtTime, qs, type CI, type Relation } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { CiDrawer } from '../components/CiDrawer'
import { CiIcon, CmdbGraph } from '../components/CmdbGraph'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { Empty, Field, Modal, PageHeader, SevBadge, SideList } from '../components/ui'

export function CmdbPage() {
  const { team, toast } = useApp()
  const [filter, setFilter] = useState('all')
  const [q, setQ] = useState('')
  const [mode, setMode] = useState<'table' | 'graph'>('table')
  const [openCi, setOpenCi] = useState<string | null>(null)
  const [openInc, setOpenInc] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const isType = filter.startsWith('type:')
  const url = `/api/cis${qs({ team, q, state: filter === 'problem' || filter === 'ok' ? filter : undefined, type: isType ? filter.slice(5) : undefined })}`
  const { data, reload } = useFetch<{ items: CI[] }>(url)
  const all = useFetch<{ items: CI[] }>(`/api/cis${qs({ team })}`)
  const graph = useFetch<{ nodes: CI[]; edges: Relation[] }>(mode === 'graph' ? '/api/cmdb/graph' : null)
  useLive(['alert'], () => {
    reload()
    all.reload()
    if (mode === 'graph') graph.reload()
  }, 1500)

  const items = data?.items ?? []
  const allItems = all.data?.items ?? []
  const types = Object.keys(CI_TYPE_LABEL).filter((t) => allItems.some((c) => c.type === t))

  return (
    <div className="page page-with-side">
      <SideList
        title="Фильтры CMDB"
        value={filter}
        onChange={setFilter}
        items={[
          { id: 'all', title: 'Все', count: allItems.length },
          { id: 'problem', title: 'Проблемные', count: allItems.filter((c) => c.status).length },
          { id: 'ok', title: 'OK', count: allItems.filter((c) => !c.status).length },
          ...types.map((t) => ({ id: `type:${t}`, title: CI_TYPE_LABEL[t], count: allItems.filter((c) => c.type === t).length, icon: <CiIcon type={t} size={14} /> })),
        ]}
      />
      <div className="page-main">
        <PageHeader
          title="Карта CMDB"
          sub="КЕ и связи строит CMDB Discovery по данным мониторинга; бизнес-услуги задаются вручную"
          actions={
            <>
              <div className="seg">
                <button className={`seg-btn ${mode === 'table' ? 'seg-active' : ''}`} onClick={() => setMode('table')}>
                  <Table2 size={14} /> Таблица
                </button>
                <button className={`seg-btn ${mode === 'graph' ? 'seg-active' : ''}`} onClick={() => setMode('graph')}>
                  <Workflow size={14} /> Граф
                </button>
              </div>
              <button className="btn btn-primary" onClick={() => setCreating(true)}>
                <Plus size={14} /> Создать КЕ
              </button>
            </>
          }
        />
        {mode === 'table' ? (
          <>
            <div className="filterbar">
              <div className="search">
                <Search size={15} />
                <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="ID, название, описание, группа" />
              </div>
              <div className="filterbar-spacer" />
              <span className="muted">{items.length} КЕ</span>
            </div>
            <div className="card card-flush">
              {items.length === 0 ? (
                <Empty>КЕ не найдены</Empty>
              ) : (
                <table className="table">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>Название</th>
                      <th>Тип</th>
                      <th>Состояние</th>
                      <th>Открытые тревоги</th>
                      <th>Команда</th>
                      <th>Идентификаторы</th>
                      <th>Связи</th>
                      <th>Источник</th>
                      <th>Создана</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((c) => (
                      <tr key={c.id} onClick={() => setOpenCi(c.id)}>
                        <td className="mono nowrap">{c.id}</td>
                        <td className="cell-title">
                          <div className="title-line">{c.name}</div>
                          {c.description && <div className="sub-line">{c.description}</div>}
                        </td>
                        <td className="nowrap">
                          <span className="type-cell">
                            <CiIcon type={c.type} size={14} /> {CI_TYPE_LABEL[c.type] ?? c.type}
                          </span>
                        </td>
                        <td>
                          <SevBadge sev={c.status} />
                          {c.maintenance && <span className="tag">обслуживание</span>}
                        </td>
                        <td className="num">{c.open_alerts || ''}</td>
                        <td>{c.team}</td>
                        <td className="num">{c.identities.length}</td>
                        <td className="num">
                          ↑{c.parents} ↓{c.children}
                        </td>
                        <td>{c.origin === 'discovery' ? 'Discovery' : 'вручную'}</td>
                        <td className="nowrap">{fmtTime(c.created_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </>
        ) : (
          <div className="card card-flush graph-card graph-card-tall">
            {graph.data && <CmdbGraph nodes={graph.data.nodes} edges={graph.data.edges} onSelect={setOpenCi} />}
          </div>
        )}
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
      {creating && (
        <CreateCi
          all={allItems}
          onClose={() => setCreating(false)}
          onDone={() => {
            setCreating(false)
            reload()
            all.reload()
            toast('КЕ создана')
          }}
        />
      )}
    </div>
  )
}

function CreateCi({ all, onClose, onDone }: { all: CI[]; onClose: () => void; onDone: () => void }) {
  const { meta, toast } = useApp()
  const [name, setName] = useState('')
  const [type, setType] = useState('business_service')
  const [team, setTeam] = useState(meta?.teams[0]?.id ?? '')
  const [parent, setParent] = useState('')
  const [description, setDescription] = useState('')
  const submit = async () => {
    try {
      await api.post('/api/cis', { name, type, team, parent, description })
      onDone()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <Modal
      title="Новая КЕ"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            Отмена
          </button>
          <button className="btn btn-primary" onClick={submit} disabled={!name}>
            Создать
          </button>
        </>
      }
    >
      <Field label="Название">
        <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Field>
      <Field label="Тип">
        <select value={type} onChange={(e) => setType(e.target.value)}>
          {Object.entries(CI_TYPE_LABEL).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Команда">
        <select value={team} onChange={(e) => setTeam(e.target.value)}>
          {meta?.teams.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Родительская КЕ" help="Новая КЕ станет зависимостью выбранной">
        <select value={parent} onChange={(e) => setParent(e.target.value)}>
          <option value="">—</option>
          {all.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Описание">
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </Field>
    </Modal>
  )
}
