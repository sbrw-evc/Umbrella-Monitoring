import { CheckCheck, CircleCheck, ExternalLink, Link2 } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, CI_TYPE_LABEL, fmtDuration, fmtTime, METHOD_LABEL, type EventItem, type Incident } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { Drawer, Empty, PDPill, SevBadge, StatusPill, Tabs } from './ui'

interface Detail {
  incident: Incident
  events: EventItem[]
  related: Incident | null
  grafana_url: string
}

type Tab = 'main' | 'events' | 'timeline' | 'pd' | 'comments'

// IncidentDrawer is the "i" side panel: everything about one incident.
export function IncidentDrawer({ id, onClose, onOpen }: { id: string; onClose: () => void; onOpen?: (id: string) => void }) {
  const { toast } = useApp()
  const { data, reload } = useFetch<Detail>(`/api/incidents/${id}`)
  const [tab, setTab] = useState<Tab>('main')
  const [comment, setComment] = useState('')
  useLive(['alert'], reload, 400)

  const act = async (action: string, text?: string) => {
    try {
      await api.post(`/api/incidents/${id}/${action}`, text ? { text } : {})
      reload()
      if (action === 'comment') setComment('')
      else toast(action === 'ack' ? 'Инцидент подтверждён' : 'Инцидент решён')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const inc = data?.incident
  const timeline = inc?.timeline ?? []
  return (
    <Drawer
      open
      onClose={onClose}
      wide
      title={
        <span className="drawer-title-row">
          {inc && <SevBadge sev={inc.severity} />}
          <span>{inc ? inc.title : id}</span>
        </span>
      }
      sub={inc && <>{inc.id} · {inc.ci_name}{inc.service ? ` · ${inc.service}` : ''}</>}
      actions={
        inc && (
          <>
            <a className="btn" href={data!.grafana_url} target="_blank" rel="noreferrer">
              <ExternalLink size={14} /> Контекст в Grafana
            </a>
            {inc.status === 'open' && (
              <button className="btn" onClick={() => act('ack')}>
                <CheckCheck size={14} /> Подтвердить
              </button>
            )}
            {inc.status !== 'resolved' && (
              <button className="btn btn-primary" onClick={() => act('resolve')}>
                <CircleCheck size={14} /> Решить
              </button>
            )}
          </>
        )
      }
    >
      {!inc ? (
        <Empty>Загрузка…</Empty>
      ) : (
        <>
          <Tabs<Tab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: 'main', title: 'Основное' },
              { id: 'events', title: `События · ${data!.events.length}` },
              { id: 'timeline', title: 'Хронология' },
              { id: 'pd', title: 'PagerDuty и резерв' },
              { id: 'comments', title: `Комментарии · ${timeline.filter((t) => t.kind === 'comment').length}` },
            ]}
          />
          {tab === 'main' && (
            <div className="props">
              <Prop k="Статус"><StatusPill status={inc.status} />{inc.suppressed && <span className="pill pill-muted">Подавлен окном</span>}</Prop>
              <Prop k="Severity"><SevBadge sev={inc.severity} /></Prop>
              <Prop k="КЕ">{inc.ci_name} {inc.ci_type && <span className="muted">· {CI_TYPE_LABEL[inc.ci_type] ?? inc.ci_type}</span>}{!inc.ci_id && <span className="pill pill-muted">без КЕ</span>}</Prop>
              <Prop k="ИТ-сервис">{inc.service || '—'}</Prop>
              <Prop k="Команда">{inc.team || '—'}</Prop>
              <Prop k="Сигнал"><code>{inc.signal}</code> <span className="tag">{METHOD_LABEL[inc.method]}</span></Prop>
              <Prop k="Источники">{Object.entries(inc.sources).map(([s, st]) => <span key={s} className={`tag ${st === 'resolved' ? 'tag-ok' : ''}`}>{s}: {st === 'resolved' ? 'норма' : 'активно'}</span>)}</Prop>
              <Prop k="Событий">{inc.count}</Prop>
              <Prop k="Открыт">{fmtTime(inc.first_seen)}</Prop>
              <Prop k="Последнее событие">{fmtTime(inc.last_seen)}</Prop>
              <Prop k="Длительность">{fmtDuration(inc.first_seen, inc.resolved_at)}</Prop>
              {inc.acked_by && <Prop k="Подтвердил">{inc.acked_by}</Prop>}
              <Prop k="Ключ схлопывания"><code>{inc.dedup_key}</code></Prop>
              {data!.related && (
                <Prop k={inc.method === 'red' ? 'Вероятная причина' : 'Влияет на'}>
                  <button className="link" onClick={() => onOpen?.(data!.related!.id)}>
                    <Link2 size={13} /> {data!.related.id} · {data!.related.title}
                  </button>
                </Prop>
              )}
            </div>
          )}
          {tab === 'events' && (
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>Время</th>
                  <th>Источник</th>
                  <th>Severity</th>
                  <th>Статус</th>
                  <th>Событие</th>
                </tr>
              </thead>
              <tbody>
                {data!.events.map((e) => (
                  <tr key={e.id} title={e.raw}>
                    <td className="nowrap">{fmtTime(e.received_at)}</td>
                    <td>{e.source}</td>
                    <td><SevBadge sev={e.severity} /></td>
                    <td>{e.status === 'resolved' ? 'норма' : 'активно'}</td>
                    <td>
                      {e.title}
                      <div className="raw">{e.raw}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {tab === 'timeline' && <Timeline items={timeline} />}
          {tab === 'pd' && (
            <div className="props">
              <Prop k="Состояние в PagerDuty"><PDPill state={inc.pd_state} fallback={inc.fallback} /></Prop>
              <Prop k="dedup_key"><code>{inc.pd_dedup_key}</code></Prop>
              {inc.pd_error && <Prop k="Последняя ошибка"><span className="text-danger">{inc.pd_error}</span></Prop>}
              <Prop k="Резервное оповещение">{inc.fallback ? 'было: почта и webhook дежурным' : 'не требовалось'}</Prop>
              <div className="section-title">Отправки</div>
              <Timeline items={timeline.filter((t) => t.kind === 'pagerduty' || t.kind === 'fallback')} />
            </div>
          )}
          {tab === 'comments' && (
            <div>
              <Timeline items={timeline.filter((t) => t.kind === 'comment')} empty="Комментариев пока нет" />
              {inc.status !== 'resolved' && (
                <form
                  className="comment-form"
                  onSubmit={(e) => {
                    e.preventDefault()
                    if (comment.trim()) act('comment', comment)
                  }}
                >
                  <textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Комментарий для смены" rows={3} />
                  <button className="btn btn-primary" type="submit">
                    Добавить
                  </button>
                </form>
              )}
            </div>
          )}
        </>
      )}
    </Drawer>
  )
}

function Prop({ k, children }: { k: string; children: ReactNode }) {
  return (
    <div className="prop">
      <div className="prop-k">{k}</div>
      <div className="prop-v">{children}</div>
    </div>
  )
}

const KIND_LABEL: Record<string, string> = {
  event: 'событие',
  status: 'статус',
  pagerduty: 'PagerDuty',
  fallback: 'резерв',
  comment: 'комментарий',
  maintenance: 'обслуживание',
}

function Timeline({ items, empty = 'Записей нет' }: { items: { at: string; kind: string; text: string; author?: string }[]; empty?: string }) {
  if (items.length === 0) return <Empty>{empty}</Empty>
  return (
    <ul className="timeline">
      {[...items].reverse().map((t, i) => (
        <li key={i} className={`tl tl-${t.kind}`}>
          <span className="tl-time">{fmtTime(t.at)}</span>
          <span className="tl-kind">{KIND_LABEL[t.kind] ?? t.kind}</span>
          <span className="tl-text">
            {t.text}
            {t.author && <span className="muted"> · {t.author}</span>}
          </span>
        </li>
      ))}
    </ul>
  )
}
