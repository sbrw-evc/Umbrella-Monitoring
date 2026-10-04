import { useEffect, useRef, useState, type FormEvent, type ReactNode, type RefObject } from 'react'
import { Check, CheckCircle2, Circle } from 'lucide-react'
import { api, ApiError, type Locale, type Meta, type Theme } from '../api'
import { errorText, useLocale, useT } from '../i18n'
import { useTheme } from '../theme'
import { Banner, Brand, Button, Field, formatDate, Input, Password, Preferences, Rows, Select, Switch, Textarea } from '../ui'
import { strings } from './strings'

const TOKEN_HEADER = 'X-Setup-Token'

type StepId = 'code' | 'prefs' | 'openbao' | 'postgres' | 'ldap' | 'admin' | 'review'
const STEPS: StepId[] = ['code', 'prefs', 'openbao', 'postgres', 'ldap', 'admin', 'review']

type OpenBao = {
  addr: string
  mount: string
  namespace: string
  auth: 'approle' | 'token'
  token: string
  role_id: string
  secret_id: string
  approle_path: string
  ca_cert: string
  skip_verify: boolean
}

type Postgres = { host: string; port: string; database: string; user: string; password: string; sslmode: string; reuse_existing: boolean }

type Kind = 'ad' | 'openldap'

type Ldap = {
  enabled: boolean
  kind: Kind
  url: string
  start_tls: boolean
  skip_verify: boolean
  ca_cert: string
  bind_dn: string
  bind_password: string
  base_dn: string
  user_filter: string
  username_attr: string
  name_attr: string
  email_attr: string
  admin_group_dn: string
  test_username: string
  test_password: string
}

type Admin = { username: string; name: string; email: string; password: string; confirm: string }

type Check<T> = { key: string; ok: boolean; result?: T; error?: string } | null

type ObResult = { ok: boolean; error?: string; write_ok: boolean; status: { version?: string; policies?: string[]; token_expires?: string; mount?: string } }
type PgResult = { ok: boolean; error?: string; probe: { version: string; database: string; user: string; can_create: boolean; has_state: boolean; saved_at?: string } }
type LdResult = {
  ok: boolean
  error?: string
  probe: { server: string; tls: string; base_dn: string; admin_group: boolean; user_authenticated: boolean; user?: { dn: string; name: string; admin: boolean } }
}

const DEFAULTS: Record<Kind, Pick<Ldap, 'user_filter' | 'username_attr' | 'name_attr' | 'email_attr'>> = {
  ad: { user_filter: '(&(objectCategory=person)(objectClass=user)(sAMAccountName={username}))', username_attr: 'sAMAccountName', name_attr: 'displayName', email_attr: 'mail' },
  openldap: { user_filter: '(&(objectClass=inetOrgPerson)(uid={username}))', username_attr: 'uid', name_attr: 'cn', email_attr: 'mail' },
}

const USERNAME = /^[A-Za-z0-9][A-Za-z0-9._@-]{1,63}$/

function stepOf(code: string): StepId | null {
  if (code === 'invalid_setup_token') return 'code'
  if (code.startsWith('openbao_')) return 'openbao'
  if (code.startsWith('postgres_')) return 'postgres'
  if (code.startsWith('ldap_')) return 'ldap'
  if (['weak_password', 'invalid_username', 'invalid_email', 'invalid_name'].includes(code)) return 'admin'
  if (code === 'invalid_locale' || code === 'invalid_theme') return 'prefs'
  return null
}

function obBody(o: OpenBao) {
  const base = { addr: o.addr.trim(), mount: o.mount.trim(), namespace: o.namespace.trim(), auth: o.auth, ca_cert: o.ca_cert.trim(), skip_verify: o.skip_verify }
  return o.auth === 'token' ? { ...base, token: o.token } : { ...base, role_id: o.role_id, secret_id: o.secret_id, approle_path: o.approle_path.trim() }
}

function pgBody(p: Postgres) {
  return { host: p.host.trim(), port: Number(p.port) || 0, database: p.database.trim(), user: p.user.trim(), password: p.password, sslmode: p.sslmode, reuse_existing: p.reuse_existing }
}

function ldConfig(l: Ldap) {
  return {
    enabled: l.enabled,
    kind: l.kind,
    url: l.url.trim(),
    start_tls: l.start_tls,
    skip_verify: l.skip_verify,
    ca_cert: l.ca_cert.trim(),
    bind_dn: l.bind_dn.trim(),
    base_dn: l.base_dn.trim(),
    user_filter: l.user_filter.trim(),
    username_attr: l.username_attr.trim(),
    name_attr: l.name_attr.trim(),
    email_attr: l.email_attr.trim(),
    admin_group_dn: l.admin_group_dn.trim(),
  }
}

function passwordRules(a: Admin) {
  return {
    length: [...a.password].length >= 10,
    mix: /\p{L}/u.test(a.password) && /\p{N}/u.test(a.password),
    user: a.password !== '' && a.password.toLowerCase() !== a.username.trim().toLowerCase(),
    match: a.password !== '' && a.password === a.confirm,
  }
}

export default function SetupApp({ meta }: { meta: Meta }) {
  const t = useT(strings)
  const { locale, setLocale } = useLocale()
  const { theme, setTheme } = useTheme()

  const [step, setStep] = useState<StepId>('code')
  const [reached, setReached] = useState(0)
  const [code, setCode] = useState('')
  const [codeOk, setCodeOk] = useState(false)
  const [defaults, setDefaults] = useState<{ locale: Locale; theme: Theme }>({ locale, theme })
  const [ob, setOb] = useState<OpenBao>({ addr: '', mount: 'umbrella', namespace: '', auth: 'approle', token: '', role_id: '', secret_id: '', approle_path: 'approle', ca_cert: '', skip_verify: false })
  const [pg, setPg] = useState<Postgres>({ host: '', port: '5432', database: 'umbrella', user: 'umbrella', password: '', sslmode: 'prefer', reuse_existing: false })
  const [ld, setLd] = useState<Ldap>({
    enabled: false,
    kind: 'ad',
    url: '',
    start_tls: false,
    skip_verify: false,
    ca_cert: '',
    bind_dn: '',
    bind_password: '',
    base_dn: '',
    ...DEFAULTS.ad,
    admin_group_dn: '',
    test_username: '',
    test_password: '',
  })
  const [admin, setAdmin] = useState<Admin>({ username: 'admin', name: '', email: '', password: '', confirm: '' })
  const [obCheck, setObCheck] = useState<Check<ObResult>>(null)
  const [pgCheck, setPgCheck] = useState<Check<PgResult>>(null)
  const [ldCheck, setLdCheck] = useState<Check<LdResult>>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ message: string; detail?: string; step?: StepId | null } | null>(null)
  const [done, setDone] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)

  const headers = { [TOKEN_HEADER]: code.trim() }
  const obKey = JSON.stringify(obBody(ob))
  const pgKey = JSON.stringify({ ...pgBody(pg), reuse_existing: undefined })
  const ldKey = JSON.stringify({ c: ldConfig(ld), p: ld.bind_password })

  const obValid = obCheck?.ok === true && obCheck.key === obKey
  const pgValid = pgCheck?.ok === true && pgCheck.key === pgKey && (pgCheck.result?.probe.can_create || pgCheck.result?.probe.has_state) === true
  const pgNeedsReuse = pgValid && pgCheck?.result?.probe.has_state === true
  const ldValid = !ld.enabled || (ldCheck?.ok === true && ldCheck.key === ldKey)
  const rules = passwordRules(admin)
  const adminValid = USERNAME.test(admin.username.trim()) && Object.values(rules).every(Boolean)

  const canNext: Record<StepId, boolean> = {
    code: codeOk,
    prefs: true,
    openbao: obValid,
    postgres: pgValid && (!pgNeedsReuse || pg.reuse_existing),
    ldap: ldValid,
    admin: adminValid,
    review: false,
  }

  useEffect(() => {
    heading.current?.focus()
    setError(null)
  }, [step])

  const go = (s: StepId) => {
    const i = STEPS.indexOf(s)
    setReached((r) => Math.max(r, i))
    setStep(s)
  }

  const next = () => {
    const i = STEPS.indexOf(step)
    if (canNext[step] && i < STEPS.length - 1) go(STEPS[i + 1])
  }

  const back = () => {
    const i = STEPS.indexOf(step)
    if (i > 0) setStep(STEPS[i - 1])
  }

  const run = async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true)
    setError(null)
    try {
      return await fn()
    } catch (e) {
      const target = e instanceof ApiError ? stepOf(e.code) : null
      setError({ ...errorText(t, e), step: target })
      if (e instanceof ApiError && e.code === 'invalid_setup_token') {
        setCodeOk(false)
      }
      return undefined
    } finally {
      setBusy(false)
    }
  }

  const verifyCode = (e: FormEvent) => {
    e.preventDefault()
    void run(async () => {
      await api('POST', '/api/setup/token', undefined, headers)
      setCodeOk(true)
      go('prefs')
    })
  }

  const testOb = () =>
    run(async () => {
      const key = obKey
      const r = await api<ObResult>('POST', '/api/setup/openbao/test', obBody(ob), headers)
      setObCheck({ key, ok: r.ok, result: r, error: r.error })
    })

  const testPg = () =>
    run(async () => {
      const key = pgKey
      const r = await api<PgResult>('POST', '/api/setup/postgres/test', pgBody(pg), headers)
      setPgCheck({ key, ok: r.ok, result: r, error: r.error })
    })

  const testLd = () =>
    run(async () => {
      const key = ldKey
      const r = await api<LdResult>(
        'POST',
        '/api/setup/ldap/test',
        { config: ldConfig(ld), bind_password: ld.bind_password, test_username: ld.test_username.trim(), test_password: ld.test_password },
        headers,
      )
      setLdCheck({ key, ok: r.ok, result: r, error: r.error })
    })

  const finish = () =>
    run(async () => {
      await api(
        'POST',
        '/api/setup/complete',
        {
          locale: defaults.locale,
          theme: defaults.theme,
          openbao: obBody(ob),
          postgres: pgBody(pg),
          ldap: ld.enabled ? { config: ldConfig(ld), bind_password: ld.bind_password } : { config: { enabled: false } },
          admin: { username: admin.username.trim(), name: admin.name.trim(), email: admin.email.trim(), password: admin.password },
        },
        headers,
      )
      setDone(true)
    })

  const setKind = (kind: Kind) => {
    setLd((l) => {
      const old = DEFAULTS[l.kind]
      const fresh = DEFAULTS[kind]
      return {
        ...l,
        kind,
        user_filter: l.user_filter === old.user_filter || !l.user_filter ? fresh.user_filter : l.user_filter,
        username_attr: l.username_attr === old.username_attr || !l.username_attr ? fresh.username_attr : l.username_attr,
        name_attr: l.name_attr === old.name_attr || !l.name_attr ? fresh.name_attr : l.name_attr,
        email_attr: l.email_attr === old.email_attr || !l.email_attr ? fresh.email_attr : l.email_attr,
      }
    })
  }

  if (done) {
    return (
      <div className="center-page">
        <div className="card panel" style={{ width: 'min(520px, 100%)' }}>
          <CheckCircle2 size={40} color="var(--ok)" aria-hidden />
          <h1>{t('done.title')}</h1>
          <p className="muted">{t('done.text', { user: admin.username.trim() })}</p>
          <Button variant="primary" onClick={() => window.location.assign('/')}>
            {t('done.signin')}
          </Button>
        </div>
      </div>
    )
  }

  const current = STEPS.indexOf(step)
  const errorBanner = error && (
    <Banner kind="error" title={error.message}>
      {error.detail && <div>{error.detail}</div>}
      {error.step && error.step !== step && (
        <Button variant="ghost" onClick={() => setStep(error.step!)} style={{ marginTop: 8, paddingLeft: 0 }}>
          {t('rv.goto')}: {t(`step.${error.step}`)}
        </Button>
      )}
    </Banner>
  )

  return (
    <div className="wizard">
      <header className="wizard-top">
        <Brand subtitle={`${t('title')} · ${meta.version}`} />
        <Preferences persist={false} />
      </header>
      <div className="wizard-body">
        <nav aria-label={t('title')}>
          <ol className="steps">
            {STEPS.map((s, i) => {
              const isDone = i < current || (i <= reached && canNext[s] && s !== step)
              return (
                <li key={s}>
                  <button
                    type="button"
                    className={`step ${s === step ? 'current' : ''} ${isDone ? 'done' : ''}`}
                    disabled={i > reached || (!codeOk && s !== 'code')}
                    aria-current={s === step ? 'step' : undefined}
                    onClick={() => setStep(s)}
                  >
                    <span className="num">{isDone ? <Check size={14} /> : i + 1}</span>
                    <span className="step-label">
                      <span>{t(`step.${s}`)}</span>
                      <small>{t(`step.${s}.sub`)}</small>
                    </span>
                  </button>
                </li>
              )
            })}
          </ol>
        </nav>

        <main className="card panel">
          {step === 'code' && (
            <form className="stack" onSubmit={verifyCode} noValidate>
              <Head refEl={heading} title={t('code.title')} text={t('code.text')} />
              <div className="section">
                <div className="section-title">{t('code.where')}</div>
                <Rows
                  rows={[
                    [t('code.logs'), <code key="l">docker compose logs umbrella | grep setup_code</code>],
                    [t('code.file'), <code key="f">/data/setup-token</code>],
                  ]}
                />
              </div>
              <Field label={t('code.label')}>
                {(id) => <Input id={id} value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" spellCheck={false} autoFocus />}
              </Field>
              {errorBanner}
              <div className="panel-foot">
                <span />
                <Button variant="primary" type="submit" busy={busy} disabled={!code.trim()}>
                  {t('code.verify')}
                </Button>
              </div>
            </form>
          )}

          {step === 'prefs' && (
            <div className="stack">
              <Head refEl={heading} title={t('prefs.title')} text={t('prefs.text')} />
              <Field label={t('prefs.locale')}>
                {() => (
                  <div className="choices" role="radiogroup" aria-label={t('prefs.locale')}>
                    {(['en', 'ru'] as Locale[]).map((l) => (
                      <button
                        key={l}
                        type="button"
                        role="radio"
                        aria-checked={defaults.locale === l}
                        className="choice"
                        onClick={() => {
                          setDefaults((d) => ({ ...d, locale: l }))
                          setLocale(l, false)
                        }}
                      >
                        {defaults.locale === l ? <CheckCircle2 size={18} color="var(--accent)" /> : <Circle size={18} color="var(--muted)" />}
                        <span className="choice-title">{t(`lang.${l}`)}</span>
                      </button>
                    ))}
                  </div>
                )}
              </Field>
              <Field label={t('prefs.theme')}>
                {() => (
                  <div className="choices" role="radiogroup" aria-label={t('prefs.theme')}>
                    {(['light', 'dark'] as Theme[]).map((v) => (
                      <button
                        key={v}
                        type="button"
                        role="radio"
                        aria-checked={defaults.theme === v}
                        className="choice"
                        onClick={() => {
                          setDefaults((d) => ({ ...d, theme: v }))
                          setTheme(v, false)
                        }}
                      >
                        <span className={`swatch swatch-${v}`} aria-hidden />
                        <span>
                          <span className="choice-title">{t(`theme.${v}`)}</span>
                          <br />
                          <span className="hint">{t(`prefs.${v}.desc`)}</span>
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </Field>
              <Foot back={back} next={next} canNext t={t} />
            </div>
          )}

          {step === 'openbao' && (
            <div className="stack">
              <Head refEl={heading} title={t('ob.title')} text={t('ob.text')} />
              <div className="grid-3">
                <Field label={t('ob.addr')} hint={t('ob.addr.hint')}>
                  {(id) => <Input id={id} value={ob.addr} placeholder="https://openbao:8200" onChange={(e) => setOb({ ...ob, addr: e.target.value })} spellCheck={false} />}
                </Field>
                <Field label={t('ob.mount')} hint={t('ob.mount.hint')}>
                  {(id) => <Input id={id} value={ob.mount} onChange={(e) => setOb({ ...ob, mount: e.target.value })} spellCheck={false} />}
                </Field>
              </div>
              <Field label={t('ob.namespace')} optional={t('optional')}>
                {(id) => <Input id={id} value={ob.namespace} onChange={(e) => setOb({ ...ob, namespace: e.target.value })} spellCheck={false} />}
              </Field>
              <Field label={t('ob.auth')}>
                {() => (
                  <div className="choices" role="radiogroup" aria-label={t('ob.auth')}>
                    {(['approle', 'token'] as const).map((a) => (
                      <button key={a} type="button" role="radio" aria-checked={ob.auth === a} className="choice" onClick={() => setOb({ ...ob, auth: a })}>
                        {ob.auth === a ? <CheckCircle2 size={18} color="var(--accent)" /> : <Circle size={18} color="var(--muted)" />}
                        <span className="choice-title">{t(`ob.auth.${a}`)}</span>
                      </button>
                    ))}
                  </div>
                )}
              </Field>
              {ob.auth === 'token' ? (
                <Field label={t('ob.token')}>
                  {(id) => <Password id={id} value={ob.token} onChange={(e) => setOb({ ...ob, token: e.target.value })} autoComplete="off" />}
                </Field>
              ) : (
                <>
                  <div className="grid-2">
                    <Field label={t('ob.roleId')}>
                      {(id) => <Input id={id} value={ob.role_id} onChange={(e) => setOb({ ...ob, role_id: e.target.value })} autoComplete="off" spellCheck={false} />}
                    </Field>
                    <Field label={t('ob.secretId')}>
                      {(id) => <Password id={id} value={ob.secret_id} onChange={(e) => setOb({ ...ob, secret_id: e.target.value })} autoComplete="off" />}
                    </Field>
                  </div>
                  <Field label={t('ob.approlePath')}>
                    {(id) => <Input id={id} value={ob.approle_path} onChange={(e) => setOb({ ...ob, approle_path: e.target.value })} spellCheck={false} />}
                  </Field>
                </>
              )}
              <div className="section">
                <div className="section-title">{t('ob.tls')}</div>
                <Field label={t('ob.ca')} hint={t('ob.ca.hint')} optional={t('optional')}>
                  {(id) => <Textarea id={id} value={ob.ca_cert} onChange={(e) => setOb({ ...ob, ca_cert: e.target.value })} placeholder="-----BEGIN CERTIFICATE-----" spellCheck={false} />}
                </Field>
                <Switch checked={ob.skip_verify} onChange={(v) => setOb({ ...ob, skip_verify: v })} label={t('ob.skip')} hint={t('ob.skip.hint')} />
              </div>
              <CheckResult
                check={obCheck}
                currentKey={obKey}
                t={t}
                ok={(r) => (
                  <Banner kind="ok" title={t('ob.ok')}>
                    <p>{t('ob.ok.text', { version: r.status.version ?? '?', mount: ob.mount.trim() || 'umbrella' })}</p>
                    <Rows rows={[[t('ob.policies'), r.status.policies?.join(', ')], [t('ob.expires'), formatDate(r.status.token_expires, locale)]]} />
                  </Banner>
                )}
                failTitle={t('ob.fail')}
                failExtra={t('ob.policy.hint', { mount: ob.mount.trim() || 'umbrella' })}
              />
              {errorBanner}
              <Foot back={back} next={next} canNext={canNext.openbao} t={t} check={testOb} busy={busy} checkDisabled={!ob.addr.trim()} />
            </div>
          )}

          {step === 'postgres' && (
            <div className="stack">
              <Head refEl={heading} title={t('pg.title')} text={t('pg.text')} />
              <div className="grid-3">
                <Field label={t('pg.host')}>
                  {(id) => <Input id={id} value={pg.host} placeholder="postgres.example.com" onChange={(e) => setPg({ ...pg, host: e.target.value })} spellCheck={false} />}
                </Field>
                <Field label={t('pg.port')}>
                  {(id) => <Input id={id} value={pg.port} inputMode="numeric" onChange={(e) => setPg({ ...pg, port: e.target.value.replace(/\D/g, '') })} />}
                </Field>
              </div>
              <div className="grid-2">
                <Field label={t('pg.database')}>
                  {(id) => <Input id={id} value={pg.database} onChange={(e) => setPg({ ...pg, database: e.target.value })} spellCheck={false} />}
                </Field>
                <Field label={t('pg.sslmode')}>
                  {(id) => (
                    <Select id={id} value={pg.sslmode} onChange={(e) => setPg({ ...pg, sslmode: e.target.value })}>
                      {['disable', 'prefer', 'require', 'verify-ca', 'verify-full'].map((m) => (
                        <option key={m} value={m}>
                          {m}
                        </option>
                      ))}
                    </Select>
                  )}
                </Field>
              </div>
              <div className="grid-2">
                <Field label={t('pg.user')}>
                  {(id) => <Input id={id} value={pg.user} onChange={(e) => setPg({ ...pg, user: e.target.value })} autoComplete="off" spellCheck={false} />}
                </Field>
                <Field label={t('pg.password')}>
                  {(id) => <Password id={id} value={pg.password} onChange={(e) => setPg({ ...pg, password: e.target.value })} autoComplete="new-password" />}
                </Field>
              </div>
              <CheckResult
                check={pgCheck}
                currentKey={pgKey}
                t={t}
                ok={(r) => (
                  <>
                    <Banner kind={r.probe.can_create || r.probe.has_state ? 'ok' : 'error'} title={r.probe.can_create || r.probe.has_state ? t('pg.ok') : t('pg.fail')}>
                      {t('pg.ok.text', { version: r.probe.version, database: r.probe.database, user: r.probe.user })}
                      {!r.probe.can_create && !r.probe.has_state && <p>{t('pg.nocreate')}</p>}
                    </Banner>
                    {r.probe.has_state && (
                      <Banner kind="warn" title={t('pg.state', { at: formatDate(r.probe.saved_at, locale) })}>
                        <p>{t('pg.state.text')}</p>
                        <div style={{ marginTop: 10 }}>
                          <Switch checked={pg.reuse_existing} onChange={(v) => setPg({ ...pg, reuse_existing: v })} label={t('pg.reuse')} />
                        </div>
                      </Banner>
                    )}
                  </>
                )}
                failTitle={t('pg.fail')}
              />
              {errorBanner}
              <Foot back={back} next={next} canNext={canNext.postgres} t={t} check={testPg} busy={busy} checkDisabled={!pg.host.trim() || !pg.database.trim() || !pg.user.trim()} />
            </div>
          )}

          {step === 'ldap' && (
            <div className="stack">
              <Head refEl={heading} title={t('ld.title')} text={t('ld.text')} />
              <Switch checked={ld.enabled} onChange={(v) => setLd({ ...ld, enabled: v })} label={t('ld.enable')} hint={t('ld.enable.hint')} />
              {ld.enabled ? (
                <>
                  <Field label={t('ld.kind')}>
                    {() => (
                      <div className="choices" role="radiogroup" aria-label={t('ld.kind')}>
                        {(['ad', 'openldap'] as Kind[]).map((k) => (
                          <button key={k} type="button" role="radio" aria-checked={ld.kind === k} className="choice" onClick={() => setKind(k)}>
                            {ld.kind === k ? <CheckCircle2 size={18} color="var(--accent)" /> : <Circle size={18} color="var(--muted)" />}
                            <span className="choice-title">{t(`ld.kind.${k}`)}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </Field>
                  <Field label={t('ld.url')} hint={t('ld.url.hint')}>
                    {(id) => <Input id={id} value={ld.url} placeholder="ldaps://dc1.corp.example:636" onChange={(e) => setLd({ ...ld, url: e.target.value })} spellCheck={false} />}
                  </Field>
                  {ld.url.trim().toLowerCase().startsWith('ldap://') && <Switch checked={ld.start_tls} onChange={(v) => setLd({ ...ld, start_tls: v })} label={t('ld.starttls')} hint={t('ld.starttls.hint')} />}
                  <div className="grid-2">
                    <Field label={t('ld.bindDn')} hint={t(`ld.bindDn.hint.${ld.kind}`)}>
                      {(id) => <Input id={id} value={ld.bind_dn} onChange={(e) => setLd({ ...ld, bind_dn: e.target.value })} autoComplete="off" spellCheck={false} />}
                    </Field>
                    <Field label={t('ld.bindPassword')}>
                      {(id) => <Password id={id} value={ld.bind_password} onChange={(e) => setLd({ ...ld, bind_password: e.target.value })} autoComplete="new-password" />}
                    </Field>
                  </div>
                  <Field label={t('ld.baseDn')} hint={t('ld.baseDn.hint')}>
                    {(id) => <Input id={id} value={ld.base_dn} onChange={(e) => setLd({ ...ld, base_dn: e.target.value })} spellCheck={false} />}
                  </Field>
                  <Field label={t('ld.filter')} hint={t('ld.filter.hint')}>
                    {(id) => <Input id={id} value={ld.user_filter} onChange={(e) => setLd({ ...ld, user_filter: e.target.value })} spellCheck={false} />}
                  </Field>
                  <Field label={t('ld.adminGroup')} hint={t('ld.adminGroup.hint')} optional={t('optional')}>
                    {(id) => <Input id={id} value={ld.admin_group_dn} onChange={(e) => setLd({ ...ld, admin_group_dn: e.target.value })} spellCheck={false} />}
                  </Field>
                  <div className="section">
                    <div className="section-title">{t('ld.attrs')}</div>
                    <div className="grid-2">
                      <Field label={t('ld.attr.username')}>
                        {(id) => <Input id={id} value={ld.username_attr} onChange={(e) => setLd({ ...ld, username_attr: e.target.value })} spellCheck={false} />}
                      </Field>
                      <Field label={t('ld.attr.name')}>
                        {(id) => <Input id={id} value={ld.name_attr} onChange={(e) => setLd({ ...ld, name_attr: e.target.value })} spellCheck={false} />}
                      </Field>
                    </div>
                    <Field label={t('ld.attr.email')}>
                      {(id) => <Input id={id} value={ld.email_attr} onChange={(e) => setLd({ ...ld, email_attr: e.target.value })} spellCheck={false} />}
                    </Field>
                  </div>
                  <div className="section">
                    <div className="section-title">{t('ob.tls')}</div>
                    <Field label={t('ob.ca')} hint={t('ob.ca.hint')} optional={t('optional')}>
                      {(id) => <Textarea id={id} value={ld.ca_cert} onChange={(e) => setLd({ ...ld, ca_cert: e.target.value })} placeholder="-----BEGIN CERTIFICATE-----" spellCheck={false} />}
                    </Field>
                    <Switch checked={ld.skip_verify} onChange={(v) => setLd({ ...ld, skip_verify: v })} label={t('ob.skip')} hint={t('ob.skip.hint')} />
                  </div>
                  <div className="section">
                    <div className="section-title">{t('ld.testUser')}</div>
                    <p className="hint">{t('ld.testUser.hint')}</p>
                    <div className="grid-2">
                      <Field label={t('ld.testUsername')} optional={t('optional')}>
                        {(id) => <Input id={id} value={ld.test_username} onChange={(e) => setLd({ ...ld, test_username: e.target.value })} autoComplete="off" spellCheck={false} />}
                      </Field>
                      <Field label={t('ld.testPassword')} optional={t('optional')}>
                        {(id) => <Password id={id} value={ld.test_password} onChange={(e) => setLd({ ...ld, test_password: e.target.value })} autoComplete="off" />}
                      </Field>
                    </div>
                  </div>
                  <CheckResult
                    check={ldCheck}
                    currentKey={ldKey}
                    t={t}
                    ok={(r) => (
                      <Banner kind="ok" title={t('ld.ok')}>
                        <p>
                          {t('ld.ok.text')} {r.probe.admin_group && t('ld.ok.group')}
                        </p>
                        {r.probe.user && (
                          <p>
                            {t('ld.user', { name: r.probe.user.name, dn: r.probe.user.dn })} {r.probe.user_authenticated && t('ld.user.auth')}{' '}
                            {ld.admin_group_dn.trim() && (r.probe.user.admin ? t('ld.user.admin') : t('ld.user.notAdmin'))}
                          </p>
                        )}
                      </Banner>
                    )}
                    failTitle={t('ld.fail')}
                  />
                </>
              ) : (
                <Banner kind="info" title={t('ld.off')} />
              )}
              {errorBanner}
              <Foot
                back={back}
                next={next}
                canNext={canNext.ldap}
                t={t}
                check={ld.enabled ? testLd : undefined}
                busy={busy}
                checkDisabled={!ld.url.trim() || !ld.bind_dn.trim() || !ld.bind_password || !ld.base_dn.trim()}
              />
            </div>
          )}

          {step === 'admin' && (
            <form
              className="stack"
              noValidate
              onSubmit={(e) => {
                e.preventDefault()
                next()
              }}
            >
              <Head refEl={heading} title={t('ad.title')} text={t('ad.text')} />
              <div className="grid-2">
                <Field label={t('ad.username')} hint={admin.username && !USERNAME.test(admin.username.trim()) ? t('ad.username.bad') : undefined}>
                  {(id) => <Input id={id} value={admin.username} onChange={(e) => setAdmin({ ...admin, username: e.target.value })} autoComplete="username" spellCheck={false} />}
                </Field>
                <Field label={t('ad.name')} optional={t('optional')}>
                  {(id) => <Input id={id} value={admin.name} onChange={(e) => setAdmin({ ...admin, name: e.target.value })} autoComplete="name" />}
                </Field>
              </div>
              <Field label={t('ad.email')} optional={t('optional')}>
                {(id) => <Input id={id} type="email" value={admin.email} onChange={(e) => setAdmin({ ...admin, email: e.target.value })} autoComplete="email" />}
              </Field>
              <div className="grid-2">
                <Field label={t('ad.password')}>
                  {(id) => <Password id={id} value={admin.password} onChange={(e) => setAdmin({ ...admin, password: e.target.value })} autoComplete="new-password" />}
                </Field>
                <Field label={t('ad.confirm')}>
                  {(id) => <Password id={id} value={admin.confirm} onChange={(e) => setAdmin({ ...admin, confirm: e.target.value })} autoComplete="new-password" />}
                </Field>
              </div>
              <ul className="stack" style={{ gap: 6, listStyle: 'none', padding: 0, margin: 0 }}>
                {(['length', 'mix', 'user', 'match'] as const).map((r) => (
                  <li key={r} className="row" style={{ gap: 8, color: rules[r] ? 'var(--ok)' : 'var(--muted)' }}>
                    {rules[r] ? <CheckCircle2 size={16} /> : <Circle size={16} />}
                    {t(`ad.rule.${r}`)}
                  </li>
                ))}
              </ul>
              <Foot back={back} next={next} canNext={canNext.admin} t={t} submit />
            </form>
          )}

          {step === 'review' && (
            <div className="stack">
              <Head refEl={heading} title={t('rv.title')} text={t('rv.text')} />
              <div className="summary">
                <Summary title={t('step.prefs')} edit={() => setStep('prefs')} t={t} rows={[[t('rv.locale'), t(`lang.${defaults.locale}`)], [t('rv.theme'), t(`theme.${defaults.theme}`)]]} />
                <Summary
                  title="OpenBao"
                  edit={() => setStep('openbao')}
                  t={t}
                  rows={[
                    [t('rv.address'), <code key="a">{ob.addr.trim()}</code>],
                    [t('ob.mount'), ob.mount.trim()],
                    [t('rv.auth'), t(`ob.auth.${ob.auth}`)],
                  ]}
                />
                <Summary
                  title="PostgreSQL"
                  edit={() => setStep('postgres')}
                  t={t}
                  rows={[
                    [t('rv.database'), <code key="d">{`${pg.user.trim()}@${pg.host.trim()}:${pg.port}/${pg.database.trim()}`}</code>],
                    [t('pg.sslmode'), pg.sslmode],
                    ...(pgNeedsReuse ? ([[t('rv.reuse'), t('yes')]] as [ReactNode, ReactNode][]) : []),
                  ]}
                />
                <Summary
                  title="LDAP / AD"
                  edit={() => setStep('ldap')}
                  t={t}
                  rows={
                    ld.enabled
                      ? [
                          [t('rv.ldap'), `${t(`ld.kind.${ld.kind}`)} · ${ld.url.trim()}`],
                          [t('ld.baseDn'), ld.base_dn.trim()],
                          [t('rv.adminGroup'), ld.admin_group_dn.trim()],
                        ]
                      : [[t('rv.ldap'), t('rv.ldap.off')]]
                  }
                />
                <Summary
                  title={t('step.admin')}
                  edit={() => setStep('admin')}
                  t={t}
                  rows={[
                    [t('rv.login'), admin.username.trim()],
                    [t('rv.name'), admin.name.trim() || admin.username.trim()],
                    [t('ad.email'), admin.email.trim()],
                  ]}
                />
              </div>
              {error && (
                <Banner kind="error" title={`${t('rv.failed')}: ${error.message}`}>
                  {error.detail && <div>{error.detail}</div>}
                  {error.step && (
                    <Button variant="ghost" onClick={() => setStep(error.step!)} style={{ marginTop: 8, paddingLeft: 0 }}>
                      {t('rv.goto')}: {t(`step.${error.step}`)}
                    </Button>
                  )}
                </Banner>
              )}
              <div className="panel-foot">
                <Button variant="ghost" onClick={back}>
                  {t('back')}
                </Button>
                <Button variant="primary" onClick={finish} busy={busy} disabled={!(obValid && pgValid && (!pgNeedsReuse || pg.reuse_existing) && ldValid && adminValid)}>
                  {busy ? t('rv.finishing') : t('rv.finish')}
                </Button>
              </div>
            </div>
          )}
        </main>
      </div>
    </div>
  )
}

function Head({ title, text, refEl }: { title: string; text: string; refEl: RefObject<HTMLHeadingElement> }) {
  return (
    <div className="panel-head">
      <h1 ref={refEl} tabIndex={-1} style={{ outline: 'none' }}>
        {title}
      </h1>
      <p className="muted">{text}</p>
    </div>
  )
}

function Foot({
  back,
  next,
  canNext,
  t,
  check,
  busy,
  checkDisabled,
  submit,
}: {
  back: () => void
  next: () => void
  canNext: boolean
  t: (k: string) => string
  check?: () => void
  busy?: boolean
  checkDisabled?: boolean
  submit?: boolean
}) {
  return (
    <div className="panel-foot">
      <Button variant="ghost" onClick={back}>
        {t('back')}
      </Button>
      <div className="row">
        {check && (
          <Button onClick={check} busy={busy} disabled={checkDisabled}>
            {busy ? t('checking') : t('check')}
          </Button>
        )}
        <Button variant="primary" type={submit ? 'submit' : 'button'} onClick={submit ? undefined : next} disabled={!canNext}>
          {t('next')}
        </Button>
      </div>
    </div>
  )
}

function CheckResult<T>({
  check,
  currentKey,
  t,
  ok,
  failTitle,
  failExtra,
}: {
  check: Check<T>
  currentKey: string
  t: (k: string) => string
  ok: (r: T) => ReactNode
  failTitle: string
  failExtra?: string
}) {
  const latest = check
  if (!latest) return null
  if (latest.key !== currentKey) return <Banner kind="info" title={t('changed')} />
  if (latest.ok && latest.result) return <>{ok(latest.result)}</>
  return (
    <Banner kind="error" title={failTitle}>
      {latest.error && <div>{latest.error}</div>}
      {failExtra && <div className="hint" style={{ marginTop: 6 }}>{failExtra}</div>}
    </Banner>
  )
}

function Summary({ title, rows, edit, t }: { title: string; rows: [ReactNode, ReactNode][]; edit: () => void; t: (k: string) => string }) {
  return (
    <section className="section">
      <header>
        <h3>{title}</h3>
        <Button variant="ghost" onClick={edit}>
          {t('rv.edit')}
        </Button>
      </header>
      <Rows rows={rows} />
    </section>
  )
}
