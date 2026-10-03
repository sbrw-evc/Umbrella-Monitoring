import { Plus, Search, Table2, Workflow } from 'lucide-react'
import { useState } from 'react'
import { api, ciTypeLabel, fmtTime, qs, type CI, type Relation } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { CiDrawer } from '../components/CiDrawer'
import { originLabel } from './Cis'
import { CiIcon, CmdbGraph } from '../components/CmdbGraph'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { Empty, Field, Modal, PageHeader, SevBadge, SideList } from '../components/ui'

const CI_TYPES = ['business_service', 'it_service', 'host', 'database', 'cloud_group', 'deployment', 'network']

export function CmdbPage() {
  const { team, toast, can } = useApp()
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
  const types = CI_TYPES.filter((ty) => allItems.some((c) => c.type === ty))

  return (
    <div className="page page-with-side">
      <SideList
        title={t('cmdb.filters.title')}
        value={filter}
        onChange={setFilter}
        items={[
          { id: 'all', title: t('common.words.all'), count: allItems.length },
          { id: 'problem', title: t('cmdb.filters.problem'), count: allItems.filter((c) => c.status).length },
          { id: 'ok', title: t('cmdb.filters.ok'), count: allItems.filter((c) => !c.status).length },
          ...types.map((ty) => ({ id: `type:${ty}`, title: ciTypeLabel(ty), count: allItems.filter((c) => c.type === ty).length, icon: <CiIcon type={ty} size={14} /> })),
        ]}
      />
      <div className="page-main">
        <PageHeader
          title={t('cmdb.header.title')}
          sub={t('cmdb.header.sub')}
          actions={
            <>
              <div className="seg">
                <button className={`seg-btn ${mode === 'table' ? 'seg-active' : ''}`} onClick={() => setMode('table')}>
                  <Table2 size={14} /> {t('cmdb.header.table')}
                </button>
                <button className={`seg-btn ${mode === 'graph' ? 'seg-active' : ''}`} onClick={() => setMode('graph')}>
                  <Workflow size={14} /> {t('cmdb.header.graph')}
                </button>
              </div>
              {can('cmdb.edit') && (
                <button className="btn btn-primary" onClick={() => setCreating(true)}>
                  <Plus size={14} /> {t('cmdb.header.create')}
                </button>
              )}
            </>
          }
        />
        {mode === 'table' ? (
          <>
            <div className="filterbar">
              <div className="search">
                <Search size={15} />
                <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('cmdb.filters.searchPlaceholder')} />
              </div>
              <div className="filterbar-spacer" />
              <span className="muted">{t('cmdb.filters.count', { n: items.length })}</span>
            </div>
            <div className="card card-flush">
              {items.length === 0 ? (
                <Empty>{t('cmdb.table.empty')}</Empty>
              ) : (
                <table className="table">
                  <thead>
                    <tr>
                      <th>{t('cmdb.table.id')}</th>
                      <th>{t('cmdb.table.name')}</th>
                      <th>{t('cmdb.table.type')}</th>
                      <th>{t('cmdb.table.state')}</th>
                      <th>{t('cmdb.table.openAlerts')}</th>
                      <th>{t('cmdb.table.team')}</th>
                      <th>{t('cmdb.table.identities')}</th>
                      <th>{t('cmdb.table.relations')}</th>
                      <th>{t('cmdb.table.source')}</th>
                      <th>{t('cmdb.table.created')}</th>
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
                            <CiIcon type={c.type} size={14} /> {ciTypeLabel(c.type)}
                          </span>
                        </td>
                        <td>
                          <SevBadge sev={c.status} />
                          {c.maintenance && <span className="tag">{t('cmdb.table.maintenance')}</span>}
                        </td>
                        <td className="num">{c.open_alerts || ''}</td>
                        <td>{c.team}</td>
                        <td className="num">{c.identities.length}</td>
                        <td className="num">
                          ↑{c.parents} ↓{c.children}
                        </td>
                        <td>{originLabel(c.origin)}</td>
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
            {graph.data && <CmdbGraph nodes={graph.data.nodes} edges={graph.data.edges} selected={openCi ?? undefined} onSelect={setOpenCi} />}
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
            toast(t('cmdb.create.done'))
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
      title={t('cmdb.create.title')}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={submit} disabled={!name}>
            {t('common.actions.create')}
          </button>
        </>
      }
    >
      <Field label={t('cmdb.create.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Field>
      <Field label={t('cmdb.create.type')}>
        <select value={type} onChange={(e) => setType(e.target.value)}>
          {CI_TYPES.map((k) => (
            <option key={k} value={k}>
              {ciTypeLabel(k)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('cmdb.create.team')}>
        <select value={team} onChange={(e) => setTeam(e.target.value)}>
          {meta?.teams.map((tm) => (
            <option key={tm.id} value={tm.id}>
              {tm.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('cmdb.create.parent')} help={t('cmdb.create.parentHelp')}>
        <select value={parent} onChange={(e) => setParent(e.target.value)}>
          <option value="">—</option>
          {all.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('cmdb.create.description')}>
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </Field>
    </Modal>
  )
}
