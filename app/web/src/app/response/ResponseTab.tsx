import { ExternalLink } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner, ErrorFlash } from '../../connections/ConnectionCard'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Banner, Button, formatDate } from '../../ui'
import { AssessmentView } from './parts'
import { responseStrings } from './strings'
import type { JiraIssue, ResponseState } from './types'
import './response.css'

type View = { mode: 'off' | 'dry_run' | 'live'; state: ResponseState | null }

// ResponseTab is the response of one incident: its assessment, the escalation so far, the war
// room, the call and the Jira issues, with the actions people may take by hand.
export function ResponseTab({ id, status, actor, epoch }: { id: string; status: string; actor: boolean; epoch: number }) {
  const t = useT(responseStrings)
  const { locale } = useLocale()
  const res = useResource<View>(`/api/incidents/${encodeURIComponent(id)}/response`, epoch)
  const act = useAction()
  const run = (action: string) =>
    act.run(async () => {
      await api('POST', `/api/incidents/${encodeURIComponent(id)}/response/${action}`)
      await res.reload()
    })
  if (res.error) return <ErrorBanner error={res.error} strings={responseStrings} />
  const v = res.data
  if (!v) return <p className="muted">{t('loading')}</p>
  if (v.mode === 'off') return <Banner kind="info" title={t('ir.off')} />
  const st = v.state
  const active = status !== 'resolved'
  const link = (label: string, url?: string, dry?: boolean, extra?: string) => (
    <div className="rs-link">
      <span className="hint">{label}</span>
      {url ? (
        <a href={url} target="_blank" rel="noopener noreferrer">
          {extra ?? t('ir.open')} <ExternalLink size={13} aria-hidden />
        </a>
      ) : (
        <span>{extra ?? '—'}</span>
      )}
      {dry && <span className="pill pill-off">{t('ir.dry_tag')}</span>}
    </div>
  )
  const issue = (label: string, j?: JiraIssue) => j && link(label, j.url, j.dry_run, j.key)
  return (
    <div className="rs-incident">
      {v.mode === 'dry_run' && <Banner kind="info" title={t('ir.dry')} />}
      {!st && <p className="muted">{t('ir.none')}</p>}
      {st && (
        <>
          <AssessmentView as={{ ...st.assessment, priority: st.priority }} />
          <div className="rs-links">
            {st.room && link(t('ir.room'), st.room.url, st.room.dry_run, st.room.url ? undefined : t('ir.members', { n: st.room.members.length }))}
            {st.bridge && link(`${t('ir.bridge')} · ${st.bridge.provider === 'zoom' ? 'Zoom' : 'Teams'}`, st.bridge.url, st.bridge.dry_run)}
            {issue(t('ir.task'), st.task)}
            {issue(t('ir.pm'), st.postmortem)}
          </div>
          {st.steps.length > 0 && (
            <>
              <h3 className="rs-h">{t('ir.steps')}</h3>
              <ol className="rs-ir-steps">
                {st.steps.map((s) => (
                  <li key={`${s.index}-${s.at}`}>
                    <b>{t('ir.step', { n: s.index + 1, at: formatDate(s.at, locale) })}</b>
                    {s.dry_run && <span className="pill pill-off">{t('ir.dry_tag')}</span>}
                    {s.people && s.people.length > 0 && <div>{t('ir.step.people', { people: s.people.join(', ') })}</div>}
                    {s.reached.length > 0 && <div className="hint">{t('ir.step.reached', { list: s.reached.map((r) => (r.startsWith('call_') || r === 'war_room' ? t(`method.${r}`) : r)).join(', ') })}</div>}
                    {s.failed && s.failed.length > 0 && <div className="rs-failed">{t('ir.step.failed', { list: s.failed.join('; ') })}</div>}
                  </li>
                ))}
              </ol>
            </>
          )}
          {st.failures &&
            Object.entries(st.failures).map(([k, f]) => <Banner key={k} kind="warn" title={t('ir.failure', { action: t(`ir.do.${k}`), error: f.error, n: f.attempts })} />)}
        </>
      )}
      {actor && (
        <div className="row inc-actions">
          {active && !st?.room && <Button busy={act.busy} onClick={() => void run('room')}>{t('ir.do.room')}</Button>}
          {active && !st?.bridge && <Button busy={act.busy} onClick={() => void run('bridge')}>{t('ir.do.bridge')}</Button>}
          {!st?.task && <Button busy={act.busy} onClick={() => void run('task')}>{t('ir.do.task')}</Button>}
          {!active && !st?.postmortem && <Button busy={act.busy} onClick={() => void run('postmortem')}>{t('ir.do.postmortem')}</Button>}
          <Button variant="ghost" busy={act.busy} onClick={() => void run('assess')}>{t('ir.do.assess')}</Button>
        </div>
      )}
      <ErrorFlash error={act.error} strings={responseStrings} />
    </div>
  )
}
