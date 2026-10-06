import { useEffect, useState, type ReactNode } from 'react'
import { ExternalLink } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Button, formatDate, Modal, Rows, Segmented, Textarea } from '../../ui'
import { useSession } from '../session'
import { entryText, severityTone } from './format'
import { strings } from './strings'
import type { Detail, Incident, PD, Person, Severity } from './types'

export function SeverityPill({ severity }: { severity: Severity }) {
  const t = useT(strings)
  return (
    <span className={`pill inc-sev inc-sev-${severityTone(severity)}`}>
      <span className="inc-dot" aria-hidden />
      {t(`inc.sev.${severity}`)}
    </span>
  )
}

export function StatusPill({ status }: { status: Incident['status'] }) {
  const t = useT(strings)
  const tone = status === 'open' ? 'error' : status === 'acknowledged' ? 'warn' : 'ok'
  return <span className={`pill pill-${tone}`}>{t(`inc.status.${status}`)}</span>
}

// PDPill: "off" (PagerDuty is turned off) is grey and neutral, not a failure.
export function PDPill({ state }: { state: PD['state'] }) {
  const t = useT(strings)
  const tone = state === 'accepted' || state === 'acked' ? 'ok' : state === 'failed' ? 'error' : state === 'pending' ? 'warn' : 'off'
  const label = t(`inc.pd.${state}`)
  return <span className={`pill pill-${tone}`}>{label === `inc.pd.${state}` ? state : label}</span>
}

type Props = { id: string | null; actor: boolean; onClose: () => void; onChanged: () => void; onOpen: (id: string) => void }

type Tab = 'main' | 'timeline' | 'sources'

export function IncidentDetail({ id, actor, onClose, onChanged, onOpen }: Props) {
  const t = useT(strings)
  const [epoch, setEpoch] = useState(0)
  const [tab, setTab] = useState<Tab>('main')
  const detail = useResource<Detail>(id ? `/api/incidents/${encodeURIComponent(id)}` : '', epoch)
  const act = useAction()
  useEffect(() => {
    setTab('main')
    act.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])
  useEffect(() => {
    if (!id) return
    const timer = window.setInterval(() => setEpoch((e) => e + 1), 10_000)
    return () => window.clearInterval(timer)
  }, [id])
  const d = detail.data && detail.data.alert.id === id ? detail.data : null
  const a = d?.alert
  const run = (action: string, body?: unknown) =>
    act.run(async () => {
      await api('POST', `/api/incidents/${encodeURIComponent(id!)}/${action}`, body)
      setEpoch((e) => e + 1)
      onChanged()
    })

  const footer = (
    <>
      {d?.grafana_url && (
        <a className="btn btn-ghost" href={d.grafana_url} target="_blank" rel="noopener noreferrer">
          {t('inc.grafana')}
          <ExternalLink size={14} aria-hidden />
        </a>
      )}
      {a?.pd.incident_url && (
        <a className="btn btn-ghost" href={a.pd.incident_url} target="_blank" rel="noopener noreferrer">
          {t('inc.pd.open')}
          <ExternalLink size={14} aria-hidden />
        </a>
      )}
      {actor && a?.status === 'open' && (
        <Button busy={act.busy} onClick={() => void run('ack')}>
          {t('inc.ack')}
        </Button>
      )}
      {actor && a && a.status !== 'resolved' && (
        <Button variant="primary" busy={act.busy} onClick={() => void run('resolve')}>
          {t('inc.resolve')}
        </Button>
      )}
      <Button onClick={onClose}>{t('inc.close')}</Button>
    </>
  )
  return (
    <Modal open={id !== null} title={a ? `${a.id} · ${a.title}` : (id ?? '')} onClose={onClose} footer={footer}>
      {!d && detail.error ? <ErrorBanner error={detail.error} strings={strings} /> : null}
      {!d && !detail.error && <p className="muted">{t('loading')}</p>}
      {d && a && (
        <div className="inc-detail">
          <div className="row">
            <SeverityPill severity={a.severity} />
            <StatusPill status={a.status} />
            <PDPill state={a.pd.state} />
          </div>
          <Segmented
            label={t('inc.tab.main')}
            value={tab}
            onChange={setTab}
            options={[
              { value: 'main', label: t('inc.tab.main') },
              { value: 'timeline', label: `${t('inc.tab.timeline')} (${d.timeline.length})` },
              { value: 'sources', label: `${t('inc.tab.sources')} (${Object.keys(a.sources).length})` },
            ]}
          />
          {tab === 'main' && <Main a={a} onOpen={onOpen} />}
          {tab === 'timeline' && <Timeline d={d} actor={actor} busy={act.busy} onComment={(text) => run('comment', { text })} />}
          {tab === 'sources' && <Sources d={d} />}
        </div>
      )}
      <ErrorBanner error={act.error} strings={strings} />
    </Modal>
  )
}

function People({ list }: { list: Person[] }) {
  const t = useT(strings)
  if (list.length === 0) return <span className="muted">{t('inc.none')}</span>
  return (
    <ul className="ci-owners">
      {list.map((p) => (
        <li key={p.user_id + (p.role ?? '')}>
          {p.name}
          {p.role && <span className="muted"> · {p.role === 'lead' || p.role === 'member' ? t(`inc.role.${p.role}`) : p.role}</span>}
          {p.email && (
            <>
              {' · '}
              <a href={`mailto:${p.email}`}>{p.email}</a>
            </>
          )}
          {p.telegram && <span className="muted"> · Telegram</span>}
        </li>
      ))}
    </ul>
  )
}

function Main({ a, onOpen }: { a: Incident; onOpen: (id: string) => void }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const at = (v?: string) => formatDate(v, locale, timezone)
  const rows: [string, ReactNode][] = [
    [
      t('inc.field.ci'),
      <span key="ci">
        {a.ci_name}
        {!a.ci_id && <span className="inc-warn"> ({t('inc.noci')})</span>}
      </span>,
    ],
    [t('inc.field.signal'), a.signal],
    [t('inc.field.method'), t(`inc.method.${a.method}`)],
    [t('inc.field.services'), a.route.services.map((s) => s.name).join(', ')],
    [t('inc.field.team'), a.route.team?.name ?? ''],
    [t('inc.field.route'), t(`inc.via.${a.route.via}`)],
    [t('inc.field.people'), <People key="p" list={a.route.people} />],
    [t('inc.field.owners'), <People key="o" list={a.route.owners} />],
    [t('inc.field.opened'), at(a.opened_at)],
    [t('inc.field.first'), at(a.first_seen)],
    [t('inc.field.last'), `${at(a.last_seen)} · ${a.count}`],
  ]
  if (a.acked_at) rows.push([t('inc.field.acked'), `${at(a.acked_at)} · ${a.acked_by ?? ''}`])
  if (a.resolved_at) rows.push([t('inc.field.resolved'), `${at(a.resolved_at)}${a.resolved_by ? ` · ${a.resolved_by}` : ''}`])
  rows.push([t('inc.field.pd'), <PDPill key="pd" state={a.pd.state} />])
  if (a.pd.route) rows.push([t('inc.field.pdroute'), a.pd.route])
  if (a.pd.error) rows.push([t('inc.field.pderror'), <span key="e" className="inc-warn">{a.pd.error}</span>])
  if (a.fallback) rows.push([t('inc.field.fallback'), at(a.fallback_at)])
  if (a.suppressed) rows.push([t('inc.field.maintenance'), a.maintenance_id ?? ''])
  if (a.related_id)
    rows.push([
      t('inc.field.related'),
      <button key="r" type="button" className="cn-link" onClick={() => onOpen(a.related_id!)}>
        {a.related_id}
      </button>,
    ])
  const labels = Object.entries(a.labels ?? {})
  if (labels.length) rows.push([t('inc.field.labels'), <span key="l" className="cn-mono">{labels.map(([k, v]) => `${k}=${v}`).join(', ')}</span>])
  rows.push([t('inc.field.key'), <code key="k">{a.pd.key}</code>])
  return <Rows rows={rows} />
}

function Timeline({ d, actor, busy, onComment }: { d: Detail; actor: boolean; busy: boolean; onComment: (text: string) => Promise<unknown> }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const [text, setText] = useState('')
  return (
    <div className="stack">
      {actor && (
        <div className="inc-comment">
          <Textarea value={text} rows={2} placeholder={t('inc.comment.placeholder')} aria-label={t('inc.comment')} onChange={(e) => setText(e.target.value)} />
          <Button
            busy={busy}
            disabled={!text.trim()}
            onClick={() =>
              void onComment(text).then(() => {
                setText('')
              })
            }
          >
            {t('inc.comment.add')}
          </Button>
        </div>
      )}
      <ol className="inc-timeline">
        {[...d.timeline].reverse().map((e) => (
          <li key={e.id} className={`inc-tl inc-tl-${e.kind}`}>
            <span className="inc-tl-at muted">{formatDate(e.at, locale, timezone)}</span>
            <span className="inc-tl-text">
              {entryText(t, e, d.connectors)}
              {e.author && <span className="muted"> · {e.author}</span>}
            </span>
          </li>
        ))}
      </ol>
    </div>
  )
}

function Sources({ d }: { d: Detail }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const list = Object.values(d.alert.sources).sort((x, y) => y.last_seen.localeCompare(x.last_seen))
  return (
    <table className="cn-table compact">
      <tbody>
        {list.map((s) => (
          <tr key={s.connector_id + s.key}>
            <td>{d.connectors[s.connector_id] ?? s.connector_id}</td>
            <td>
              {s.title}
              {s.value && <span className="muted"> · {s.value}</span>}
            </td>
            <td>
              <SeverityPill severity={s.severity} />
            </td>
            <td>
              <span className={`pill pill-${s.status === 'firing' ? 'error' : 'ok'}`}>{t(`inc.src.${s.status}`)}</span>
            </td>
            <td className="muted">{formatDate(s.last_seen, locale, timezone)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
