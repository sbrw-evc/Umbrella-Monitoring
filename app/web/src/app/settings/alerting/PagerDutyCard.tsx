import { useEffect, useState, type ReactNode } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { api } from '../../../api'
import { useResource } from '../../../connections/useRequest'
import { useLocale, useT } from '../../../i18n'
import { Banner, Button, Field, formatDate, Input, Password, Rows, Select, Switch } from '../../../ui'
import { ProfileCard } from '../../profile/ProfileCard'
import { useAction } from '../../profile/useAction'
import { useSession } from '../../session'
import { strings } from './strings'
import { SEVERITIES, type PagerDutyView, type PDService, type Refs } from './types'

type RouteDraft = { key: string; id: string; name: string; team_id: string; service_id: string; routing_key: string; pd_service_id: string; has_key: boolean }

type Draft = {
  enabled: boolean
  region: 'us' | 'eu'
  min_severity: string
  public_url: string
  events_url: string
  api_url: string
  routing_key: string
  pd_service_id: string
  api_token: string
  webhook_secret: string
  routes: RouteDraft[]
}

let seq = 0

function draftOf(v: PagerDutyView): Draft {
  return {
    enabled: v.enabled,
    region: v.region,
    min_severity: v.min_severity ?? '',
    public_url: v.public_url,
    events_url: v.events_url ?? '',
    api_url: v.api_url ?? '',
    routing_key: '',
    pd_service_id: v.service_id ?? '',
    api_token: '',
    webhook_secret: '',
    routes: v.routes.map((r) => ({
      key: r.id,
      id: r.id,
      name: r.name,
      team_id: r.team_id ?? '',
      service_id: r.service_id ?? '',
      routing_key: '',
      pd_service_id: r.pd_service_id ?? '',
      has_key: r.has_key,
    })),
  }
}

function bodyOf(d: Draft) {
  return {
    enabled: d.enabled,
    region: d.region,
    min_severity: d.min_severity,
    public_url: d.public_url,
    events_url: d.events_url,
    api_url: d.api_url,
    routing_key: d.routing_key,
    pd_service_id: d.pd_service_id,
    api_token: d.api_token,
    webhook_secret: d.webhook_secret,
    routes: d.routes.map((r) => ({ id: r.id, name: r.name, team_id: r.team_id, service_id: r.service_id, routing_key: r.routing_key, pd_service_id: r.pd_service_id })),
  }
}

export function PagerDutyCard({ onSaved }: { onSaved?: () => void }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { can, timezone } = useSession()
  const canEdit = can('settings.alerting:edit')
  const canTest = can('settings.alerting:test')
  const [view, setView] = useState<PagerDutyView | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [services, setServices] = useState<PDService[] | null>(null)
  const refs = useResource<Refs>('/api/refs', 0)
  const loader = useAction(strings)
  const saver = useAction(strings)
  const other = useAction(strings)

  const apply = (v: PagerDutyView) => {
    setView(v)
    setDraft(draftOf(v))
  }
  const { run: load } = loader
  useEffect(() => {
    void load(async () => apply(await api<PagerDutyView>('GET', '/api/pagerduty')))
  }, [load])
  const hasToken = !!view?.has_api_token
  useEffect(() => {
    if (!hasToken || !canEdit) return
    api<PDService[]>('GET', '/api/pagerduty/services').then(setServices, () => setServices(null))
  }, [hasToken, canEdit])

  if (!view || !draft) {
    return (
      <ProfileCard title={t('pd.title')} action={loader}>
        {!loader.error && <p className="muted">{t('loading')}</p>}
      </ProfileCard>
    )
  }
  const at = (v?: string) => formatDate(v, locale, timezone)
  const dirty = JSON.stringify(bodyOf(draft)) !== JSON.stringify(bodyOf(draftOf(view)))
  const set = (p: Partial<Draft>) => setDraft({ ...draft, ...p })
  const setRoute = (i: number, p: Partial<RouteDraft>) => set({ routes: draft.routes.map((r, j) => (j === i ? { ...r, ...p } : r)) })
  const st = view.status

  const save = () =>
    saver.run(async () => {
      apply(await api<PagerDutyView>('PUT', '/api/pagerduty', bodyOf(draft)))
      onSaved?.()
      return t('saved')
    })
  const call = (method: string, path: string, done: string, body?: unknown) =>
    other.run(async () => {
      const r = await api<PagerDutyView | unknown>(method, path, body)
      if (r && typeof r === 'object' && 'routes' in r) apply(r as PagerDutyView)
      return done
    })

  const servicePicker = (value: string, onChange: (v: string) => void, label: string) =>
    services && services.length > 0 ? (
      <Field label={label}>
        {(id) => (
          <Select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
            <option value="">{t('pd.service.manual')}</option>
            {services.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        )}
      </Field>
    ) : null

  const status: [string, ReactNode][] = [
    [
      t('pd.state'),
      <span key="s" className={`pill pill-${!view.enabled ? 'off' : st.breaker_open ? 'error' : 'ok'}`}>
        {t(!view.enabled ? 'pd.state.off' : st.breaker_open ? 'pd.state.breaker' : 'pd.state.on')}
      </span>,
    ],
    [t('pd.sent'), `${st.sent} / ${st.failed}`],
    [t('pd.queue'), String(st.queue)],
    [t('pd.lastSuccess'), at(st.last_success_at)],
    [t('pd.lastWebhook'), at(st.last_webhook_at)],
  ]
  if (st.last_error) status.push([t('pd.lastError'), <span key="e" className="inc-warn">{`${at(st.last_error_at)} · ${st.last_error}`}</span>])

  return (
    <ProfileCard
      title={t('pd.title')}
      action={saver}
      onSubmit={save}
      wide
      footer={
        <div className="row">
          {canTest && view.enabled && (
            <Button busy={other.busy} disabled={dirty} onClick={() => void call('POST', '/api/pagerduty/test', t('pd.test.ok'))}>
              {t('pd.test')}
            </Button>
          )}
          {canEdit && (
            <Button type="submit" variant="primary" busy={saver.busy} disabled={!dirty}>
              {t('save')}
            </Button>
          )}
        </div>
      }
    >
      <p className="muted">{t('pd.text')}</p>
      <Rows rows={status} />
      <fieldset className="plain-fieldset stack" disabled={!canEdit}>
        <Switch checked={draft.enabled} onChange={(enabled) => set({ enabled })} label={t('pd.enable')} hint={t('pd.enable.hint')} />
        <Field label={t('pd.public')} hint={t('pd.public.hint')}>
          {(id) => <Input id={id} value={draft.public_url} placeholder="https://umbrella.example.com" onChange={(e) => set({ public_url: e.target.value })} />}
        </Field>
        <div className="al-grid">
          <div className="al-col stack">
            <div className="al-row">
              <Field label={t('pd.region')}>
                {(id) => (
                  <Select id={id} value={draft.region} onChange={(e) => set({ region: e.target.value as Draft['region'] })}>
                    <option value="us">{t('pd.region.us')}</option>
                    <option value="eu">{t('pd.region.eu')}</option>
                  </Select>
                )}
              </Field>
              <Field label={t('pd.min')} hint={t('pd.min.hint')}>
                {(id) => (
                  <Select id={id} value={draft.min_severity} onChange={(e) => set({ min_severity: e.target.value })}>
                    <option value="">{t('sev.info')}</option>
                    {SEVERITIES.filter((s) => s !== 'info').map((s) => (
                      <option key={s} value={s}>
                        {t(`sev.${s}`)}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
            <Field label={t('pd.token')} hint={view.has_api_token && !draft.api_token ? t('keep') : t('pd.token.hint')}>
              {(id) => <Password id={id} value={draft.api_token} autoComplete="off" onChange={(e) => set({ api_token: e.target.value })} />}
            </Field>
            {canTest && (draft.api_token || view.has_api_token) && (
              <div className="row">
                <Button busy={other.busy} onClick={() => void call('POST', '/api/pagerduty/check', t('pd.token.ok'), { api_token: draft.api_token })}>
                  {t('pd.token.check')}
                </Button>
              </div>
            )}
          </div>
          <div className="al-col stack">
            <h3 className="al-sub">{t('pd.default')}</h3>
            <p className="hint">{t('pd.default.hint')}</p>
            {servicePicker(draft.pd_service_id, (pd_service_id) => set({ pd_service_id, routing_key: '' }), t('pd.service'))}
            {(!services || !draft.pd_service_id) && (
              <Field label={t('pd.key')} hint={view.has_routing_key && !draft.routing_key ? t('keep') : undefined}>
                {(id) => <Password id={id} value={draft.routing_key} autoComplete="off" onChange={(e) => set({ routing_key: e.target.value })} />}
              </Field>
            )}
            {view.service_name && draft.pd_service_id === view.service_id && <p className="hint">{view.service_name}</p>}
          </div>
        </div>

        <h3 className="al-sub">{t('pd.routes')}</h3>
        <p className="hint">{t('pd.routes.hint')}</p>
        {draft.routes.map((r, i) => (
          <div key={r.key} className="al-route">
            <Field label={t('pd.route.name')}>{(id) => <Input id={id} value={r.name} onChange={(e) => setRoute(i, { name: e.target.value })} />}</Field>
            <Field label={t('pd.route.service')}>
              {(id) => (
                <Select id={id} value={r.service_id} onChange={(e) => setRoute(i, { service_id: e.target.value })}>
                  <option value="">{t('pd.route.any')}</option>
                  {(refs.data?.services ?? []).map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label={t('pd.route.team')}>
              {(id) => (
                <Select id={id} value={r.team_id} onChange={(e) => setRoute(i, { team_id: e.target.value })}>
                  <option value="">{t('pd.route.any')}</option>
                  {(refs.data?.teams ?? []).map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {servicePicker(r.pd_service_id, (pd_service_id) => setRoute(i, { pd_service_id, routing_key: '' }), t('pd.service'))}
            {(!services || !r.pd_service_id) && (
              <Field label={t('pd.key')} hint={r.has_key && !r.routing_key ? t('keep') : undefined}>
                {(id) => <Password id={id} value={r.routing_key} autoComplete="off" onChange={(e) => setRoute(i, { routing_key: e.target.value })} />}
              </Field>
            )}
            <button type="button" className="icon-btn al-remove" aria-label={t('remove')} title={t('remove')} onClick={() => set({ routes: draft.routes.filter((_, j) => j !== i) })}>
              <Trash2 size={16} />
            </button>
          </div>
        ))}
        <div className="row">
          <Button
            onClick={() =>
              set({ routes: [...draft.routes, { key: `new-${++seq}`, id: '', name: '', team_id: '', service_id: '', routing_key: '', pd_service_id: '', has_key: false }] })
            }
          >
            <Plus size={16} aria-hidden />
            {t('pd.route.add')}
          </Button>
        </div>

        <h3 className="al-sub">{t('pd.webhook')}</h3>
        <Rows
          rows={[
            [t('pd.webhook.url'), view.webhook_url ? <code key="u">{view.webhook_url}</code> : <span className="muted">{t('pd.webhook.nourl')}</span>],
            [
              t('state'),
              <span key="w" className={`pill pill-${view.webhook_subscription_id || view.has_webhook_secret ? 'ok' : 'off'}`}>
                {t(view.webhook_subscription_id || view.has_webhook_secret ? 'pd.webhook.on' : 'pd.webhook.off')}
              </span>,
            ],
          ]}
        />
        {view.webhook_url && view.has_api_token && (
          <div className="row">
            <Button busy={other.busy} disabled={dirty} onClick={() => void call('POST', '/api/pagerduty/subscription', t('saved'))}>
              {t('pd.webhook.subscribe')}
            </Button>
            {view.webhook_subscription_id && (
              <Button busy={other.busy} disabled={dirty} onClick={() => void call('DELETE', '/api/pagerduty/subscription', t('saved'))}>
                {t('pd.webhook.unsubscribe')}
              </Button>
            )}
          </div>
        )}
        <Field label={t('pd.webhook.secret')} hint={view.has_webhook_secret && !draft.webhook_secret ? t('keep') : t('pd.webhook.secret.hint')}>
          {(id) => <Password id={id} value={draft.webhook_secret} autoComplete="off" onChange={(e) => set({ webhook_secret: e.target.value })} />}
        </Field>
      </fieldset>
      {other.notice && <Banner kind="ok" title={other.notice} />}
      {other.error && (
        <Banner kind="error" title={other.error.message}>
          {other.error.detail}
        </Banner>
      )}
    </ProfileCard>
  )
}
