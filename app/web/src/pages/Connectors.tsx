import { MoreVertical, Play, Plus, Square, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, fmtTime, qs, type Connector } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { Empty, Field, Modal, PageHeader, SideList } from '../components/ui'

export function ConnectorsPage() {
  const { team, toast } = useApp()
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
        if (!window.confirm(`Удалить коннектор «${c.name}»?`)) return
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
        title="Состояние"
        value={filter}
        onChange={setFilter}
        items={[
          { id: 'all', title: 'Все', count: all.length },
          { id: 'running', title: 'Запущены', count: all.filter((c) => c.status === 'running').length },
          { id: 'stopped', title: 'Остановлены', count: all.filter((c) => c.status === 'stopped').length },
          { id: 'draft', title: 'Есть неопубликованные изменения', count: all.filter((c) => c.draft_dirty).length },
        ]}
      />
      <div className="page-main">
        <PageHeader
          title="Коннекторы"
          sub="Подключения к источникам собираются из блоков в конструкторе: получение, парсинг, шаблон, подтверждение"
          actions={
            <button className="btn btn-primary" onClick={() => setCreating(true)}>
              <Plus size={14} /> Создать коннектор
            </button>
          }
        />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>Коннекторов нет</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Название</th>
                  <th>Состояние</th>
                  <th>Версия</th>
                  <th>Событий</th>
                  <th>Ошибок разбора</th>
                  <th>Последнее событие</th>
                  <th>Команда</th>
                  <th>Изменён</th>
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
                      <span className={`pill ${c.status === 'running' ? 'pill-run' : 'pill-muted'}`}>{c.status === 'running' ? 'Запущен' : 'Остановлен'}</span>
                    </td>
                    <td className="nowrap">
                      {c.version ? `v${c.version}` : '—'}
                      {c.draft_dirty && <span className="dirty-dot" title="Есть неопубликованные изменения" />}
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
                      <button className="icon-btn" onClick={() => setMenu(menu === c.id ? null : c.id)}>
                        <MoreVertical size={16} />
                      </button>
                      {menu === c.id && (
                        <div className="row-menu">
                          {c.status === 'running' ? (
                            <button onClick={() => act(c, 'stop')}>
                              <Square size={13} /> Остановить
                            </button>
                          ) : (
                            <button onClick={() => act(c, 'start')}>
                              <Play size={13} /> Запустить
                            </button>
                          )}
                          <button
                            onClick={() => {
                              navigator.clipboard?.writeText(`${location.origin}/api/ingest/${c.id}`)
                              setMenu(null)
                              toast('Адрес приёма скопирован')
                            }}
                          >
                            Копировать адрес приёма
                          </button>
                          <button className="danger" onClick={() => act(c, 'delete')}>
                            <Trash2 size={13} /> Удалить
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
      title="Новый коннектор"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            Отмена
          </button>
          <button className="btn btn-primary" disabled={!name} onClick={submit}>
            Создать и открыть конструктор
          </button>
        </>
      }
    >
      <Field label="Название">
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Например: Webhook сетевого мониторинга" autoFocus />
      </Field>
      <Field label="Команда-владелец">
        <select value={team} onChange={(e) => setTeam(e.target.value)}>
          {meta?.teams.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Заготовка">
        <div className="choice">
          <label className={`choice-item ${template === 'webhook-json' ? 'choice-active' : ''}`}>
            <input type="radio" checked={template === 'webhook-json'} onChange={() => setTemplate('webhook-json')} />
            <b>Push: входящий webhook</b>
            <span>Источник сам присылает события в Umbrella</span>
          </label>
          <label className={`choice-item ${template === 'pull-http' ? 'choice-active' : ''}`}>
            <input type="radio" checked={template === 'pull-http'} onChange={() => setTemplate('pull-http')} />
            <b>Pull: опрос API по расписанию</b>
            <span>Umbrella сама забирает события по HTTP</span>
          </label>
        </div>
      </Field>
    </Modal>
  )
}
