import { Copy, Download, ExternalLink, KeyRound, Pencil, Plus, RefreshCw, Send, ShieldCheck, Trash2, Wrench, X } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  api,
  ciTypeLabel,
  fmtTime,
  SEVERITIES,
  sevLabel,
  type CheckResult,
  type CI,
  type Integration,
  type IntegrationType,
  type OpenBaoStatus,
  type PDView,
  type Severity,
} from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { Empty, Field, Modal, PageHeader, StatusDot, Tabs, Toggle } from '../components/ui'

type Tab = 'sources' | 'pagerduty' | 'grafana' | 'openbao'

export function IntegrationsPage() {
  const { can, meta } = useApp()
  const admin = can('integrations.edit')
  const [tab, setTab] = useState<Tab>('sources')
  const tabs: { id: Tab; title: string }[] = [{ id: 'sources', title: t('integrations.tabs.sources') }]
  if (admin) tabs.push({ id: 'pagerduty', title: t('integrations.tabs.pagerduty') }, { id: 'grafana', title: t('integrations.tabs.grafana') }, { id: 'openbao', title: t('integrations.tabs.openbao') })
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader title={t('integrations.header.title')} sub={t('integrations.header.sub')} />
        {meta && !meta.openbao && <p className="hint hint-warn">{t('integrations.noVault')}</p>}
        {tabs.length > 1 && <Tabs<Tab> tabs={tabs} value={tab} onChange={setTab} />}
        {tab === 'sources' && <Sources />}
        {tab === 'pagerduty' && admin && <PagerDuty />}
        {tab === 'grafana' && admin && <Grafana />}
        {tab === 'openbao' && admin && <OpenBao />}
      </div>
    </div>
  )
}

function copy(text: string, toast: (s: string) => void) {
  void navigator.clipboard?.writeText(text)
  toast(t('integrations.sources.copied'))
}

function Result({ label, ok, text, at }: { label: string; ok: boolean; text?: string; at?: string }) {
  if (!text) return null
  return (
    <div className="int-line">
      <StatusDot ok={ok} />
      <span>
        <b>{label}</b> {at && <span className="muted">{fmtTime(at)}</span>} · {text}
      </span>
    </div>
  )
}

function Sources() {
  const { can, toast } = useApp()
  const edit = can('connectors.edit')
  const { data, reload } = useFetch<{ items: Integration[] }>('/api/integrations')
  const types = useFetch<{ items: IntegrationType[]; openbao: boolean }>('/api/integration-types').data
  const [picking, setPicking] = useState(false)
  const [form, setForm] = useState<{ type: IntegrationType; it?: Integration } | null>(null)
  const [busy, setBusy] = useState('')
  const [token, setToken] = useState<{ it: Integration; token: string } | null>(null)
  useLive(['event', 'alert'], reload, 3000)
  const spec = (id: string) => types?.items.find((x) => x.id === id)
  const act = async (it: Integration, action: 'check' | 'setup' | 'sync') => {
    setBusy(it.id + action)
    try {
      const r = await api.post<CheckResult>(`/api/integrations/${it.id}/${action}`)
      toast(r.message, r.ok ? 'ok' : 'error')
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setBusy('')
    }
  }
  const remove = async (it: Integration) => {
    if (!window.confirm(t('integrations.sources.confirmDelete', { name: it.name }))) return
    const teardown = it.setup_at && window.confirm(t('integrations.sources.confirmTeardown'))
    await api.del(`/api/integrations/${it.id}${teardown ? '?teardown=1' : ''}`)
    toast(t('integrations.sources.deleted'))
    reload()
  }
  const reveal = async (it: Integration) => {
    const r = await api.post<{ token: string }>(`/api/integrations/${it.id}/token`)
    setToken({ it, token: r.token })
  }
  const items = data?.items ?? []
  return (
    <>
      <div className="toolbar">
        <div className="filterbar-spacer" />
        {edit && types?.openbao && (
          <button className="btn btn-primary" onClick={() => setPicking(true)}>
            <Plus size={15} /> {t('integrations.sources.add')}
          </button>
        )}
      </div>
      {items.length === 0 ? (
        <div className="card">
          <Empty>{t('integrations.sources.empty')}</Empty>
        </div>
      ) : (
        <div className="int-grid">
          {items.map((it) => (
            <div key={it.id} className="int-card">
              <div className="int-head">
                <StatusDot ok={(it.synced_at ? it.sync_ok : true) && (it.last_check_at ? it.last_check_ok : true)} warn={!it.last_check_at && !it.synced_at} />
                <div className="int-title">
                  {it.name}
                  <div className="int-sub">
                    {it.type_title} · {it.id}
                    {it.team && ` · ${it.team}`} · {t(`integrations.sources.mode.${it.mode}`)}
                  </div>
                </div>
              </div>
              {it.url && (
                <div className="int-line">
                  <ExternalLink size={13} />
                  <a className="link mono" href={it.url} target="_blank" rel="noreferrer">
                    {it.url}
                  </a>
                </div>
              )}
              {it.connector_id && (
                <div className="int-line">
                  <Link className="link" to={`/connectors/${it.connector_id}`}>
                    {t('integrations.sources.connector', { id: it.connector_id, v: it.connector_version ?? 1 })}
                  </Link>
                  {it.connector_edited && <span className="muted">{t('integrations.sources.connectorEdited')}</span>}
                </div>
              )}
              {it.ingest_url && (
                <div className="copy-box">
                  <input readOnly value={it.ingest_url} />
                  <button className="icon-btn icon-btn-sm" onClick={() => copy(it.ingest_url!, toast)} title={t('integrations.sources.ingest')}>
                    <Copy size={14} />
                  </button>
                </div>
              )}
              {it.connector_id && (
                <div className="int-line muted">
                  {it.last_event_at
                    ? t('integrations.sources.events', { n: it.events_total, e: it.errors_total, time: fmtTime(it.last_event_at) })
                    : t('integrations.sources.noEvents')}
                </div>
              )}
              <Result label={t('integrations.sources.lastCheck')} ok={it.last_check_ok} text={it.last_check} at={it.last_check_at} />
              <Result label={t('integrations.sources.lastSetup')} ok={!!it.setup_at} text={it.setup_info} at={it.setup_at} />
              <Result label={t('integrations.sources.lastSync')} ok={it.sync_ok} text={it.sync_info} at={it.synced_at} />
              {it.snippet && (
                <details>
                  <summary className="link">{t('integrations.sources.snippet')}</summary>
                  <pre className="json">{it.snippet}</pre>
                </details>
              )}
              {edit && (
                <div className="int-actions">
                  <button className="btn btn-sm" disabled={!!busy} onClick={() => act(it, 'check')}>
                    <ShieldCheck size={13} /> {t('integrations.sources.check')}
                  </button>
                  {it.can_setup && (
                    <button className="btn btn-sm" disabled={!!busy} onClick={() => act(it, 'setup')}>
                      <Wrench size={13} /> {t('integrations.sources.setup')}
                    </button>
                  )}
                  {it.can_sync && (
                    <button className="btn btn-sm" disabled={!!busy} onClick={() => act(it, 'sync')}>
                      <Download size={13} /> {t('integrations.sources.sync')}
                    </button>
                  )}
                  {it.webhook_token_ref && (
                    <button className="btn btn-sm" onClick={() => reveal(it)}>
                      <KeyRound size={13} /> {t('integrations.sources.token')}
                    </button>
                  )}
                  <button className="btn btn-sm" onClick={() => spec(it.type) && setForm({ type: spec(it.type)!, it })}>
                    <Pencil size={13} /> {t('integrations.sources.edit')}
                  </button>
                  <button className="btn btn-sm btn-ghost text-danger" onClick={() => remove(it)}>
                    <Trash2 size={13} />
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      {picking && types && (
        <Modal title={t('integrations.sources.pickType')} onClose={() => setPicking(false)} wide>
          <div className="type-pick">
            {types.items.map((ty) => (
              <button
                key={ty.id}
                type="button"
                className="type-pick-item"
                onClick={() => {
                  setPicking(false)
                  setForm({ type: ty })
                }}
              >
                <b>{ty.title}</b>
                <span>{ty.description}</span>
              </button>
            ))}
          </div>
        </Modal>
      )}
      {form && (
        <IntegrationForm
          type={form.type}
          it={form.it}
          onClose={() => setForm(null)}
          onDone={() => {
            setForm(null)
            reload()
          }}
        />
      )}
      {token && (
        <Modal title={`${t('integrations.sources.tokenShown')}: ${token.it.name}`} onClose={() => setToken(null)}>
          <div className="copy-box">
            <input readOnly value={token.token} />
            <button className="icon-btn" onClick={() => copy(token.token, toast)}>
              <Copy size={15} />
            </button>
          </div>
          {token.it.token_header && <p className="hint">{t('integrations.sources.header', { header: token.it.token_header })}</p>}
          {token.it.snippet && <pre className="json">{token.it.snippet.replace('<токен приёма>', token.token)}</pre>}
        </Modal>
      )}
    </>
  )
}

function IntegrationForm({ type, it, onClose, onDone }: { type: IntegrationType; it?: Integration; onClose: () => void; onDone: () => void }) {
  const { toast, meta } = useApp()
  const params0: Record<string, string> = {}
  for (const f of type.params ?? []) params0[f.key] = it?.params[f.key] ?? f.default ?? ''
  const [name, setName] = useState(it?.name ?? type.title)
  const [team, setTeam] = useState(it?.team ?? '')
  const [slug, setSlug] = useState(it?.slug ?? '')
  const [url, setUrl] = useState(it?.url ?? '')
  const [auth, setAuth] = useState(it?.auth_type ?? type.auth[0])
  const [username, setUsername] = useState(it?.username ?? '')
  const [secret, setSecret] = useState('')
  const [hook, setHook] = useState('')
  const [tls, setTls] = useState(it?.tls_skip_verify ?? false)
  const [params, setParams] = useState(params0)
  const [regenerate, setRegenerate] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const connector = type.mode === 'push' || type.mode === 'pull'
  const save = async () => {
    setError('')
    setBusy(true)
    const body: Record<string, unknown> = { type: type.id, name, team, url, auth_type: auth, username, tls_skip_verify: tls, params, regenerate }
    if (connector) body.slug = slug
    if (secret) body.secret = secret
    if (hook) body.webhook_token = hook
    try {
      if (it) {
        await api.put(`/api/integrations/${it.id}`, body)
        toast(t('integrations.sources.saved'))
      } else {
        await api.post('/api/integrations', body)
        toast(t('integrations.sources.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      wide
      title={it ? t('integrations.form.editTitle', { name: it.name }) : t('integrations.form.createTitle', { type: type.title })}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={busy || !name || (type.url_required && !url)}>
            {it ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      <p className="hint">{type.description}</p>
      <div className="row3">
        <Field label={t('integrations.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <Field label={t('integrations.form.team')}>
          <input value={team} onChange={(e) => setTeam(e.target.value)} list="int-teams" />
          <datalist id="int-teams">
            {meta?.teams.map((x) => (
              <option key={x.id} value={x.id} />
            ))}
          </datalist>
        </Field>
        {connector && (
          <Field label={t('integrations.form.slug')} help={t('integrations.form.slugHelp')}>
            <input className="mono" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder={type.id} />
          </Field>
        )}
      </div>
      {type.url_label && (
        <Field label={type.url_label}>
          <input className="mono" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={type.url_placeholder} />
        </Field>
      )}
      {type.auth.length > 1 || type.auth[0] !== 'none' ? (
        <div className="row3">
          <Field label={t('integrations.form.auth')}>
            <select value={auth} onChange={(e) => setAuth(e.target.value)}>
              {type.auth.map((a) => (
                <option key={a} value={a}>
                  {t(`integrations.form.authNames.${a}`)}
                </option>
              ))}
            </select>
          </Field>
          {auth === 'basic' && (
            <Field label={t('integrations.form.username')}>
              <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
            </Field>
          )}
          {auth !== 'none' && (
            <Field label={type.secret_label}>
              <input type="password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={it?.secret_set ? t('integrations.form.secretKeep') : ''} autoComplete="new-password" />
            </Field>
          )}
        </div>
      ) : null}
      {type.url_label && (
        <label className="check">
          <input type="checkbox" checked={tls} onChange={(e) => setTls(e.target.checked)} /> {t('integrations.form.tls')}
        </label>
      )}
      {type.mode === 'push' && (
        <Field label={t('integrations.form.webhookToken')} help={t('integrations.form.webhookTokenHelp')}>
          <input type="password" value={hook} onChange={(e) => setHook(e.target.value)} placeholder={it?.webhook_token_ref ? t('integrations.form.secretKeep') : ''} autoComplete="new-password" />
        </Field>
      )}
      {(type.params ?? []).length > 0 && (
        <div className="row2">
          {(type.params ?? []).map((f) => (
            <Field key={f.key} label={f.label} help={f.help}>
              {f.type === 'select' ? (
                <select value={params[f.key]} onChange={(e) => setParams({ ...params, [f.key]: e.target.value })}>
                  {f.options?.map((o) => (
                    <option key={o}>{o}</option>
                  ))}
                </select>
              ) : f.type === 'textarea' ? (
                <textarea className="mono" rows={3} value={params[f.key]} onChange={(e) => setParams({ ...params, [f.key]: e.target.value })} />
              ) : (
                <input className="mono" value={params[f.key]} placeholder={f.placeholder} onChange={(e) => setParams({ ...params, [f.key]: e.target.value })} />
              )}
            </Field>
          ))}
        </div>
      )}
      {it && connector && it.connector_edited && (
        <label className="check">
          <input type="checkbox" checked={regenerate} onChange={(e) => setRegenerate(e.target.checked)} /> {t('integrations.form.regenerate')}
        </label>
      )}
      {error && <div className="form-error">{error}</div>}
    </Modal>
  )
}

function Section({ title, children, actions }: { title: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="card">
      <div className="card-head">
        <h3>{title}</h3>
        {actions}
      </div>
      {children}
    </div>
  )
}

function PagerDuty() {
  const { toast } = useApp()
  const { data, reload, setData } = useFetch<PDView>('/api/pagerduty')
  const cis = useFetch<{ items: CI[] }>('/api/cis?type=it_service').data?.items ?? []
  const [form, setForm] = useState<Record<string, string>>({})
  const [services, setServices] = useState<{ id: string; name: string; events_key: boolean }[] | null>(null)
  const [service, setService] = useState('')
  const [createKey, setCreateKey] = useState(false)
  const [policies, setPolicies] = useState<{ id: string; name: string }[] | null>(null)
  const [route, setRoute] = useState({ name: '', team: '', service: '', routing_key: '', service_id: '' })
  const [busy, setBusy] = useState(false)
  useLive(['alert'], reload, 5000)
  if (!data) return <Empty>{t('common.words.loading')}</Empty>
  const s = data.settings
  const st = data.status
  const call = async <T,>(f: () => Promise<T>, ok?: string) => {
    setBusy(true)
    try {
      const r = await f()
      if (ok) toast(ok)
      return r
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }
  const put = async (body: Record<string, unknown>, ok = t('integrations.pd.saved')) => {
    const v = await call(() => api.put<PDView>('/api/pagerduty', body), ok)
    if (v) {
      setData(v)
      setForm({})
    }
  }
  const save = () => {
    const body: Record<string, unknown> = { region: form.region ?? s.region, min_severity: form.min_severity ?? s.min_severity }
    for (const k of ['routing_key', 'api_token', 'webhook_secret']) if (form[k]) body[k] = form[k]
    if (form.events_url !== undefined) body.events_url = form.events_url
    if (form.api_url !== undefined) body.api_url = form.api_url
    void put(body)
  }
  const check = async () => {
    const r = await call(() => api.post<CheckResult>('/api/pagerduty/check'))
    if (r) toast(r.message, r.ok ? 'ok' : 'error')
  }
  const test = async () => {
    const r = await call(() => api.post<CheckResult>('/api/pagerduty/test-event'))
    if (r) toast(r.message, r.ok ? 'ok' : 'error')
  }
  const loadServices = async () => {
    const r = await call(() => api.get<{ items: { id: string; name: string; events_key: boolean }[] }>('/api/pagerduty/services'))
    if (r) setServices(r.items)
  }
  const useService = async () => {
    const v = await call(() => api.post<PDView>('/api/pagerduty/service-key', { service_id: service, create: createKey }), t('integrations.pd.saved'))
    if (v) setData(v)
  }
  const loadPolicies = async () => {
    const r = await call(() => api.get<{ items: { id: string; name: string }[] }>('/api/pagerduty/policies'))
    if (r) setPolicies(r.items)
  }
  const subscribe = async () => {
    const v = await call(() => api.post<PDView>('/api/pagerduty/webhook-subscription'))
    if (v) {
      setData(v)
      toast(t('integrations.pd.subscribed', { id: v.settings.webhook_subscription_id ?? '' }))
    }
  }
  const unsubscribe = async () => {
    await call(() => api.del('/api/pagerduty/webhook-subscription'))
    reload()
  }
  const addRoute = async () => {
    const v = await call(() => api.post<PDView>('/api/pagerduty/routes', { ...route, create: createKey }), t('integrations.pd.saved'))
    if (v) {
      setData(v)
      setRoute({ name: '', team: '', service: '', routing_key: '', service_id: '' })
    }
  }
  const delRoute = async (id: string) => {
    await api.del(`/api/pagerduty/routes/${id}`)
    reload()
  }
  const syncOnCall = async () => {
    const v = await call(() => api.post<PDView>('/api/pagerduty/oncall/sync'))
    if (v) setData(v)
  }
  const f = (k: string, v: string) => setForm((x) => ({ ...x, [k]: v }))
  return (
    <>
      <div className="card">
        <div className="int-head">
          <Toggle on={s.enabled} onChange={(on) => put({ enabled: on })} disabled={busy} />
          <div className="int-title">
            {t('integrations.pd.enabled')}
            <div className="int-sub">{t('integrations.pd.status', { sent: st.sent, failed: st.failed, queue: st.queue })}</div>
          </div>
          <button className="btn btn-sm" onClick={check} disabled={busy || !s.api_token_ref}>
            <ShieldCheck size={13} /> {t('integrations.pd.check')}
          </button>
          <button className="btn btn-sm" onClick={test} disabled={busy || !s.routing_key_ref} title={t('integrations.pd.testHint')}>
            <Send size={13} /> {t('integrations.pd.test')}
          </button>
        </div>
        {!s.enabled && <p className="hint hint-warn">{t('integrations.pd.disabledHint')}</p>}
        {st.last_error && <Result label="Events API" ok={false} text={st.last_error} at={st.last_error_at} />}
      </div>
      <Section title={t('integrations.pd.events')}>
        <div className="row3">
          <Field label={t('integrations.pd.region')}>
            <select value={form.region ?? s.region} onChange={(e) => f('region', e.target.value)}>
              <option value="us">US (events.pagerduty.com)</option>
              <option value="eu">EU (events.eu.pagerduty.com)</option>
            </select>
          </Field>
          <Field label={t('integrations.pd.routingKey')} help={s.routing_key_ref ? `${t('integrations.pd.keySet')}${s.service_name ? ` · ${s.service_name}` : ''}` : t('integrations.pd.routingKeyHelp')}>
            <input type="password" value={form.routing_key ?? ''} onChange={(e) => f('routing_key', e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t('integrations.pd.minSeverity')}>
            <select value={form.min_severity ?? s.min_severity} onChange={(e) => f('min_severity', e.target.value as Severity)}>
              {SEVERITIES.map((x) => (
                <option key={x} value={x}>
                  {sevLabel(x)}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {s.api_token_ref && (
          <div className="row3">
            <Field label={t('integrations.pd.service')}>
              {services ? (
                <select value={service} onChange={(e) => setService(e.target.value)}>
                  <option value="">{t('integrations.pd.pickService')}</option>
                  {services.map((x) => (
                    <option key={x.id} value={x.id}>
                      {x.name}
                      {x.events_key ? '' : ' *'}
                    </option>
                  ))}
                </select>
              ) : (
                <button className="btn" onClick={loadServices} disabled={busy}>
                  {t('integrations.pd.loadServices')}
                </button>
              )}
            </Field>
            {services && (
              <>
                <label className="check">
                  <input type="checkbox" checked={createKey} onChange={(e) => setCreateKey(e.target.checked)} /> {t('integrations.pd.createKey')}
                </label>
                <div>
                  <button className="btn" onClick={useService} disabled={busy || !service}>
                    {t('integrations.pd.useService')}
                  </button>
                </div>
              </>
            )}
          </div>
        )}
        <details>
          <summary className="link">{t('integrations.pd.advanced')}</summary>
          <div className="row2">
            <Field label="Events API URL">
              <input className="mono" value={form.events_url ?? s.events_url ?? ''} placeholder={data.events_url} onChange={(e) => f('events_url', e.target.value)} />
            </Field>
            <Field label="REST API URL">
              <input className="mono" value={form.api_url ?? s.api_url ?? ''} placeholder={data.api_url} onChange={(e) => f('api_url', e.target.value)} />
            </Field>
          </div>
        </details>
      </Section>
      <Section title={t('integrations.pd.rest')}>
        <div className="row2">
          <Field label={t('integrations.pd.apiToken')} help={s.api_token_ref ? t('integrations.pd.tokenSet') : t('integrations.pd.apiTokenHelp')}>
            <input type="password" value={form.api_token ?? ''} onChange={(e) => f('api_token', e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t('integrations.pd.policies')}>
            {policies ? (
              <select
                multiple
                value={s.escalation_policies}
                onChange={(e) => put({ escalation_policies: Array.from(e.target.selectedOptions).map((o) => o.value) })}
              >
                {policies.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            ) : (
              <button className="btn" onClick={loadPolicies} disabled={busy || !s.api_token_ref}>
                {s.escalation_policies.length ? s.escalation_policies.join(', ') : t('integrations.pd.allPolicies')} · {t('integrations.pd.loadPolicies')}
              </button>
            )}
          </Field>
        </div>
      </Section>
      <Section title={t('integrations.pd.webhooks')}>
        <Field label={t('integrations.pd.webhookURL')}>
          <div className="copy-box">
            <input readOnly value={data.webhook_url} />
            <button className="icon-btn icon-btn-sm" onClick={() => copy(data.webhook_url, toast)}>
              <Copy size={14} />
            </button>
          </div>
        </Field>
        <div className="row2">
          <Field label={t('integrations.pd.webhookSecret')} help={s.webhook_secret_ref ? t('integrations.pd.secretSet') : t('integrations.pd.webhookSecretHelp')}>
            <input type="password" value={form.webhook_secret ?? ''} onChange={(e) => f('webhook_secret', e.target.value)} autoComplete="new-password" />
          </Field>
          <div className="int-actions">
            {s.api_token_ref && (
              <button className="btn" onClick={subscribe} disabled={busy}>
                {t('integrations.pd.subscribe')}
              </button>
            )}
            {s.webhook_subscription_id && (
              <button className="btn btn-ghost" onClick={unsubscribe} disabled={busy}>
                {t('integrations.pd.unsubscribe')} {s.webhook_subscription_id}
              </button>
            )}
          </div>
        </div>
        <button className="btn btn-primary" onClick={save} disabled={busy}>
          {t('integrations.pd.save')}
        </button>
      </Section>
      <Section title={t('integrations.pd.routes')}>
        <p className="hint">{t('integrations.pd.routesHint')}</p>
        {s.routes.length === 0 ? (
          <p className="muted">{t('integrations.pd.noRoutes')}</p>
        ) : (
          <table className="table table-compact">
            <thead>
              <tr>
                <th>{t('integrations.pd.routeName')}</th>
                <th>{t('integrations.pd.routeTeam')}</th>
                <th>{t('integrations.pd.routeService')}</th>
                <th>{t('integrations.pd.routeTarget')}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {s.routes.map((r) => (
                <tr key={r.id}>
                  <td>{r.name}</td>
                  <td>{r.team || '—'}</td>
                  <td>{r.service || '—'}</td>
                  <td className="mono">{r.service_name || r.routing_key_ref}</td>
                  <td>
                    <button className="icon-btn icon-btn-sm" onClick={() => delRoute(r.id)}>
                      <X size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row3">
          <input placeholder={t('integrations.pd.routeName')} value={route.name} onChange={(e) => setRoute({ ...route, name: e.target.value })} />
          <input placeholder={t('integrations.pd.routeTeam')} value={route.team} onChange={(e) => setRoute({ ...route, team: e.target.value })} />
          <select value={route.service} onChange={(e) => setRoute({ ...route, service: e.target.value })}>
            <option value="">{t('integrations.pd.routeService')}: —</option>
            {cis.map((c) => (
              <option key={c.id} value={c.name}>
                {c.name} ({ciTypeLabel(c.type)})
              </option>
            ))}
          </select>
        </div>
        <div className="row3">
          {services ? (
            <select value={route.service_id} onChange={(e) => setRoute({ ...route, service_id: e.target.value })}>
              <option value="">{t('integrations.pd.pickService')}</option>
              {services.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
            </select>
          ) : (
            <input type="password" placeholder={t('integrations.pd.routeKey')} value={route.routing_key} onChange={(e) => setRoute({ ...route, routing_key: e.target.value })} autoComplete="new-password" />
          )}
          <div />
          <button className="btn" onClick={addRoute} disabled={busy || !route.name || (!route.team && !route.service) || (!route.routing_key && !route.service_id)}>
            <Plus size={13} /> {t('integrations.pd.addRoute')}
          </button>
        </div>
      </Section>
      <Section
        title={t('integrations.pd.oncall')}
        actions={
          <button className="btn btn-sm" onClick={syncOnCall} disabled={busy || !s.api_token_ref}>
            <RefreshCw size={13} /> {t('integrations.pd.oncallSync')}
          </button>
        }
      >
        {data.oncall.error && <div className="form-error">{data.oncall.error}</div>}
        {data.oncall.entries.length === 0 ? (
          <p className="muted">{t('integrations.pd.oncallEmpty')}</p>
        ) : (
          <>
            <p className="hint">{t('integrations.pd.oncallSynced', { time: fmtTime(data.oncall.synced_at) })}</p>
            <table className="table table-compact">
              <thead>
                <tr>
                  <th>{t('integrations.pd.policy')}</th>
                  <th>{t('integrations.pd.level')}</th>
                  <th>{t('integrations.pd.user')}</th>
                  <th>{t('integrations.pd.until')}</th>
                </tr>
              </thead>
              <tbody>
                {data.oncall.entries.map((e, i) => (
                  <tr key={i}>
                    <td>{e.policy_name}</td>
                    <td className="num">{e.level}</td>
                    <td>
                      {e.user_name} {e.email && <span className="muted">{e.email}</span>}
                    </td>
                    <td>{e.end ? fmtTime(e.end) : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </Section>
    </>
  )
}

function Grafana() {
  const { toast } = useApp()
  const { data, setData } = useFetch<{ settings: { grafana_url: string } }>('/api/settings')
  const [url, setUrl] = useState<string | null>(null)
  if (!data) return <Empty>{t('common.words.loading')}</Empty>
  const save = async () => {
    try {
      setData(await api.put<{ settings: { grafana_url: string } }>('/api/settings', { grafana_url: url ?? data.settings.grafana_url }))
      setUrl(null)
      toast(t('integrations.grafana.saved'))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <Section title={t('integrations.grafana.title')}>
      <p className="hint">{t('integrations.grafana.hint')}</p>
      <Field label={t('integrations.grafana.url')}>
        <input className="mono" value={url ?? data.settings.grafana_url} onChange={(e) => setUrl(e.target.value)} placeholder="https://grafana.example.com/d/umbrella-incidents/incidents" />
      </Field>
      <button className="btn btn-primary" onClick={save}>
        {t('integrations.grafana.save')}
      </button>
    </Section>
  )
}

function OpenBao() {
  const { data } = useFetch<OpenBaoStatus>('/api/openbao')
  if (!data) return <Empty>{t('common.words.loading')}</Empty>
  const ok = data.configured && data.token_ok && data.mount_ok && !data.sealed
  const state = ok ? t('integrations.openbao.ok') : !data.configured ? t('integrations.openbao.off') : data.sealed ? t('integrations.openbao.sealed') : t('integrations.openbao.fail')
  const row = (k: string, v: ReactNode) => (
    <div className="prop">
      <div className="prop-k">{k}</div>
      <div className="prop-v">{v ?? '—'}</div>
    </div>
  )
  return (
    <Section title={t('integrations.openbao.title')}>
      <p className="hint">{t('integrations.openbao.hint')}</p>
      <div className="props">
        {row(t('integrations.openbao.state'), (
          <>
            <StatusDot ok={ok} /> {state} {data.error && <span className="text-danger">{data.error}</span>}
          </>
        ))}
        {row(t('integrations.openbao.addr'), <span className="mono">{data.addr}</span>)}
        {row(t('integrations.openbao.version'), data.version)}
        {row(t('integrations.openbao.mount'), <span className="mono">{data.mount}</span>)}
        {row(t('integrations.openbao.auth'), data.auth)}
        {row(t('integrations.openbao.token'), data.token_expires ? fmtTime(data.token_expires) : '—')}
        {row(t('integrations.openbao.policies'), data.policies?.join(', '))}
      </div>
    </Section>
  )
}
