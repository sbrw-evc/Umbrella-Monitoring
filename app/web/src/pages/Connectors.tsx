import { MoreVertical, Play, Plus, Square, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, fmtTime, qs, type Connector } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { Empty, Field, Modal, PageHeader, SideList } from '../components/ui'
import { t } from '../i18n'

export function ConnectorsPage() {
  const { team, toast, can } = useApp()
  const nav = useNavigate()
  const [filter, setFilter] = useState('all')
  const [creating, setCreating] = useState(false)
  const [menu, setMenu] = useState<string | null>(null)
  const { data, reload } = useFetch<{ items: Connector[] }>(`/api/connectors${qs({ team })}`)
  useLive(['event', 'parse_error'], reload, 3000)

  const all = data?.items ?? []
  const items = all.filter((c) => filter === 'all' || c.status === filter || (filter === 'draft' && c.draft_dirty))

  const act = async (c: Connector, action: 'start' | 'stop' | 'delete') => {
    setMenu(null)
    try {
      if (action === 'delete') {
        if (!window.confirm(t('connectors.confirm.delete', { name: c.name }))) return
        await api.del(`/api/connectors/${c.id}`)
      } else {
        await api.post(`/api/connectors/${c.id}/${action}`)
      }
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  return (
    <div className="page page-with-side">
      <SideList
        title={t('connectors.side.title')}
        value={filter}
        onChange={setFilter}
        items={[
          { id: 'all', title: t('connectors.side.all'), count: all.length },
          { id: 'running', title: t('connectors.side.running'), count: all.filter((c) => c.status === 'running').length },
          { id: 'stopped', title: t('connectors.side.stopped'), count: all.filter((c) => c.status === 'stopped').length },
          { id: 'draft', title: t('connectors.side.draft'), count: all.filter((c) => c.draft_dirty).length },
        ]}
      />
      <div className="page-main">
        <PageHeader
          title={t('connectors.header.title')}
          sub={t('connectors.header.sub')}
          actions={
            can('connectors.edit') && (
              <button className="btn btn-primary" onClick={() => setCreating(true)}>
                <Plus size={14} /> {t('connectors.header.create')}
              </button>
            )
          }
        />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('connectors.table.empty')}</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('connectors.columns.id')}</th>
                  <th>{t('connectors.columns.name')}</th>
                  <th>{t('connectors.columns.status')}</th>
                  <th>{t('connectors.columns.version')}</th>
                  <th>{t('connectors.columns.events')}</th>
                  <th>{t('connectors.columns.errors')}</th>
                  <th>{t('connectors.columns.lastEvent')}</th>
                  <th>{t('connectors.columns.team')}</th>
                  <th>{t('connectors.columns.updated')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map((c) => (
                  <tr key={c.id} onClick={() => nav(`/connectors/${c.id}`)}>
                    <td className="mono nowrap">{c.id}</td>
                    <td className="cell-title">
                      <div className="title-line">{c.name}</div>
                      {c.description && <div className="sub-line">{c.description}</div>}
                    </td>
                    <td>
                      <span className={`pill ${c.status === 'running' ? 'pill-run' : 'pill-muted'}`}>{c.status === 'running' ? t('connectors.status.running') : t('connectors.status.stopped')}</span>
                    </td>
                    <td className="nowrap">
                      {c.version ? `v${c.version}` : '—'}
                      {c.draft_dirty && <span className="dirty-dot" title={t('connectors.table.unpublished')} />}
                    </td>
                    <td className="num">{c.events_total}</td>
                    <td className={`num ${c.errors_total ? 'text-danger' : ''}`}>{c.errors_total}</td>
                    <td className="nowrap">{fmtTime(c.last_event_at)}</td>
                    <td>{c.team}</td>
                    <td className="nowrap">
                      {fmtTime(c.updated_at)}
                      <div className="sub-line">{c.updated_by}</div>
                    </td>
                    <td onClick={(e) => e.stopPropagation()} className="menu-cell">
                      <button className="icon-btn" onClick={() => setMenu(menu === c.id ? null : c.id)} hidden={!can('connectors.edit')}>
                        <MoreVertical size={16} />
                      </button>
                      {menu === c.id && (
                        <div className="row-menu">
                          {c.status === 'running' ? (
                            <button onClick={() => act(c, 'stop')}>
                              <Square size={13} /> {t('connectors.actions.stop')}
                            </button>
                          ) : (
                            <button onClick={() => act(c, 'start')}>
                              <Play size={13} /> {t('connectors.actions.start')}
                            </button>
                          )}
                          <button
                            onClick={() => {
                              navigator.clipboard?.writeText(`${location.origin}/api/ingest/${c.id}`)
                              setMenu(null)
                              toast(t('connectors.toasts.ingestCopied'))
                            }}
                          >
                            {t('connectors.actions.copyIngest')}
                          </button>
                          <button className="danger" onClick={() => act(c, 'delete')}>
                            <Trash2 size={13} /> {t('connectors.actions.delete')}
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {creating && <CreateConnector onClose={() => setCreating(false)} onDone={(id) => nav(`/connectors/${id}`)} />}
    </div>
  )
}

function CreateConnector({ onClose, onDone }: { onClose: () => void; onDone: (id: string) => void }) {
  const { meta, toast } = useApp()
  const [name, setName] = useState('')
  const [team, setTeam] = useState(meta?.teams[0]?.id ?? '')
  const [template, setTemplate] = useState('webhook-json')
  const submit = async () => {
    try {
      const c = await api.post<Connector>('/api/connectors', { name, team, template })
      onDone(c.id)
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <Modal
      title={t('connectors.create.title')}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" disabled={!name} onClick={submit}>
            {t('connectors.create.submit')}
          </button>
        </>
      }
    >
      <Field label={t('connectors.create.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('connectors.create.namePlaceholder')} autoFocus />
      </Field>
      <Field label={t('connectors.create.team')}>
        <select value={team} onChange={(e) => setTeam(e.target.value)}>
          {meta?.teams.map((tm) => (
            <option key={tm.id} value={tm.id}>
              {tm.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('connectors.create.template')}>
        <div className="choice">
          <label className={`choice-item ${template === 'webhook-json' ? 'choice-active' : ''}`}>
            <input type="radio" checked={template === 'webhook-json'} onChange={() => setTemplate('webhook-json')} />
            <b>{t('connectors.create.pushTitle')}</b>
            <span>{t('connectors.create.pushSub')}</span>
          </label>
          <label className={`choice-item ${template === 'pull-http' ? 'choice-active' : ''}`}>
            <input type="radio" checked={template === 'pull-http'} onChange={() => setTemplate('pull-http')} />
            <b>{t('connectors.create.pullTitle')}</b>
            <span>{t('connectors.create.pullSub')}</span>
          </label>
        </div>
      </Field>
    </Modal>
  )
}
