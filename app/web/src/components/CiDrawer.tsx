import { useState } from 'react'
import { CI_TYPE_LABEL, fmtTime, type CI, type Incident, type Maintenance, type Relation, type Severity } from '../api'
import { useFetch, useLive } from '../context'
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

export const MAINT_STATE: Record<string, string> = { planned: 'Запланировано', active: 'В действии', finished: 'Завершено' }

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
      sub={ci && <>{ci.id} · {CI_TYPE_LABEL[ci.type] ?? ci.type}</>}
    >
      {!data || !ci ? (
        <Empty>Загрузка…</Empty>
      ) : (
        <>
          <Tabs<Tab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: 'params', title: 'Параметры' },
              { id: 'ids', title: `Идентификаторы · ${ci.identities.length}` },
              { id: 'rels', title: `Связи · ${data.relations.length}` },
              { id: 'alerts', title: `Инциденты · ${data.alerts.filter((a) => a.status !== 'resolved').length}` },
              { id: 'maint', title: 'Обслуживание' },
            ]}
          />
          {tab === 'params' && (
            <div className="props">
              <div className="prop"><div className="prop-k">Тип</div><div className="prop-v">{CI_TYPE_LABEL[ci.type] ?? ci.type}</div></div>
              <div className="prop"><div className="prop-k">Состояние</div><div className="prop-v"><SevBadge sev={ci.status} /> {ci.status !== ci.own_status && <span className="muted">с учётом зависимых КЕ</span>}</div></div>
              <div className="prop"><div className="prop-k">Описание</div><div className="prop-v">{ci.description || '—'}</div></div>
              <div className="prop"><div className="prop-k">Команда</div><div className="prop-v">{ci.team || '—'}</div></div>
              <div className="prop"><div className="prop-k">ИТ-сервис</div><div className="prop-v">{data.service || '—'}</div></div>
              {ci.logical_group && <div className="prop"><div className="prop-k">Логическая группа</div><div className="prop-v">{ci.logical_group}</div></div>}
              <div className="prop"><div className="prop-k">Происхождение</div><div className="prop-v">{ci.origin === 'discovery' ? 'CMDB Discovery' : 'вручную'}</div></div>
              <div className="prop"><div className="prop-k">Создана</div><div className="prop-v">{fmtTime(ci.created_at)}</div></div>
              {ci.maintenance && <div className="prop"><div className="prop-k">Обслуживание</div><div className="prop-v"><span className="pill pill-muted">идёт окно обслуживания</span></div></div>}
            </div>
          )}
          {tab === 'ids' && (
            <>
              <p className="hint">Источники называют КЕ по-разному. ID облачного инстанса меняется после перезапуска, поэтому он хранится как история, а КЕ остаётся той же.</p>
              <table className="table table-compact">
                <thead><tr><th>Вид</th><th>Значение</th><th>С</th><th>По</th></tr></thead>
                <tbody>
                  {ci.identities.map((i, n) => (
                    <tr key={n} className={i.until ? 'row-resolved' : ''}>
                      <td>{i.kind}</td>
                      <td className="mono">{i.value}</td>
                      <td>{fmtTime(i.since)}</td>
                      <td>{i.until ? fmtTime(i.until) : 'действует'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
          {tab === 'rels' && (
            <table className="table table-compact">
              <thead><tr><th /><th>КЕ</th><th>Тип</th><th>Связь</th></tr></thead>
              <tbody>
                {data.relations.map((r, n) => (
                  <tr key={n} onClick={() => onOpenCi(r.other)}>
                    <td><SevDot sev={r.status} /></td>
                    <td>{r.name}</td>
                    <td>{CI_TYPE_LABEL[r.ci_type] ?? r.ci_type}</td>
                    <td>{r.dir === 'up' ? `← ${r.type === 'runs_on' ? 'работает для' : 'нужна для'}` : `→ ${r.type === 'runs_on' ? 'работает на' : 'зависит от'}`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {tab === 'alerts' && <IncidentTable items={data.alerts} onOpen={onOpenIncident} compact />}
          {tab === 'maint' &&
            (data.maintenance.length === 0 ? (
              <Empty>Окон обслуживания нет</Empty>
            ) : (
              <table className="table table-compact">
                <thead><tr><th>Окно</th><th>Состояние</th><th>Начало</th><th>Конец</th></tr></thead>
                <tbody>
                  {data.maintenance.map(({ maintenance: m, state }) => (
                    <tr key={m.id}><td>{m.title}</td><td>{MAINT_STATE[state]}</td><td>{fmtTime(m.start)}</td><td>{fmtTime(m.end)}</td></tr>
                  ))}
                </tbody>
              </table>
            ))}
        </>
      )}
    </Drawer>
  )
}
