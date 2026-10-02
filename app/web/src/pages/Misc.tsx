import { Pause, Play, Plus, Search, Trash2 } from 'lucide-react'
import { Fragment, type ReactNode, useState } from 'react'
import { api, fmtTime, METHOD_LABEL, qs, type CI, type EventItem, type Maintenance, type ParseError, type Rule } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { MAINT_STATE } from '../components/CiDrawer'
import { Empty, Field, Modal, PageHeader, SevBadge } from '../components/ui'

export function EventsPage() {
  const [q, setQ] = useState('')
  const [paused, setPaused] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  const { data, reload } = useFetch<{ items: EventItem[] }>(`/api/events${qs({ q, limit: 300 })}`)
  useLive(['event'], () => !paused && reload(), 700)
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title="События"
          sub="Нормализованные события всех коннекторов, новые сверху"
          actions={
            <button className="btn" onClick={() => setPaused((p) => !p)}>
              {paused ? <Play size={14} /> : <Pause size={14} />} {paused ? 'Продолжить' : 'Пауза'}
            </button>
          }
        />
        <div className="filterbar">
          <div className="search">
            <Search size={15} />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Заголовок, КЕ, сигнал, источник" />
          </div>
        </div>
        <div className="card card-flush">
          <table className="table table-compact">
            <thead>
              <tr>
                <th>Время</th>
                <th>Severity</th>
                <th>Статус</th>
                <th>Событие</th>
                <th>КЕ</th>
                <th>Сигнал</th>
                <th>Источник</th>
                <th>Инцидент</th>
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map((e) => (
                <Fragment key={e.id}>
                  <tr onClick={() => setOpen(open === e.id ? null : e.id)}>
                    <td className="nowrap">{fmtTime(e.received_at)}</td>
                    <td>
                      <SevBadge sev={e.severity} />
                    </td>
                    <td>{e.status === 'resolved' ? 'норма' : 'активно'}</td>
                    <td>
                      {e.title}
                      {e.suppressed && <span className="tag">подавлено</span>}
                    </td>
                    <td className="nowrap">
                      {e.ci_name || '—'}
                      {!e.ci_id && <span className="tag tag-warn">без КЕ</span>}
                    </td>
                    <td className="mono">
                      {e.signal} <span className="tag">{METHOD_LABEL[e.method]}</span>
                    </td>
                    <td>{e.source}</td>
                    <td className="mono">{e.alert_id ? <a href={`/incidents?id=${e.alert_id}&view=all`}>{e.alert_id}</a> : '—'}</td>
                  </tr>
                  {open === e.id && (
                    <tr className="row-detail">
                      <td colSpan={8}>
                        <pre className="json">{e.raw}</pre>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

export function ParseErrorsPage() {
  const { data, reload } = useFetch<{ items: ParseError[] }>('/api/parse-errors')
  useLive(['parse_error'], reload)
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title="Ошибки разбора" sub="События, которые коннектор не смог разобрать (events.dlq), с исходным телом" />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>Ошибок нет</Empty>
          ) : (
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>Время</th>
                  <th>Коннектор</th>
                  <th>Блок</th>
                  <th>Ошибка</th>
                  <th>Исходные данные</th>
                </tr>
              </thead>
              <tbody>
                {items.map((p) => (
                  <tr key={p.id}>
                    <td className="nowrap">{fmtTime(p.at)}</td>
                    <td>
                      <a href={`/connectors/${p.connector_id}`}>{p.connector}</a>
                    </td>
                    <td className="mono">{p.block}</td>
                    <td className="text-danger">{p.error}</td>
                    <td>
                      <div className="raw">{p.raw || '—'}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

export function RulesPage() {
  const { data } = useFetch<{ items: Rule[] }>('/api/rules')
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title="Правила RED/USE" sub="RED формирует тревоги по ИТ-сервисам, USE — по ресурсам. В этом шаге правила только просматриваются; редактор правил — следующий шаг" />
        <div className="card card-flush">
          <table className="table">
            <thead>
              <tr>
                <th>Метод</th>
                <th>Сигнал</th>
                <th>Правило</th>
                <th>Условие по умолчанию</th>
                <th>Применяется к</th>
                <th>Severity</th>
                <th>Включено</th>
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map((r) => (
                <tr key={r.id}>
                  <td>
                    <span className={`tag tag-${r.method}`}>{METHOD_LABEL[r.method]}</span>
                  </td>
                  <td className="mono">{r.signal}</td>
                  <td>{r.name}</td>
                  <td className="mono">{r.condition}</td>
                  <td>{r.applies_to}</td>
                  <td>
                    <SevBadge sev={r.severity} />
                  </td>
                  <td>{r.enabled ? 'да' : 'нет'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function localInput(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

export function MaintenancePage() {
  const { toast } = useApp()
  const { data, reload } = useFetch<{ items: { maintenance: Maintenance; state: string }[] }>('/api/maintenance')
  const cis = useFetch<{ items: CI[] }>('/api/cis')
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState(() => ({ title: '', ci_id: '', start: localInput(new Date()), end: localInput(new Date(Date.now() + 3600e3)) }))

  const submit = async () => {
    try {
      await api.post('/api/maintenance', { ...form, start: new Date(form.start).toISOString(), end: new Date(form.end).toISOString() })
      setCreating(false)
      reload()
      toast('Окно обслуживания создано')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const remove = async (id: string) => {
    if (!window.confirm('Удалить окно обслуживания?')) return
    await api.del(`/api/maintenance/${id}`)
    reload()
  }
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title="Окна обслуживания"
          sub="Во время окна тревоги по КЕ и её сервису не отправляются в PagerDuty; после окончания активные тревоги уходят дежурным"
          actions={
            <button className="btn btn-primary" onClick={() => setCreating(true)}>
              <Plus size={14} /> Создать окно
            </button>
          }
        />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>Окон нет</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Окно</th>
                  <th>КЕ</th>
                  <th>Состояние</th>
                  <th>Начало</th>
                  <th>Конец</th>
                  <th>Автор</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map(({ maintenance: m, state }) => (
                  <tr key={m.id}>
                    <td className="mono nowrap">{m.id}</td>
                    <td>{m.title}</td>
                    <td>{m.ci_name}</td>
                    <td>
                      <span className={`pill ${state === 'active' ? 'pill-acknowledged' : state === 'planned' ? 'pill-open' : 'pill-muted'}`}>{MAINT_STATE[state]}</span>
                    </td>
                    <td className="nowrap">{fmtTime(m.start)}</td>
                    <td className="nowrap">{fmtTime(m.end)}</td>
                    <td>{m.author}</td>
                    <td>
                      <button className="icon-btn" onClick={() => remove(m.id)}>
                        <Trash2 size={15} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {creating && (
        <Modal
          title="Новое окно обслуживания"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button className="btn" onClick={() => setCreating(false)}>
                Отмена
              </button>
              <button className="btn btn-primary" disabled={!form.title || !form.ci_id} onClick={submit}>
                Создать
              </button>
            </>
          }
        >
          <Field label="Название">
            <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder="Обновление ПО" />
          </Field>
          <Field label="КЕ" help="Для ИТ-сервиса окно подавляет тревоги всех его КЕ">
            <select value={form.ci_id} onChange={(e) => setForm({ ...form, ci_id: e.target.value })}>
              <option value="">Выберите КЕ</option>
              {(cis.data?.items ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          <div className="row2">
            <Field label="Начало">
              <input type="datetime-local" value={form.start} onChange={(e) => setForm({ ...form, start: e.target.value })} />
            </Field>
            <Field label="Конец">
              <input type="datetime-local" value={form.end} onChange={(e) => setForm({ ...form, end: e.target.value })} />
            </Field>
          </div>
        </Modal>
      )}
    </div>
  )
}

interface SelfCheck {
  uptime_s: number
  events: number
  alerts: number
  active_alerts: number
  parse_errors: number
  connectors: number
  connectors_running: number
  last_event_at?: string
  store: string
  bus: string
  pagerduty: {
    mode: string
    breaker_open: boolean
    simulated_outage: boolean
    consecutive_failures: number
    sent: number
    failed: number
    queue: number
    last_success_at?: string
    last_error?: string
  }
}

export function SelfCheckPage() {
  const { toast } = useApp()
  const { data, reload } = useFetch<SelfCheck>('/api/selfcheck')
  useLive(['alert', 'event'], reload, 2000)
  if (!data) return <Empty>Загрузка…</Empty>
  const pd = data.pagerduty
  const outage = async (on: boolean) => {
    await api.post('/api/selfcheck/pd-outage', { on })
    toast(on ? 'Симуляция: PagerDuty недоступен' : 'PagerDuty снова принимает события')
    reload()
  }
  const tile = (title: string, value: ReactNode, sub?: string, cls = '') => (
    <div className={`stat ${cls}`}>
      <div className="stat-title">{title}</div>
      <div className="stat-value">{value}</div>
      {sub && <div className="stat-sub">{sub}</div>}
    </div>
  )
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title="Самоконтроль" sub="Состояние конвейера: приём, обработка, передача в PagerDuty" />
        <div className="stats">
          {tile('Работает', `${Math.floor(data.uptime_s / 60)} мин`, `хранилище: ${data.store}, шина: ${data.bus}`)}
          {tile('Коннекторы', `${data.connectors_running} / ${data.connectors}`, 'запущено / всего')}
          {tile('События', data.events, `последнее: ${fmtTime(data.last_event_at)}`)}
          {tile('Ошибки разбора', data.parse_errors, undefined, data.parse_errors ? 'stat-warn' : '')}
          {tile('Тревоги', `${data.active_alerts} / ${data.alerts}`, 'активные / всего')}
        </div>
        <div className="card">
          <h3>PagerDuty Gateway</h3>
          <div className="stats">
            {tile('Режим', pd.mode === 'live' ? 'Events API v2' : 'dry-run', pd.mode === 'live' ? 'отправка в PagerDuty' : 'ключ интеграции не задан: события не уходят наружу')}
            {tile('Circuit breaker', pd.breaker_open ? 'открыт' : 'закрыт', `ошибок подряд: ${pd.consecutive_failures}`, pd.breaker_open ? 'stat-bad' : '')}
            {tile('Отправлено', pd.sent, `ошибок: ${pd.failed}, в очереди: ${pd.queue}`)}
            {tile('Последний успех', fmtTime(pd.last_success_at), pd.last_error ? `ошибка: ${pd.last_error}` : undefined)}
          </div>
          <div className="outage">
            <div>
              <b>Проверка резервного оповещения.</b> Симуляция недоступности PagerDuty: отправки начнут падать, откроется circuit breaker, а тревоги error и critical, не принятые за 2 минуты, получат отметку «Резерв».
            </div>
            {pd.simulated_outage ? (
              <button className="btn btn-primary" onClick={() => outage(false)}>
                Вернуть PagerDuty
              </button>
            ) : (
              <button className="btn btn-danger" onClick={() => outage(true)}>
                Симулировать недоступность
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export function AuditPage() {
  const { data } = useFetch<{ items: { at: string; actor: string; action: string; object: string }[] }>('/api/audit')
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title="Журнал аудита" sub="Все изменения конфигурации и действия с инцидентами" />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>Записей пока нет</Empty>
          ) : (
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>Время</th>
                  <th>Кто</th>
                  <th>Действие</th>
                  <th>Объект</th>
                </tr>
              </thead>
              <tbody>
                {items.map((a, i) => (
                  <tr key={i}>
                    <td className="nowrap">{fmtTime(a.at)}</td>
                    <td>{a.actor}</td>
                    <td className="mono">{a.action}</td>
                    <td className="mono">{a.object}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

const ROLES = [
  { role: 'Наблюдатель', perms: 'просмотр дашборда и карты в своей области' },
  { role: 'Дежурный инженер', perms: 'incident.ack, incident.silence, комментарии' },
  { role: 'Инженер мониторинга', perms: 'коннекторы, шаблоны, правила RED/USE, окна обслуживания' },
  { role: 'Владелец сервиса', perms: 'РСМ своих сервисов, резервные контакты, окна обслуживания' },
  { role: 'Аудитор', perms: 'журнал аудита, только чтение' },
  { role: 'Администратор ролей', perms: 'привязка групп каталога к ролям и предустановкам (в два ключа)' },
]

export function RolesPage() {
  const { meta } = useApp()
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title="Роли и группы" sub="Группы приходят из корпоративного каталога; группа задаёт роль, область видимости и предустановку дашборда. Синхронизация с каталогом и OIDC — следующий шаг" />
        <div className="card card-flush">
          <table className="table">
            <thead>
              <tr>
                <th>Роль</th>
                <th>Разрешения</th>
              </tr>
            </thead>
            <tbody>
              {ROLES.map((r) => (
                <tr key={r.role}>
                  <td>{r.role}</td>
                  <td>{r.perms}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="card">
          <h3>Области (команды)</h3>
          <div className="tags">
            {meta?.teams.map((t) => (
              <span key={t.id} className="tag">
                {t.name} · <code>{t.id}</code>
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
