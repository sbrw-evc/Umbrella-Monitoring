import { useEffect, useState, type ReactNode } from 'react'
import { Send } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { api } from '../../../api'
import { useT } from '../../../i18n'
import { Banner, Button, Field, Input, Password, Select, Switch, Textarea } from '../../../ui'
import { ProfileCard } from '../../profile/ProfileCard'
import { SummaryCard } from '../../profile/SummaryCard'
import { useAction } from '../../profile/useAction'
import { useSession } from '../../session'
import { reveal } from './PagerDutyCard'
import { strings } from './strings'
import type { NotifyView } from './types'

type Draft = {
  email: NotifyView['email'] & { password: string; port_text: string }
  telegram: { enabled: boolean; api_url: string; token: string }
  extra_emails: string
  extra_telegram: string
}

function draftOf(v: NotifyView): Draft {
  return {
    email: { ...v.email, password: '', port_text: v.email.port ? String(v.email.port) : '' },
    telegram: { enabled: v.telegram.enabled, api_url: v.telegram.api_url ?? '', token: '' },
    extra_emails: v.extra_emails.join('\n'),
    extra_telegram: v.extra_telegram.join('\n'),
  }
}

const lines = (v: string) =>
  v
    .split(/[\n,;]/)
    .map((x) => x.trim())
    .filter(Boolean)

function bodyOf(d: Draft) {
  return {
    email: {
      enabled: d.email.enabled,
      host: d.email.host,
      port: Number(d.email.port_text) || 0,
      security: d.email.security,
      skip_verify: d.email.skip_verify,
      username: d.email.username,
      password: d.email.password,
      from: d.email.from,
    },
    telegram: { enabled: d.telegram.enabled, token: d.telegram.token, api_url: d.telegram.api_url },
    extra_emails: lines(d.extra_emails),
    extra_telegram: lines(d.extra_telegram),
  }
}

export function NotifyCard() {
  const t = useT(strings)
  const { can } = useSession()
  const canEdit = can('settings.alerting:edit')
  const canTest = can('settings.alerting:test')
  const [view, setView] = useState<NotifyView | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const loader = useAction(strings)
  const saver = useAction(strings)
  const tester = useAction(strings)
  const [testTo, setTestTo] = useState({ email: '', telegram: '' })

  const apply = (v: NotifyView) => {
    setView(v)
    setDraft(draftOf(v))
  }
  const { run: load } = loader
  useEffect(() => {
    void load(async () => apply(await api<NotifyView>('GET', '/api/notifications')))
  }, [load])

  if (!view || !draft) {
    return (
      <ProfileCard title={t('nt.title')} action={loader}>
        {!loader.error && <p className="muted">{t('loading')}</p>}
      </ProfileCard>
    )
  }
  const dirty = JSON.stringify(bodyOf(draft)) !== JSON.stringify(bodyOf(draftOf(view)))
  const setEmail = (p: Partial<Draft['email']>) => setDraft({ ...draft, email: { ...draft.email, ...p } })
  const setTg = (p: Partial<Draft['telegram']>) => setDraft({ ...draft, telegram: { ...draft.telegram, ...p } })

  const save = () =>
    saver.run(async () => {
      apply(await api<NotifyView>('PUT', '/api/notifications', bodyOf(draft)))
      return t('saved')
    })
  const test = (channel: 'email' | 'telegram') =>
    tester.run(async () => {
      const r = await api<{ to: string; bot?: string }>('POST', '/api/notifications/test', { channel, to: testTo[channel] })
      return r.bot ? t('nt.test.okBot', { to: r.to, bot: r.bot }) : t('nt.test.ok', { to: r.to })
    })

  const on = view.email.enabled || view.telegram.enabled
  const extra = [
    view.extra_emails.length ? t('nt.extra.n.emails', { n: view.extra_emails.length }) : '',
    view.extra_telegram.length ? t('nt.extra.n.chats', { n: view.extra_telegram.length }) : '',
  ]
    .filter(Boolean)
    .join(', ')
  const rows: [string, ReactNode][] = [
    [t('nt.email'), view.email.enabled ? [t('pd.state.on'), view.email.host].filter(Boolean).join(' · ') : t('pd.state.off')],
    [t('nt.telegram'), t(view.telegram.enabled ? 'pd.state.on' : 'pd.state.off')],
    [t('nt.extra'), extra],
  ]

  return (
    <>
      <SummaryCard title={t('nt.title')} badge={<span className={`pill pill-${on ? 'ok' : 'off'}`}>{t(on ? 'pd.state.on' : 'pd.state.off')}</span>} text={t('nt.text')} rows={rows}>
        {!view.links && <Banner kind="info" title={t('nt.links.off')} />}
      </SummaryCard>
      <ProfileCard
        title={t('nt.settings')}
        action={saver}
        onSubmit={save}
        footer={
          canEdit && (
            <Button type="submit" variant="primary" busy={saver.busy} disabled={!dirty}>
              {t('save')}
            </Button>
          )
        }
      >
        <fieldset className="plain-fieldset al-grid" disabled={!canEdit}>
          <div className="al-col stack">
            <h3 className="al-sub">{t('nt.email')}</h3>
            <Switch checked={draft.email.enabled} onChange={(enabled) => setEmail({ enabled })} label={t('nt.email.enable')} />
            <Reveal open={draft.email.enabled}>
              <div className="grid-2">
                <Field label={t('nt.host')}>{(id) => <Input id={id} value={draft.email.host} placeholder="smtp.example.com" onChange={(e) => setEmail({ host: e.target.value })} />}</Field>
                <Field label={t('nt.port')} hint={t('nt.port.hint')}>
                  {(id) => <Input id={id} value={draft.email.port_text} inputMode="numeric" placeholder="587" onChange={(e) => setEmail({ port_text: e.target.value.replace(/\D/g, '') })} />}
                </Field>
              </div>
              <Field label={t('nt.security')}>
                {(id) => (
                  <Select id={id} value={draft.email.security} onChange={(e) => setEmail({ security: e.target.value as Draft['email']['security'] })}>
                    {(['starttls', 'tls', 'none'] as const).map((s) => (
                      <option key={s} value={s}>
                        {t(`nt.security.${s}`)}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              {draft.email.security !== 'none' && <Switch checked={draft.email.skip_verify} onChange={(skip_verify) => setEmail({ skip_verify })} label={t('nt.skip')} />}
              <div className="grid-2">
                <Field label={t('nt.user')}>
                  {(id) => <Input id={id} value={draft.email.username} autoComplete="off" onChange={(e) => setEmail({ username: e.target.value })} />}
                </Field>
                <Field label={t('nt.password')} hint={view.has_password && !draft.email.password ? t('keep') : undefined}>
                  {(id) => <Password id={id} value={draft.email.password} autoComplete="new-password" onChange={(e) => setEmail({ password: e.target.value })} />}
                </Field>
              </div>
              <Field label={t('nt.from')} hint={t('nt.from.hint')}>
                {(id) => <Input id={id} value={draft.email.from} onChange={(e) => setEmail({ from: e.target.value })} />}
              </Field>
            </Reveal>
          </div>
          <div className="al-col stack">
            <h3 className="al-sub">{t('nt.telegram')}</h3>
            <Switch checked={draft.telegram.enabled} onChange={(enabled) => setTg({ enabled })} label={t('nt.telegram.enable')} />
            <Reveal open={draft.telegram.enabled}>
              <Field label={t('nt.token')} hint={view.has_token && !draft.telegram.token ? t('keep') : t('nt.token.hint')}>
                {(id) => <Password id={id} value={draft.telegram.token} autoComplete="off" placeholder="123456:ABC…" onChange={(e) => setTg({ token: e.target.value })} />}
              </Field>
              <Field label={t('nt.api')} hint={t('nt.api.hint')}>
                {(id) => <Input id={id} value={draft.telegram.api_url} placeholder="https://api.telegram.org" onChange={(e) => setTg({ api_url: e.target.value })} />}
              </Field>
            </Reveal>
          </div>
        </fieldset>
        <section className="al-section">
          <h3 className="al-sub">{t('nt.extra')}</h3>
          <fieldset className="plain-fieldset grid-2" disabled={!canEdit}>
            <Field label={t('nt.extra.emails')} hint={t('nt.extra.emails.hint')}>
              {(id) => <Textarea id={id} rows={2} value={draft.extra_emails} onChange={(e) => setDraft({ ...draft, extra_emails: e.target.value })} />}
            </Field>
            <Field label={t('nt.extra.chats')} hint={t('nt.extra.chats.hint')}>
              {(id) => <Textarea id={id} rows={2} value={draft.extra_telegram} onChange={(e) => setDraft({ ...draft, extra_telegram: e.target.value })} />}
            </Field>
          </fieldset>
        </section>
        {canTest && (view.email.enabled || view.telegram.enabled) && (
          <section className="al-section">
            <h3 className="al-sub">{t('nt.test')}</h3>
            {dirty && <p className="hint">{t('nt.test.saveFirst')}</p>}
            {view.email.enabled && (
              <div className="al-test-row">
                <Input aria-label={t('nt.test.to')} value={testTo.email} placeholder={t('nt.test.to.email')} onChange={(e) => setTestTo({ ...testTo, email: e.target.value })} />
                <Button busy={tester.busy} onClick={() => void test('email')}>
                  <Send size={15} aria-hidden />
                  {t('nt.email')}
                </Button>
              </div>
            )}
            {view.telegram.enabled && (
              <div className="al-test-row">
                <Input aria-label={t('nt.test.to')} value={testTo.telegram} placeholder={t('nt.test.to.chat')} onChange={(e) => setTestTo({ ...testTo, telegram: e.target.value })} />
                <Button busy={tester.busy} onClick={() => void test('telegram')}>
                  <Send size={15} aria-hidden />
                  Telegram
                </Button>
              </div>
            )}
            {tester.notice && <Banner kind="ok" title={tester.notice} />}
            {tester.error && (
              <Banner kind="error" title={tester.error.message}>
                {tester.error.detail}
              </Banner>
            )}
          </section>
        )}
      </ProfileCard>
    </>
  )
}

function Reveal({ open, children }: { open: boolean; children: ReactNode }) {
  return (
    <AnimatePresence initial={false}>
      {open && (
        <motion.div className="reveal-box" initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }} transition={reveal}>
          <div className="stack">{children}</div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
