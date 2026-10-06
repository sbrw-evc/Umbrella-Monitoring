import { useEffect, useState, type ReactNode } from 'react'
import { api } from '../../api'
import { useT } from '../../i18n'
import { Rows } from '../../ui'
import { strings } from './strings'
import './routing.css'

type Ref = { id: string; name: string }
type Person = { user_id: string; name: string; email?: string; telegram?: string; role?: string }

export type RoutePreviewData = {
  services: Ref[]
  service?: Ref
  team?: Ref
  people: Person[]
  owners: Person[]
  channel?: { email?: string; telegram?: string }
  via: 'service' | 'ci_owners' | 'none'
  pagerduty_route: { id: string; name: string; min_severity: string; no_key?: boolean } | null
  elsewhere?: { ci: Ref; service: Ref; team?: Ref }[]
}

/** RoutePreview shows where an incident of a CI or a service would go now (GET /api/{kind}/{id}/route). */
export function RoutePreview({ kind, id, version }: { kind: 'cis' | 'services'; id: string; version?: unknown }) {
  const t = useT(strings)
  const [data, setData] = useState<RoutePreviewData | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let live = true
    setFailed(false)
    api<RoutePreviewData>('GET', `/api/${kind}/${encodeURIComponent(id)}/route`)
      .then((d) => live && setData(d))
      .catch(() => live && setFailed(true))
    return () => {
      live = false
    }
  }, [kind, id, version])

  let body: ReactNode
  if (failed) body = <p className="muted">{t('rt.error')}</p>
  else if (!data) body = <p className="muted">{t('rt.loading')}</p>
  else body = <RouteRows data={data} />
  return (
    <section className="rt-preview" aria-label={t('rt.title')}>
      <div className="section-title">{t('rt.title')}</div>
      {body}
      <p className="muted rt-rule">{t('rt.rule')}</p>
    </section>
  )
}

function names(ps: Person[], lead: string) {
  return ps.map((p) => (p.role === 'lead' ? `${p.name} (${lead})` : p.name)).join(', ')
}

function RouteRows({ data }: { data: RoutePreviewData }) {
  const t = useT(strings)
  const others = data.services.filter((s) => s.id !== data.service?.id)
  const channel = [data.channel?.email, data.channel?.telegram].filter(Boolean).join(', ')
  let receives: ReactNode
  if (data.via === 'service') {
    receives = (
      <span>
        {data.people.length > 0 && t('rt.people.n', { n: data.people.length, list: names(data.people, t('rt.lead')) })}
        {data.people.length > 0 && channel && ' · '}
        {channel && t('rt.channel', { list: channel })}
      </span>
    )
  } else if (data.via === 'ci_owners') {
    receives = t('rt.people.owners', { n: data.owners.length, list: names(data.owners, t('rt.lead')) })
  } else {
    receives = <span className="rt-warn">{t('rt.people.none')}</span>
  }
  let pd: ReactNode = <span className="muted">{t('rt.pd.off')}</span>
  if (data.pagerduty_route) {
    const r = data.pagerduty_route
    const name = r.id === 'default' ? t(r.no_key ? 'rt.pd.nokey' : 'rt.pd.default') : r.name
    pd = (
      <span>
        {name}
        {r.min_severity && <span className="muted"> · {t('rt.pd.min', { sev: r.min_severity })}</span>}
      </span>
    )
  }
  const rows: [ReactNode, ReactNode][] = [
    [
      t('rt.service'),
      data.service ? (
        <span>
          <strong>{data.service.name}</strong>
          {data.services.length > 1 && <span className="muted"> · {t('rt.service.primary')}</span>}
          {others.length > 0 && <span className="muted"> · {t('rt.service.others', { list: others.map((s) => s.name).join(', ') })}</span>}
        </span>
      ) : (
        <span className="rt-warn">{t('rt.service.none')}</span>
      ),
    ],
    [t('rt.team'), data.team ? data.team.name : <span className="muted">{t('rt.team.none')}</span>],
    [t('rt.people'), receives],
    [t('rt.pd'), pd],
  ]
  return (
    <>
      <Rows rows={rows} />
      {data.elsewhere && data.elsewhere.length > 0 && (
        <div className="rt-elsewhere">
          <p className="muted">{t('rt.elsewhere')}</p>
          <ul>
            {data.elsewhere.map((e) => (
              <li key={e.ci.id}>{t('rt.elsewhere.row', { ci: e.ci.name, service: e.service.name, team: e.team?.name ?? t('rt.elsewhere.noteam') })}</li>
            ))}
          </ul>
        </div>
      )}
    </>
  )
}
