import { ExternalLink, Pencil, Plus, X } from 'lucide-react'
import { useState } from 'react'
import { api, ciTypeLabel, fmtTime, type CI, type Incident, type Maintenance, type Relation, type Severity } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { OwnerList, originLabel } from '../pages/Cis'
import { CiIcon } from './CmdbGraph'
import { IncidentTable } from './IncidentTable'
import { Drawer, Empty, SevBadge, SevDot, Tabs } from './ui'

export interface CiDetail {
  ci: CI
  relations: (Relation & { dir: 'up' | 'down'; other: string; name: string; ci_type: string; status: Severity | '' })[]
  alerts: Incident[]
  maintenance: { maintenance: Maintenance; state: string }[]
  service: string
}

type Tab = 'params' | 'ids' | 'rels' | 'alerts' | 'maint'

export function maintStateLabel(state: string): string {
  const key = `cmdb.maintenanceState.${state}`
  const v = t(key)
  return v === key ? state : v
}

// Kept for existing call sites (MAINT_STATE[state]); labels are read at access time.
export const MAINT_STATE: Record<string, string> = new Proxy({} as Record<string, string>, {
  get: (_, state) => (typeof state === 'string' ? maintStateLabel(state) : undefined),
})

export function CiDrawer({
  id,
  onClose,
  onOpenCi,
  onOpenIncident,
  onEdit,
  onChanged,
}: {
  id: string
  onClose: () => void
  onOpenCi: (id: string) => void
  onOpenIncident: (id: string) => void
  onEdit?: (ci: CI) => void
  onChanged?: () => void
}) {
  const { can, toast } = useApp()
  const { data, reload } = useFetch<CiDetail>(`/api/cis/${id}`)
  const all = useFetch<{ items: CI[] }>(can('cmdb.edit') ? '/api/cis' : null)
  const [tab, setTab] = useState<Tab>('params')
  const [relTo, setRelTo] = useState('')
  const [relType, setRelType] = useState('depends_on')
  const [relDir, setRelDir] = useState<'down' | 'up'>('down')
  useLive(['alert'], reload, 1000)
  const ci = data?.ci
  const addRel = async () => {
    const body = relDir === 'down' ? { from: id, to: relTo, type: relType } : { from: relTo, to: id, type: relType }
    try {
      await api.post('/api/relations', body)
      setRelTo('')
      reload()
      onChanged?.()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const delRel = async (from: string, to: string) => {
    await api.del(`/api/relations?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`)
    reload()
    onChanged?.()
  }
  return (
    <Drawer
      open
      onClose={onClose}
      wide
      title={
        <span className="drawer-title-row">
          {ci && <CiIcon type={ci.type} size={18} />}
          <span>{ci?.name ?? id}</span>
          {ci && <SevBadge sev={ci.status} />}
        </span>
      }
      sub={ci && <>{ci.id} · {ciTypeLabel(ci.type)}</>}
      actions={
        ci && (onEdit || ci.external_url) ? (
          <>
            {onEdit && can('cmdb.edit') && (
              <button className="btn" onClick={() => onEdit(ci)}>
                <Pencil size={14} /> {t('common.actions.edit')}
              </button>
            )}
            {ci.external_url && (
              <a className="btn" href={ci.external_url} target="_blank" rel="noreferrer">
                <ExternalLink size={14} /> {originLabel(ci.origin)}
              </a>
            )}
          </>
        ) : undefined
      }
    >
      {!data || !ci ? (
        <Empty>{t('common.words.loading')}</Empty>
      ) : (
        <>
          <Tabs<Tab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: 'params', title: t('cmdb.tabs.params') },
              { id: 'ids', title: t('cmdb.tabs.ids', { n: ci.identities.length }) },
              { id: 'rels', title: t('cmdb.tabs.rels', { n: data.relations.length }) },
              { id: 'alerts', title: t('cmdb.tabs.alerts', { n: data.alerts.filter((a) => a.status !== 'resolved').length }) },
              { id: 'maint', title: t('cmdb.tabs.maint') },
            ]}
          />
          {tab === 'params' && (
            <div className="props">
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.type')}</div><div className="prop-v">{ciTypeLabel(ci.type)}</div></div>
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.state')}</div><div className="prop-v"><SevBadge sev={ci.status} /> {ci.status !== ci.own_status && <span className="muted">{t('cmdb.drawer.withDependents')}</span>}</div></div>
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.description')}</div><div className="prop-v">{ci.description || '—'}</div></div>
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.team')}</div><div className="prop-v">{ci.team || '—'}</div></div>
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.itService')}</div><div className="prop-v">{data.service || '—'}</div></div>
              {ci.logical_group && <div className="prop"><div className="prop-k">{t('cmdb.drawer.logicalGroup')}</div><div className="prop-v">{ci.logical_group}</div></div>}
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.owners')}</div><div className="prop-v"><OwnerList owners={ci.owners} /></div></div>
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.origin')}</div><div className="prop-v">{originLabel(ci.origin)}</div></div>
              {ci.labels && Object.keys(ci.labels).length > 0 && (
                <div className="prop"><div className="prop-k">{t('cmdb.drawer.labels')}</div><div className="prop-v"><div className="tags">{Object.entries(ci.labels).map(([k, v]) => <span key={k} className="tag mono">{k}={v}</span>)}</div></div></div>
              )}
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.created')}</div><div className="prop-v">{fmtTime(ci.created_at)}</div></div>
              {ci.maintenance && <div className="prop"><div className="prop-k">{t('cmdb.drawer.maintenance')}</div><div className="prop-v"><span className="pill pill-muted">{t('cmdb.drawer.maintenanceActive')}</span></div></div>}
            </div>
          )}
          {tab === 'ids' && (
            <>
              <p className="hint">{t('cmdb.drawer.idsHint')}</p>
              <table className="table table-compact">
                <thead><tr><th>{t('cmdb.drawer.idKind')}</th><th>{t('cmdb.drawer.idValue')}</th><th>{t('cmdb.drawer.idSince')}</th><th>{t('cmdb.drawer.idUntil')}</th></tr></thead>
                <tbody>
                  {ci.identities.map((i, n) => (
                    <tr key={n} className={i.until ? 'row-resolved' : ''}>
                      <td>{i.kind}</td>
                      <td className="mono">{i.value}</td>
                      <td>{fmtTime(i.since)}</td>
                      <td>{i.until ? fmtTime(i.until) : t('cmdb.drawer.idActive')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
          {tab === 'rels' && (
            <>
              <table className="table table-compact">
                <thead><tr><th /><th>{t('cmdb.relations.ci')}</th><th>{t('cmdb.relations.type')}</th><th>{t('cmdb.relations.relation')}</th>{can('cmdb.edit') && <th />}</tr></thead>
                <tbody>
                  {data.relations.map((r, n) => (
                    <tr key={n} className="row-click" onClick={() => onOpenCi(r.other)}>
                      <td><SevDot sev={r.status} /></td>
                      <td>{r.name}</td>
                      <td>{ciTypeLabel(r.ci_type)}</td>
                      <td>{r.dir === 'up' ? `← ${r.type === 'runs_on' ? t('cmdb.relations.runsFor') : t('cmdb.relations.neededFor')}` : `→ ${r.type === 'runs_on' ? t('cmdb.relations.runsOn') : t('cmdb.relations.dependsOn')}`}</td>
                      {can('cmdb.edit') && (
                        <td onClick={(e) => e.stopPropagation()}>
                          <button className="icon-btn icon-btn-sm" title={t('cmdb.relations.remove')} onClick={() => delRel(r.from, r.to)}>
                            <X size={14} />
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
              {can('cmdb.edit') && (
                <div className="section">
                  <h4>{t('cmdb.relations.add')}</h4>
                  <div className="row3">
                    <select value={relDir} onChange={(e) => setRelDir(e.target.value as 'down' | 'up')}>
                      <option value="down">{t('cmdb.relations.dirDown')}</option>
                      <option value="up">{t('cmdb.relations.dirUp')}</option>
                    </select>
                    <select value={relType} onChange={(e) => setRelType(e.target.value)}>
                      <option value="depends_on">depends_on</option>
                      <option value="runs_on">runs_on</option>
                      <option value="part_of">part_of</option>
                    </select>
                    <select value={relTo} onChange={(e) => setRelTo(e.target.value)}>
                      <option value="">{t('cmdb.relations.pick')}</option>
                      {(all.data?.items ?? []).filter((c) => c.id !== id).sort((a, b) => a.name.localeCompare(b.name)).map((c) => (
                        <option key={c.id} value={c.id}>{c.name} ({ciTypeLabel(c.type)})</option>
                      ))}
                    </select>
                  </div>
                  <button className="btn btn-sm" disabled={!relTo} onClick={addRel}><Plus size={13} /> {t('cmdb.relations.add')}</button>
                </div>
              )}
            </>
          )}
          {tab === 'alerts' && <IncidentTable items={data.alerts} onOpen={onOpenIncident} compact />}
          {tab === 'maint' &&
            (data.maintenance.length === 0 ? (
              <Empty>{t('cmdb.drawer.maintEmpty')}</Empty>
            ) : (
              <table className="table table-compact">
                <thead><tr><th>{t('cmdb.drawer.maintWindow')}</th><th>{t('cmdb.drawer.maintState')}</th><th>{t('cmdb.drawer.maintStart')}</th><th>{t('cmdb.drawer.maintEnd')}</th></tr></thead>
                <tbody>
                  {data.maintenance.map(({ maintenance: m, state }) => (
                    <tr key={m.id}><td>{m.title}</td><td>{maintStateLabel(state)}</td><td>{fmtTime(m.start)}</td><td>{fmtTime(m.end)}</td></tr>
                  ))}
                </tbody>
              </table>
            ))}
        </>
      )}
    </Drawer>
  )
}
