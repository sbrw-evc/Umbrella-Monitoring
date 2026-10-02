import { useState } from 'react'
import { ciTypeLabel, fmtTime, type CI, type Incident, type Maintenance, type Relation, type Severity } from '../api'
import { useFetch, useLive } from '../context'
import { t } from '../i18n'
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

export function CiDrawer({ id, onClose, onOpenCi, onOpenIncident }: { id: string; onClose: () => void; onOpenCi: (id: string) => void; onOpenIncident: (id: string) => void }) {
  const { data, reload } = useFetch<CiDetail>(`/api/cis/${id}`)
  const [tab, setTab] = useState<Tab>('params')
  useLive(['alert'], reload, 1000)
  const ci = data?.ci
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
              <div className="prop"><div className="prop-k">{t('cmdb.drawer.origin')}</div><div className="prop-v">{ci.origin === 'discovery' ? t('cmdb.drawer.originDiscovery') : t('cmdb.drawer.originManual')}</div></div>
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
            <table className="table table-compact">
              <thead><tr><th /><th>{t('cmdb.relations.ci')}</th><th>{t('cmdb.relations.type')}</th><th>{t('cmdb.relations.relation')}</th></tr></thead>
              <tbody>
                {data.relations.map((r, n) => (
                  <tr key={n} onClick={() => onOpenCi(r.other)}>
                    <td><SevDot sev={r.status} /></td>
                    <td>{r.name}</td>
                    <td>{ciTypeLabel(r.ci_type)}</td>
                    <td>{r.dir === 'up' ? `← ${r.type === 'runs_on' ? t('cmdb.relations.runsFor') : t('cmdb.relations.neededFor')}` : `→ ${r.type === 'runs_on' ? t('cmdb.relations.runsOn') : t('cmdb.relations.dependsOn')}`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
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
