import { useEffect, useRef, useState, type ComponentProps, type FormEvent, type ReactNode, type RefObject } from 'react'
import { Check, CheckCircle2, Circle } from 'lucide-react'
import { AnimatePresence, motion, MotionConfig, type Variants } from 'motion/react'
import { api, ApiError, type Locale, type Meta, type Theme } from '../api'
import { errorText, useLocale, useT } from '../i18n'
import { useTheme } from '../theme'
import {
  Banner,
  Brand,
  browserTimezone,
  Button,
  Field,
  formatDate,
  Input,
  Password,
  Preferences,
  Rows,
  Select,
  spring,
  Switch,
  Textarea,
  TimezoneSelect,
  timezones,
  zoneLabel,
} from '../ui'
import { strings } from './strings'
import { PolicyChecklist } from '../PolicyChecklist'
import { checkPassword, defaultPolicy, policyError, policyStrings, ruleText, rules as policyRules, type Letters, type PasswordPolicy } from '../policy'
import { originOf } from '../fx'

const TOKEN_HEADER = 'X-Setup-Token'

type StepId = 'code' | 'prefs' | 'openbao' | 'postgres' | 'ldap' | 'policy' | 'admin' | 'review'
const STEPS: StepId[] = ['code', 'prefs', 'openbao', 'postgres', 'ldap', 'policy', 'admin', 'review']

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
  first_name_attr: string
  last_name_attr: string
  middle_name_attr: string
  title_attr: string
  department_attr: string
  manager_attr: string
  photo_attr: string
  admin_group_dn: string
  test_username: string
  test_password: string
}

type Admin = {
  username: string
  last_name: string
  first_name: string
  middle_name: string
  title: string
  department: string
  manager: string
  email: string
  password: string
  confirm: string
}

type Check<T> = { key: string; ok: boolean; result?: T; error?: string } | null

type ObResult = { ok: boolean; error?: string; write_ok: boolean; status: { version?: string; policies?: string[]; token_expires?: string; mount?: string } }
type PgResult = {
  ok: boolean
  error?: string
  probe: { version: string; database: string; user: string; can_create: boolean; has_state: boolean; saved_at?: string }
}
type LdResult = {
  ok: boolean
  error?: string
  probe: {
    server: string
    tls: string
    base_dn: string
    admin_group: boolean
    user_authenticated: boolean
    user?: { dn: string; name: string; admin: boolean; title?: string; department?: string; manager?: string; email?: string; has_photo?: boolean }
  }
}

type AttrKey =
  | 'user_filter'
  | 'username_attr'
  | 'name_attr'
  | 'email_attr'
  | 'first_name_attr'
  | 'last_name_attr'
  | 'middle_name_attr'
  | 'title_attr'
  | 'department_attr'
  | 'manager_attr'
  | 'photo_attr'

const ATTRS: AttrKey[] = [
  'username_attr',
  'name_attr',
  'last_name_attr',
  'first_name_attr',
  'middle_name_attr',
  'title_attr',
  'department_attr',
  'manager_attr',
  'email_attr',
  'photo_attr',
]

const DEFAULTS: Record<Kind, Record<AttrKey, string>> = {
  ad: {
    user_filter: '(&(objectCategory=person)(objectClass=user)(sAMAccountName={username}))',
    username_attr: 'sAMAccountName',
    name_attr: 'displayName',
    email_attr: 'mail',
    first_name_attr: 'givenName',
    last_name_attr: 'sn',
    middle_name_attr: 'middleName',
    title_attr: 'title',
    department_attr: 'department',
    manager_attr: 'manager',
    photo_attr: 'thumbnailPhoto',
  },
  openldap: {
    user_filter: '(&(objectClass=inetOrgPerson)(uid={username}))',
    username_attr: 'uid',
    name_attr: 'cn',
    email_attr: 'mail',
    first_name_attr: 'givenName',
    last_name_attr: 'sn',
    middle_name_attr: '',
    title_attr: 'title',
    department_attr: 'departmentNumber',
    manager_attr: 'manager',
    photo_attr: 'jpegPhoto',
  },
}

const dict = { en: { ...strings.en, ...policyStrings.en }, ru: { ...strings.ru, ...policyStrings.ru } }

const stepVariants: Variants = {
  enter: (dir: 'fwd' | 'back') => ({ opacity: 0, x: dir === 'fwd' ? 36 : -36 }),
  center: { opacity: 1, x: 0 },
  exit: (dir: 'fwd' | 'back') => ({ opacity: 0, x: dir === 'fwd' ? -36 : 36 }),
}

function Pop({ children }: { children: ReactNode }) {
  return (
    <motion.span
      className="pop-icon"
      initial={{ scale: 0.3, opacity: 0 }}
      animate={{ scale: 1, opacity: 1 }}
      transition={{ type: 'spring', stiffness: 520, damping: 18 }}
    >
      {children}
    </motion.span>
  )
}

function Choice(props: ComponentProps<typeof motion.button>) {
  return <motion.button type="button" role="radio" className="choice" whileHover={{ y: -2 }} whileTap={{ scale: 0.97 }} transition={spring} {...props} />
}

const LOOPBACK = /^(localhost|127(\.\d+){3}|\[?::1\]?|0\.0\.0\.0)$/i

function hostOf(addr: string) {
  try {
    return new URL(addr.trim()).hostname
  } catch {
    return ''
  }
}

const USERNAME = /^[A-Za-z0-9][A-Za-z0-9._@-]{1,63}$/

function stepOf(code: string): StepId | null {
  if (code === 'invalid_setup_token') return 'code'
  if (code.startsWith('openbao_')) return 'openbao'
  if (code.startsWith('postgres_')) return 'postgres'
  if (code.startsWith('ldap_')) return 'ldap'
  if (['weak_password', 'invalid_username', 'invalid_email', 'invalid_name'].includes(code)) return 'admin'
  if (code === 'invalid_password_policy') return 'policy'
  if (code === 'invalid_locale' || code === 'invalid_theme' || code === 'invalid_timezone') return 'prefs'
  return null
}

function obBody(o: OpenBao) {
  const base = {
    addr: o.addr.trim(),
    mount: o.mount.trim(),
    namespace: o.namespace.trim(),
    auth: o.auth,
    ca_cert: o.ca_cert.trim(),
    skip_verify: o.skip_verify,
  }
  return o.auth === 'token' ? { ...base, token: o.token } : { ...base, role_id: o.role_id, secret_id: o.secret_id, approle_path: o.approle_path.trim() }
}

function pgBody(p: Postgres) {
  return {
    host: p.host.trim(),
    port: Number(p.port) || 0,
    database: p.database.trim(),
    user: p.user.trim(),
    password: p.password,
    sslmode: p.sslmode,
    reuse_existing: p.reuse_existing,
  }
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
    first_name_attr: l.first_name_attr.trim(),
    last_name_attr: l.last_name_attr.trim(),
    middle_name_attr: l.middle_name_attr.trim(),
    title_attr: l.title_attr.trim(),
    department_attr: l.department_attr.trim(),
    manager_attr: l.manager_attr.trim(),
    photo_attr: l.photo_attr.trim(),
    admin_group_dn: l.admin_group_dn.trim(),
  }
}

export default function SetupApp({ meta, onReady }: { meta: Meta; onReady: () => void }) {
  const t = useT(dict)
  const { locale, setLocale } = useLocale()
  const { theme, setTheme } = useTheme()

  const [step, setStepRaw] = useState<StepId>('code')
  const [dir, setDir] = useState<'fwd' | 'back'>('fwd')
  const setStep = (s: StepId) => {
    setDir(STEPS.indexOf(s) >= STEPS.indexOf(step) ? 'fwd' : 'back')
    setStepRaw(s)
  }
  const [reached, setReached] = useState(0)
  const [code, setCode] = useState('')
  const [codeOk, setCodeOk] = useState(false)
  const [defaults, setDefaults] = useState<{ locale: Locale; theme: Theme; timezone: string }>(() => {
    const tz = browserTimezone()
    return { locale, theme, timezone: timezones().some((z) => z.id === tz) ? tz : 'UTC' }
  })
  const [ob, setOb] = useState<OpenBao>({
    addr: '',
    mount: 'umbrella',
    namespace: '',
    auth: 'approle',
    token: '',
    role_id: '',
    secret_id: '',
    approle_path: 'approle',
    ca_cert: '',
    skip_verify: false,
  })
  const [pg, setPg] = useState<Postgres>({
    host: '',
    port: '5432',
    database: 'umbrella',
    user: 'umbrella',
    password: '',
    sslmode: 'prefer',
    reuse_existing: false,
  })
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
  const [admin, setAdmin] = useState<Admin>({
    username: 'admin',
    last_name: '',
    first_name: '',
    middle_name: '',
    title: '',
    department: '',
    manager: '',
    email: '',
    password: '',
    confirm: '',
  })
  const [policy, setPolicy] = useState<PasswordPolicy>(defaultPolicy)
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
  const policyProblem = policyError(policy)
  const violations = checkPassword(admin.password, admin.username.trim(), policy)
  const passwordsMatch = admin.password !== '' && admin.password === admin.confirm
  const emailOk = admin.email.trim() === '' || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(admin.email.trim())
  const adminValid = USERNAME.test(admin.username.trim()) && violations.length === 0 && passwordsMatch && emailOk

  const canNext: Record<StepId, boolean> = {
    code: codeOk,
    prefs: true,
    openbao: obValid,
    postgres: pgValid && (!pgNeedsReuse || pg.reuse_existing),
    ldap: ldValid,
    policy: policyProblem === null,
    admin: adminValid && policyProblem === null,
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
          timezone: defaults.timezone,
          password_policy: policy,
          openbao: obBody(ob),
          postgres: pgBody(pg),
          ldap: ld.enabled ? { config: ldConfig(ld), bind_password: ld.bind_password } : { config: { enabled: false } },
          admin: {
            username: admin.username.trim(),
            last_name: admin.last_name.trim(),
            first_name: admin.first_name.trim(),
            middle_name: admin.middle_name.trim(),
            title: admin.title.trim(),
            department: admin.department.trim(),
            manager: admin.manager.trim(),
            email: admin.email.trim(),
            password: admin.password,
          },
        },
        headers,
      )
      setDone(true)
    })

  const setKind = (kind: Kind) => {
    setLd((l) => {
      const old = DEFAULTS[l.kind]
      const fresh = DEFAULTS[kind]
      const next = { ...l, kind }
      for (const k of [...ATTRS, 'user_filter'] as AttrKey[]) {
        if (l[k] === old[k] || !l[k]) next[k] = fresh[k]
      }
      return next
    })
  }

  if (done) {
    return (
      <div className="center-page">
        <motion.div
          className="card panel"
          style={{ width: 'min(520px, 100%)' }}
          initial={{ opacity: 0, scale: 0.9, y: 12 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          transition={{ type: 'spring', stiffness: 260, damping: 22 }}
        >
          <svg className="done-mark" viewBox="0 0 52 52" width={56} height={56} aria-hidden>
            <motion.circle
              cx="26"
              cy="26"
              r="24"
              initial={{ pathLength: 0 }}
              animate={{ pathLength: 1 }}
              transition={{ duration: 0.6, delay: 0.15, ease: 'easeOut' }}
            />
            <motion.path
              d="M15 27l7 7 15-15"
              initial={{ pathLength: 0 }}
              animate={{ pathLength: 1 }}
              transition={{ duration: 0.4, delay: 0.65, ease: 'easeOut' }}
            />
          </svg>
          <h1>{t('done.title')}</h1>
          <p className="muted">{t('done.text', { user: admin.username.trim() })}</p>
          <Button variant="primary" onClick={onReady}>
            {t('done.signin')}
          </Button>
        </motion.div>
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
    <MotionConfig reducedMotion="user">
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
                      {s === step && <motion.span layoutId="step-current" className="step-highlight" transition={spring} />}
                      <span className="num">
                        {isDone ? (
                          <Pop>
                            <Check size={14} />
                          </Pop>
                        ) : (
                          i + 1
                        )}
                      </span>
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
            <div className="progress" role="progressbar" aria-valuemin={1} aria-valuemax={STEPS.length} aria-valuenow={current + 1}>
              <motion.span
                initial={false}
                animate={{ width: `${((current + 1) / STEPS.length) * 100}%` }}
                transition={{ type: 'spring', stiffness: 140, damping: 22 }}
              />
            </div>
            <AnimatePresence mode="wait" custom={dir} initial={false}>
              <motion.div
                key={step}
                className="step-view"
                custom={dir}
                variants={stepVariants}
                initial="enter"
                animate="center"
                exit="exit"
                transition={{ duration: 0.26, ease: [0.22, 1, 0.36, 1] }}
              >
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
                            <Choice
                              key={l}
                              aria-checked={defaults.locale === l}
                              onClick={() => {
                                setDefaults((d) => ({ ...d, locale: l }))
                                setLocale(l, false)
                              }}
                            >
                              {defaults.locale === l ? (
                                <Pop>
                                  <CheckCircle2 size={18} color="var(--accent)" />
                                </Pop>
                              ) : (
                                <Circle size={18} color="var(--muted)" />
                              )}
                              <span className="choice-title">{t(`lang.${l}`)}</span>
                            </Choice>
                          ))}
                        </div>
                      )}
                    </Field>
                    <Field label={t('prefs.theme')}>
                      {() => (
                        <div className="choices" role="radiogroup" aria-label={t('prefs.theme')}>
                          {(['light', 'dark'] as Theme[]).map((v) => (
                            <Choice
                              key={v}
                              aria-checked={defaults.theme === v}
                              onClick={(e) => {
                                setDefaults((d) => ({ ...d, theme: v }))
                                setTheme(v, false, originOf(e))
                              }}
                            >
                              <span className={`swatch swatch-${v}`} aria-hidden />
                              <span>
                                <span className="choice-title">{t(`theme.${v}`)}</span>
                                <br />
                                <span className="hint">{t(`prefs.${v}.desc`)}</span>
                              </span>
                            </Choice>
                          ))}
                        </div>
                      )}
                    </Field>
                    <Field label={t('prefs.timezone')} hint={t('prefs.timezone.hint')}>
                      {(id) => <TimezoneSelect id={id} value={defaults.timezone} onChange={(v) => setDefaults((d) => ({ ...d, timezone: v }))} />}
                    </Field>
                    <Foot back={back} next={next} canNext t={t} />
                  </div>
                )}

                {step === 'openbao' && (
                  <div className="stack">
                    <Head refEl={heading} title={t('ob.title')} text={t('ob.text')} />
                    <div className="grid-3">
                      <Field label={t('ob.addr')} hint={t('ob.addr.hint')}>
                        {(id) => (
                          <Input
                            id={id}
                            value={ob.addr}
                            placeholder="https://openbao:8200"
                            onChange={(e) => setOb({ ...ob, addr: e.target.value })}
                            spellCheck={false}
                          />
                        )}
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
                            <Choice key={a} aria-checked={ob.auth === a} onClick={() => setOb({ ...ob, auth: a })}>
                              {ob.auth === a ? (
                                <Pop>
                                  <CheckCircle2 size={18} color="var(--accent)" />
                                </Pop>
                              ) : (
                                <Circle size={18} color="var(--muted)" />
                              )}
                              <span className="choice-title">{t(`ob.auth.${a}`)}</span>
                            </Choice>
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
                            {(id) => (
                              <Input
                                id={id}
                                value={ob.role_id}
                                onChange={(e) => setOb({ ...ob, role_id: e.target.value })}
                                autoComplete="off"
                                spellCheck={false}
                              />
                            )}
                          </Field>
                          <Field label={t('ob.secretId')}>
                            {(id) => <Password id={id} value={ob.secret_id} onChange={(e) => setOb({ ...ob, secret_id: e.target.value })} autoComplete="off" />}
                          </Field>
                        </div>
                        <Field label={t('ob.approlePath')}>
                          {(id) => (
                            <Input id={id} value={ob.approle_path} onChange={(e) => setOb({ ...ob, approle_path: e.target.value })} spellCheck={false} />
                          )}
                        </Field>
                      </>
                    )}
                    <div className="section">
                      <div className="section-title">{t('ob.tls')}</div>
                      <Field label={t('ob.ca')} hint={t('ob.ca.hint')} optional={t('optional')}>
                        {(id) => (
                          <Textarea
                            id={id}
                            value={ob.ca_cert}
                            onChange={(e) => setOb({ ...ob, ca_cert: e.target.value })}
                            placeholder="-----BEGIN CERTIFICATE-----"
                            spellCheck={false}
                          />
                        )}
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
                          <Rows
                            rows={[
                              [t('ob.policies'), r.status.policies?.join(', ')],
                              [t('ob.expires'), formatDate(r.status.token_expires, locale)],
                            ]}
                          />
                        </Banner>
                      )}
                      failTitle={t('ob.fail')}
                      failExtra={(err) =>
                        meta.container && LOOPBACK.test(hostOf(ob.addr))
                          ? t('ob.loopback')
                          : /permission denied|403/i.test(err)
                            ? t('ob.policy.hint', { mount: ob.mount.trim() || 'umbrella' })
                            : undefined
                      }
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
                        {(id) => (
                          <Input
                            id={id}
                            value={pg.host}
                            placeholder="postgres.example.com"
                            onChange={(e) => setPg({ ...pg, host: e.target.value })}
                            spellCheck={false}
                          />
                        )}
                      </Field>
                      <Field label={t('pg.port')}>
                        {(id) => (
                          <Input id={id} value={pg.port} inputMode="numeric" onChange={(e) => setPg({ ...pg, port: e.target.value.replace(/\D/g, '') })} />
                        )}
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
                        {(id) => (
                          <Input id={id} value={pg.user} onChange={(e) => setPg({ ...pg, user: e.target.value })} autoComplete="off" spellCheck={false} />
                        )}
                      </Field>
                      <Field label={t('pg.password')}>
                        {(id) => (
                          <Password id={id} value={pg.password} onChange={(e) => setPg({ ...pg, password: e.target.value })} autoComplete="new-password" />
                        )}
                      </Field>
                    </div>
                    <CheckResult
                      check={pgCheck}
                      currentKey={pgKey}
                      t={t}
                      ok={(r) => (
                        <>
                          <Banner
                            kind={r.probe.can_create || r.probe.has_state ? 'ok' : 'error'}
                            title={r.probe.can_create || r.probe.has_state ? t('pg.ok') : t('pg.fail')}
                          >
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
                      failExtra={() => (meta.container && LOOPBACK.test(pg.host.trim()) ? t('pg.loopback') : undefined)}
                    />
                    {errorBanner}
                    <Foot
                      back={back}
                      next={next}
                      canNext={canNext.postgres}
                      t={t}
                      check={testPg}
                      busy={busy}
                      checkDisabled={!pg.host.trim() || !pg.database.trim() || !pg.user.trim()}
                    />
                  </div>
                )}

                {step === 'ldap' && (
                  <div className="stack">
                    <Head refEl={heading} title={t('ld.title')} text={t('ld.text')} />
                    <Switch checked={ld.enabled} onChange={(v) => setLd({ ...ld, enabled: v })} label={t('ld.enable')} hint={t('ld.enable.hint')} />
                    <AnimatePresence initial={false} mode="wait">
                      {ld.enabled ? (
                        <motion.div
                          key="ldap-on"
                          className="reveal-box"
                          initial={{ opacity: 0, height: 0 }}
                          animate={{ opacity: 1, height: 'auto' }}
                          exit={{ opacity: 0, height: 0 }}
                          transition={{ duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
                        >
                          <div className="stack">
                            <Field label={t('ld.kind')}>
                              {() => (
                                <div className="choices" role="radiogroup" aria-label={t('ld.kind')}>
                                  {(['ad', 'openldap'] as Kind[]).map((k) => (
                                    <Choice key={k} aria-checked={ld.kind === k} onClick={() => setKind(k)}>
                                      {ld.kind === k ? (
                                        <Pop>
                                          <CheckCircle2 size={18} color="var(--accent)" />
                                        </Pop>
                                      ) : (
                                        <Circle size={18} color="var(--muted)" />
                                      )}
                                      <span className="choice-title">{t(`ld.kind.${k}`)}</span>
                                    </Choice>
                                  ))}
                                </div>
                              )}
                            </Field>
                            <Field label={t('ld.url')} hint={t('ld.url.hint')}>
                              {(id) => (
                                <Input
                                  id={id}
                                  value={ld.url}
                                  placeholder="ldaps://dc1.corp.example:636"
                                  onChange={(e) => setLd({ ...ld, url: e.target.value })}
                                  spellCheck={false}
                                />
                              )}
                            </Field>
                            {ld.url.trim().toLowerCase().startsWith('ldap://') && (
                              <Switch
                                checked={ld.start_tls}
                                onChange={(v) => setLd({ ...ld, start_tls: v })}
                                label={t('ld.starttls')}
                                hint={t('ld.starttls.hint')}
                              />
                            )}
                            <div className="grid-2">
                              <Field label={t('ld.bindDn')} hint={t(`ld.bindDn.hint.${ld.kind}`)}>
                                {(id) => (
                                  <Input
                                    id={id}
                                    value={ld.bind_dn}
                                    onChange={(e) => setLd({ ...ld, bind_dn: e.target.value })}
                                    autoComplete="off"
                                    spellCheck={false}
                                  />
                                )}
                              </Field>
                              <Field label={t('ld.bindPassword')}>
                                {(id) => (
                                  <Password
                                    id={id}
                                    value={ld.bind_password}
                                    onChange={(e) => setLd({ ...ld, bind_password: e.target.value })}
                                    autoComplete="new-password"
                                  />
                                )}
                              </Field>
                            </div>
                            <Field label={t('ld.baseDn')} hint={t('ld.baseDn.hint')}>
                              {(id) => <Input id={id} value={ld.base_dn} onChange={(e) => setLd({ ...ld, base_dn: e.target.value })} spellCheck={false} />}
                            </Field>
                            <Field label={t('ld.filter')} hint={t('ld.filter.hint')}>
                              {(id) => (
                                <Input id={id} value={ld.user_filter} onChange={(e) => setLd({ ...ld, user_filter: e.target.value })} spellCheck={false} />
                              )}
                            </Field>
                            <Field label={t('ld.adminGroup')} hint={t('ld.adminGroup.hint')} optional={t('optional')}>
                              {(id) => (
                                <Input
                                  id={id}
                                  value={ld.admin_group_dn}
                                  onChange={(e) => setLd({ ...ld, admin_group_dn: e.target.value })}
                                  spellCheck={false}
                                />
                              )}
                            </Field>
                            <div className="section">
                              <div className="section-title">{t('ld.attrs')}</div>
                              <p className="hint">{t('ld.attrs.hint')}</p>
                              <div className="grid-2">
                                {ATTRS.map((k) => (
                                  <Field key={k} label={t(`ld.attr.${k}`)}>
                                    {(id) => <Input id={id} value={ld[k]} onChange={(e) => setLd({ ...ld, [k]: e.target.value })} spellCheck={false} />}
                                  </Field>
                                ))}
                              </div>
                            </div>
                            <div className="section">
                              <div className="section-title">{t('ob.tls')}</div>
                              <Field label={t('ob.ca')} hint={t('ob.ca.hint')} optional={t('optional')}>
                                {(id) => (
                                  <Textarea
                                    id={id}
                                    value={ld.ca_cert}
                                    onChange={(e) => setLd({ ...ld, ca_cert: e.target.value })}
                                    placeholder="-----BEGIN CERTIFICATE-----"
                                    spellCheck={false}
                                  />
                                )}
                              </Field>
                              <Switch
                                checked={ld.skip_verify}
                                onChange={(v) => setLd({ ...ld, skip_verify: v })}
                                label={t('ob.skip')}
                                hint={t('ob.skip.hint')}
                              />
                            </div>
                            <div className="section">
                              <div className="section-title">{t('ld.testUser')}</div>
                              <p className="hint">{t('ld.testUser.hint')}</p>
                              <div className="grid-2">
                                <Field label={t('ld.testUsername')} optional={t('optional')}>
                                  {(id) => (
                                    <Input
                                      id={id}
                                      value={ld.test_username}
                                      onChange={(e) => setLd({ ...ld, test_username: e.target.value })}
                                      autoComplete="off"
                                      spellCheck={false}
                                    />
                                  )}
                                </Field>
                                <Field label={t('ld.testPassword')} optional={t('optional')}>
                                  {(id) => (
                                    <Password
                                      id={id}
                                      value={ld.test_password}
                                      onChange={(e) => setLd({ ...ld, test_password: e.target.value })}
                                      autoComplete="off"
                                    />
                                  )}
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
                                  {r.probe.user && (
                                    <Rows
                                      rows={[
                                        [t('ad.title_field'), r.probe.user.title],
                                        [t('ad.department'), r.probe.user.department],
                                        [t('ad.manager'), r.probe.user.manager],
                                        [t('ad.email'), r.probe.user.email],
                                        [t('ld.photo'), r.probe.user.has_photo ? t('yes') : t('no')],
                                      ]}
                                    />
                                  )}
                                </Banner>
                              )}
                              failTitle={t('ld.fail')}
                              failExtra={(err) =>
                                /Strong Auth Required|Confidentiality Required/i.test(err)
                                  ? t('ld.hint.tls')
                                  : /certificate|x509/i.test(err)
                                    ? t('ld.hint.cert')
                                    : meta.container && LOOPBACK.test(hostOf(ld.url))
                                      ? t('ld.hint.loopback')
                                      : undefined
                              }
                            />
                          </div>
                        </motion.div>
                      ) : (
                        <motion.div key="ldap-off" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.2 }}>
                          <Banner kind="info" title={t('ld.off')} />
                        </motion.div>
                      )}
                    </AnimatePresence>
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

                {step === 'policy' && (
                  <div className="stack">
                    <Head refEl={heading} title={t('pol.title')} text={t('pol.text')} />
                    <Field label={t('pol.length')}>
                      {(id) => (
                        <div className="range-row">
                          <input
                            type="range"
                            min={8}
                            max={64}
                            value={Math.min(policy.min_length, 64)}
                            onChange={(e) => setPolicy({ ...policy, min_length: Number(e.target.value) })}
                            aria-label={t('pol.length')}
                          />
                          <Input
                            id={id}
                            className="num-input"
                            type="number"
                            min={8}
                            max={128}
                            value={policy.min_length}
                            onChange={(e) => setPolicy({ ...policy, min_length: Math.trunc(Number(e.target.value)) || 0 })}
                          />
                        </div>
                      )}
                    </Field>
                    <div className="section">
                      <Switch
                        checked={policy.require_digits}
                        onChange={(v) => setPolicy({ ...policy, require_digits: v, min_digits: v ? Math.max(policy.min_digits, 1) : policy.min_digits })}
                        label={t('pol.digits')}
                        hint={t('pol.digits.hint')}
                      />
                      <AnimatePresence initial={false}>
                        {policy.require_digits && (
                          <motion.div
                            key="digits"
                            initial={{ opacity: 0, height: 0 }}
                            animate={{ opacity: 1, height: 'auto' }}
                            exit={{ opacity: 0, height: 0 }}
                            className="reveal-box"
                          >
                            <Field label={t('pol.count')}>
                              {(id) => (
                                <Input
                                  id={id}
                                  className="num-input"
                                  type="number"
                                  min={1}
                                  max={16}
                                  value={policy.min_digits}
                                  onChange={(e) => setPolicy({ ...policy, min_digits: Math.trunc(Number(e.target.value)) || 0 })}
                                />
                              )}
                            </Field>
                          </motion.div>
                        )}
                      </AnimatePresence>
                    </div>
                    <div className="section">
                      <Switch
                        checked={policy.require_special}
                        onChange={(v) => setPolicy({ ...policy, require_special: v, min_special: v ? Math.max(policy.min_special, 1) : policy.min_special })}
                        label={t('pol.special')}
                        hint={t('pol.special.hint')}
                      />
                      <AnimatePresence initial={false}>
                        {policy.require_special && (
                          <motion.div
                            key="special"
                            initial={{ opacity: 0, height: 0 }}
                            animate={{ opacity: 1, height: 'auto' }}
                            exit={{ opacity: 0, height: 0 }}
                            className="reveal-box"
                          >
                            <Field label={t('pol.count')}>
                              {(id) => (
                                <Input
                                  id={id}
                                  className="num-input"
                                  type="number"
                                  min={1}
                                  max={16}
                                  value={policy.min_special}
                                  onChange={(e) => setPolicy({ ...policy, min_special: Math.trunc(Number(e.target.value)) || 0 })}
                                />
                              )}
                            </Field>
                          </motion.div>
                        )}
                      </AnimatePresence>
                    </div>
                    <div className="section">
                      <Switch
                        checked={policy.require_mixed_case}
                        onChange={(v) => setPolicy({ ...policy, require_mixed_case: v })}
                        label={t('pol.case')}
                        hint={t('pol.case.hint')}
                      />
                    </div>
                    <Field label={t('pol.letters')} hint={t('pol.letters.hint')}>
                      {() => (
                        <div className="choices" role="radiogroup" aria-label={t('pol.letters')}>
                          {(['latin', 'cyrillic', 'latin_cyrillic', 'any'] as Letters[]).map((l) => (
                            <Choice key={l} aria-checked={policy.letters === l} onClick={() => setPolicy({ ...policy, letters: l })}>
                              {policy.letters === l ? (
                                <Pop>
                                  <CheckCircle2 size={18} color="var(--accent)" />
                                </Pop>
                              ) : (
                                <Circle size={18} color="var(--muted)" />
                              )}
                              <span>
                                <span className="choice-title">{t(`pol.letters.${l}`)}</span>
                                <br />
                                <span className="hint">{t(`pol.letters.${l}.ex`)}</span>
                              </span>
                            </Choice>
                          ))}
                        </div>
                      )}
                    </Field>
                    {policyProblem ? (
                      <Banner kind="error" title={t(`pol.err.${policyProblem}`)} />
                    ) : (
                      <Banner kind="info" title={t('pol.summary')}>
                        <ul className="checklist" style={{ marginTop: 6, color: 'var(--text)' }}>
                          {policyRules(policy).map((r) => (
                            <li key={r}>• {ruleText(t, r, policy)}</li>
                          ))}
                        </ul>
                      </Banner>
                    )}
                    <Foot back={back} next={next} canNext={canNext.policy} t={t} />
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
                      <Field label={t('ad.last_name')}>
                        {(id) => (
                          <Input
                            id={id}
                            value={admin.last_name}
                            onChange={(e) => setAdmin({ ...admin, last_name: e.target.value })}
                            autoComplete="family-name"
                          />
                        )}
                      </Field>
                      <Field label={t('ad.first_name')}>
                        {(id) => (
                          <Input
                            id={id}
                            value={admin.first_name}
                            onChange={(e) => setAdmin({ ...admin, first_name: e.target.value })}
                            autoComplete="given-name"
                          />
                        )}
                      </Field>
                    </div>
                    <div className="grid-2">
                      <Field label={t('ad.middle_name')} optional={t('ad.if_any')}>
                        {(id) => (
                          <Input
                            id={id}
                            value={admin.middle_name}
                            onChange={(e) => setAdmin({ ...admin, middle_name: e.target.value })}
                            autoComplete="additional-name"
                          />
                        )}
                      </Field>
                      <Field label={t('ad.title_field')} optional={t('optional')}>
                        {(id) => (
                          <Input
                            id={id}
                            value={admin.title}
                            onChange={(e) => setAdmin({ ...admin, title: e.target.value })}
                            autoComplete="organization-title"
                          />
                        )}
                      </Field>
                    </div>
                    <div className="grid-2">
                      <Field label={t('ad.department')} optional={t('optional')}>
                        {(id) => <Input id={id} value={admin.department} onChange={(e) => setAdmin({ ...admin, department: e.target.value })} />}
                      </Field>
                      <Field label={t('ad.manager')} optional={t('optional')}>
                        {(id) => <Input id={id} value={admin.manager} onChange={(e) => setAdmin({ ...admin, manager: e.target.value })} />}
                      </Field>
                    </div>
                    <Field label={t('ad.email')} optional={t('optional')} hint={admin.email && !emailOk ? t('ad.email.bad') : t('ad.email.hint')}>
                      {(id) => (
                        <Input id={id} type="email" value={admin.email} onChange={(e) => setAdmin({ ...admin, email: e.target.value })} autoComplete="email" />
                      )}
                    </Field>
                    <Field label={t('ad.username')} hint={admin.username && !USERNAME.test(admin.username.trim()) ? t('ad.username.bad') : undefined}>
                      {(id) => (
                        <Input
                          id={id}
                          value={admin.username}
                          onChange={(e) => setAdmin({ ...admin, username: e.target.value })}
                          autoComplete="username"
                          spellCheck={false}
                        />
                      )}
                    </Field>
                    <div className="grid-2">
                      <Field label={t('ad.password')}>
                        {(id) => (
                          <Password
                            id={id}
                            value={admin.password}
                            onChange={(e) => setAdmin({ ...admin, password: e.target.value })}
                            autoComplete="new-password"
                          />
                        )}
                      </Field>
                      <Field label={t('ad.confirm')}>
                        {(id) => (
                          <Password
                            id={id}
                            value={admin.confirm}
                            onChange={(e) => setAdmin({ ...admin, confirm: e.target.value })}
                            autoComplete="new-password"
                          />
                        )}
                      </Field>
                    </div>
                    <PolicyChecklist policy={policy} password={admin.password} username={admin.username.trim()} confirm={admin.confirm} t={t} />
                    <Foot back={back} next={next} canNext={canNext.admin} t={t} submit />
                  </form>
                )}

                {step === 'review' && (
                  <div className="stack">
                    <Head refEl={heading} title={t('rv.title')} text={t('rv.text')} />
                    <div className="summary">
                      <Summary
                        title={t('step.prefs')}
                        edit={() => setStep('prefs')}
                        t={t}
                        rows={[
                          [t('rv.locale'), t(`lang.${defaults.locale}`)],
                          [t('rv.theme'), t(`theme.${defaults.theme}`)],
                          [t('rv.timezone'), zoneLabel(defaults.timezone)],
                        ]}
                      />
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
                        title={t('step.policy')}
                        edit={() => setStep('policy')}
                        t={t}
                        rows={policyRules(policy)
                          .filter((r) => r !== 'username')
                          .map((r) => [t(`pol.row.${r}`), ruleText(t, r, policy)] as [ReactNode, ReactNode])}
                      />
                      <Summary
                        title={t('step.admin')}
                        edit={() => setStep('admin')}
                        t={t}
                        rows={[
                          [t('rv.login'), admin.username.trim()],
                          [
                            t('rv.name'),
                            [admin.last_name, admin.first_name, admin.middle_name]
                              .map((x) => x.trim())
                              .filter(Boolean)
                              .join(' ') || admin.username.trim(),
                          ],
                          [t('ad.title_field'), admin.title.trim()],
                          [t('ad.department'), admin.department.trim()],
                          [t('ad.manager'), admin.manager.trim()],
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
                      <Button
                        variant="primary"
                        onClick={finish}
                        busy={busy}
                        disabled={!(obValid && pgValid && (!pgNeedsReuse || pg.reuse_existing) && ldValid && adminValid)}
                      >
                        {busy ? t('rv.finishing') : t('rv.finish')}
                      </Button>
                    </div>
                  </div>
                )}
              </motion.div>
            </AnimatePresence>
          </main>
        </div>
      </div>
    </MotionConfig>
  )
}

function Head({ title, text, refEl }: { title: string; text: string; refEl: RefObject<HTMLHeadingElement | null> }) {
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
  failExtra?: (error: string) => string | undefined
}) {
  const latest = check
  if (!latest) return null
  if (latest.key !== currentKey) return <Banner kind="info" title={t('changed')} />
  if (latest.ok && latest.result) return <>{ok(latest.result)}</>
  const extra = failExtra?.(latest.error ?? '')
  return (
    <Banner kind="error" title={failTitle}>
      {latest.error && <div>{latest.error}</div>}
      {extra && (
        <div className="hint" style={{ marginTop: 6 }}>
          {extra}
        </div>
      )}
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
