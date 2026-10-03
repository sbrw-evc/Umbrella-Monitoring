import { CheckCheck, CircleCheck, ExternalLink, Link2 } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, ciTypeLabel, fmtDuration, fmtTime, methodLabel, type EventItem, type Incident } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { Drawer, Empty, PDPill, SevBadge, StatusPill, Tabs } from './ui'

interface Detail {
  incident: Incident
  events: EventItem[]
  related: Incident | null
  grafana_url?: string
}

type Tab = 'main' | 'events' | 'timeline' | 'pd' | 'comments'

// IncidentDrawer is the "i" side panel: everything about one incident.
export function IncidentDrawer({ id, onClose, onOpen }: { id: string; onClose: () => void; onOpen?: (id: string) => void }) {
  const { toast, can } = useApp()
  const canAct = can('incidents.act')
  const { data, reload } = useFetch<Detail>(`/api/incidents/${id}`)
  const [tab, setTab] = useState<Tab>('main')
  const [comment, setComment] = useState('')
  useLive(['alert'], reload, 400)

  const act = async (action: string, text?: string) => {
    try {
      await api.post(`/api/incidents/${id}/${action}`, text ? { text } : {})
      reload()
      if (action === 'comment') setComment('')
      else toast(t(action === 'ack' ? 'incidents.toasts.acked' : 'incidents.toasts.resolved'))
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
            {data?.grafana_url && (
              <a className="btn" href={data.grafana_url} target="_blank" rel="noreferrer">
                <ExternalLink size={14} /> {t('incidents.drawer.grafana')}
              </a>
            )}
            {inc.pd_incident_url && (
              <a className="btn" href={inc.pd_incident_url} target="_blank" rel="noreferrer">
                <ExternalLink size={14} /> PagerDuty
              </a>
            )}
            {canAct && inc.status === 'open' && (
              <button className="btn" onClick={() => act('ack')}>
                <CheckCheck size={14} /> {t('incidents.drawer.ack')}
              </button>
            )}
            {canAct && inc.status !== 'resolved' && (
              <button className="btn btn-primary" onClick={() => act('resolve')}>
                <CircleCheck size={14} /> {t('incidents.drawer.resolve')}
              </button>
            )}
          </>
        )
      }
    >
      {!inc ? (
        <Empty>{t('common.words.loading')}</Empty>
      ) : (
        <>
          <Tabs<Tab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: 'main', title: t('incidents.tabs.main') },
              { id: 'events', title: t('incidents.tabs.events', { n: data!.events.length }) },
              { id: 'timeline', title: t('incidents.tabs.timeline') },
              { id: 'pd', title: t('incidents.tabs.pd') },
              { id: 'comments', title: t('incidents.tabs.comments', { n: timeline.filter((x) => x.kind === 'comment').length }) },
            ]}
          />
          {tab === 'main' && (
            <div className="props">
              <Prop k={t('common.words.status')}><StatusPill status={inc.status} />{inc.suppressed && <span className="pill pill-muted">{t('incidents.drawer.suppressed')}</span>}</Prop>
              <Prop k={t('incidents.drawer.severity')}><SevBadge sev={inc.severity} /></Prop>
              <Prop k={t('incidents.drawer.ci')}>{inc.ci_name} {inc.ci_type && <span className="muted">· {ciTypeLabel(inc.ci_type)}</span>}{!inc.ci_id && <span className="pill pill-muted">{t('incidents.drawer.noCi')}</span>}</Prop>
              <Prop k={t('incidents.drawer.service')}>{inc.service || '—'}</Prop>
              <Prop k={t('common.words.team')}>{inc.team || '—'}</Prop>
              <Prop k={t('incidents.drawer.signal')}><code>{inc.signal}</code> <span className="tag">{methodLabel(inc.method)}</span></Prop>
              <Prop k={t('incidents.drawer.sources')}>{Object.entries(inc.sources).map(([s, st]) => <span key={s} className={`tag ${st === 'resolved' ? 'tag-ok' : ''}`}>{s}: {st === 'resolved' ? t('incidents.drawer.sourceOk') : t('incidents.drawer.sourceActive')}</span>)}</Prop>
              <Prop k={t('incidents.drawer.events')}>{inc.count}</Prop>
              <Prop k={t('incidents.drawer.opened')}>{fmtTime(inc.first_seen)}</Prop>
              <Prop k={t('incidents.drawer.lastEvent')}>{fmtTime(inc.last_seen)}</Prop>
              <Prop k={t('incidents.drawer.duration')}>{fmtDuration(inc.first_seen, inc.resolved_at)}</Prop>
              {inc.acked_by && <Prop k={t('incidents.drawer.ackedBy')}>{inc.acked_by}</Prop>}
              <Prop k={t('incidents.drawer.dedupKey')}><code>{inc.dedup_key}</code></Prop>
              {data!.related && (
                <Prop k={inc.method === 'red' ? t('incidents.drawer.probableCause') : t('incidents.drawer.affects')}>
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
                  <th>{t('common.words.time')}</th>
                  <th>{t('common.words.source')}</th>
                  <th>{t('incidents.drawer.severity')}</th>
                  <th>{t('common.words.status')}</th>
                  <th>{t('incidents.drawer.event')}</th>
                </tr>
              </thead>
              <tbody>
                {data!.events.map((e) => (
                  <tr key={e.id} title={e.raw}>
                    <td className="nowrap">{fmtTime(e.received_at)}</td>
                    <td>{e.source}</td>
                    <td><SevBadge sev={e.severity} /></td>
                    <td>{e.status === 'resolved' ? t('incidents.drawer.sourceOk') : t('incidents.drawer.sourceActive')}</td>
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
              <Prop k={t('incidents.drawer.pdState')}><PDPill state={inc.pd_state} fallback={inc.fallback} /></Prop>
              <Prop k="dedup_key"><code>{inc.pd_dedup_key}</code></Prop>
              {inc.pd_route && <Prop k={t('incidents.drawer.pdRoute')}>{inc.pd_route}</Prop>}
              {inc.pd_incident_id && <Prop k={t('incidents.drawer.pdIncident')}>{inc.pd_incident_url ? <a className="link" href={inc.pd_incident_url} target="_blank" rel="noreferrer">{inc.pd_incident_id}</a> : inc.pd_incident_id}</Prop>}
              {inc.pd_retry && <Prop k={t('incidents.drawer.pdRetry')}><code>{inc.pd_retry}</code></Prop>}
              {inc.pd_error && <Prop k={t('incidents.drawer.lastError')}><span className="text-danger">{inc.pd_error}</span></Prop>}
              <Prop k={t('incidents.drawer.fallback')}>{inc.fallback ? t('incidents.drawer.fallbackYes') : t('incidents.drawer.fallbackNo')}</Prop>
              <div className="section-title">{t('incidents.drawer.deliveries')}</div>
              <Timeline items={timeline.filter((t) => t.kind === 'pagerduty' || t.kind === 'fallback')} />
            </div>
          )}
          {tab === 'comments' && (
            <div>
              <Timeline items={timeline.filter((t) => t.kind === 'comment')} empty={t('incidents.drawer.noComments')} />
              {canAct && inc.status !== 'resolved' && (
                <form
                  className="comment-form"
                  onSubmit={(e) => {
                    e.preventDefault()
                    if (comment.trim()) act('comment', comment)
                  }}
                >
                  <textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder={t('incidents.drawer.commentPlaceholder')} rows={3} />
                  <button className="btn btn-primary" type="submit">
                    {t('incidents.drawer.addComment')}
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

// Timeline entry kinds with a label in incidents.kinds; others show as is.
const KINDS = new Set(['event', 'status', 'pagerduty', 'fallback', 'comment', 'maintenance'])

function Timeline({ items, empty }: { items: { at: string; kind: string; text: string; author?: string }[]; empty?: string }) {
  if (items.length === 0) return <Empty>{empty ?? t('incidents.drawer.noRecords')}</Empty>
  return (
    <ul className="timeline">
      {[...items].reverse().map((it, i) => (
        <li key={i} className={`tl tl-${it.kind}`}>
          <span className="tl-time">{fmtTime(it.at)}</span>
          <span className="tl-kind">{KINDS.has(it.kind) ? t(`incidents.kinds.${it.kind}`) : it.kind}</span>
          <span className="tl-text">
            {it.text}
            {it.author && <span className="muted"> · {it.author}</span>}
          </span>
        </li>
      ))}
    </ul>
  )
}
