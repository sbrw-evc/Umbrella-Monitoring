import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { LogOut, RefreshCw } from 'lucide-react'
import { api, ApiError, setCsrf, type Meta } from '../api'
import { errorText, useLocale, useT, type Dict } from '../i18n'
import { Banner, Brand, Button, Field, formatDate, Input, Password, Preferences, Rows } from '../ui'

type User = {
  id: string
  username: string
  name: string
  email?: string
  source: 'local' | 'ldap'
  role: 'admin' | 'user'
  last_login_at?: string
  csrf?: string
}

type Status = {
  version: string
  openbao: {
    addr?: string
    mount?: string
    auth?: string
    version?: string
    sealed: boolean
    token_ok: boolean
    mount_ok: boolean
    token_expires?: string
    policies?: string[]
    error?: string
  }
  postgres: { where: string; ok: boolean; info: { version?: string; saved_at?: string }; persist: { pending: boolean; error?: string }; error?: string }
  ldap: { enabled: boolean; ok: boolean; kind?: string; url?: string; tls?: string; base_dn?: string; admin_group?: string; error?: string }
  settings: { default_theme: string; default_locale: string; setup_at?: string; setup_by?: string }
  users: Record<string, number>
}

export default function MainApp({ meta }: { meta: Meta }) {
  const [user, setUser] = useState<User | null>(null)
  const [checked, setChecked] = useState(false)
  const t = useT(strings)

  useEffect(() => {
    api<User>('GET', '/api/auth/me')
      .then((u) => {
        setCsrf(u.csrf ?? '')
        setUser(u)
      })
      .catch(() => setUser(null))
      .finally(() => setChecked(true))
  }, [])

  const signOut = useCallback(async () => {
    try {
      await api('POST', '/api/auth/logout')
    } catch {
      return
    } finally {
      setCsrf('')
      setUser(null)
    }
  }, [])

  if (!checked) return <div className="center-page boot">{t('loading')}</div>
  if (!user) return <SignIn meta={meta} onSignedIn={setUser} />

  return (
    <div>
      <header className="topbar">
        <Brand />
        <div className="row">
          <Preferences />
          <div className="user-chip">
            <div className="who">
              <strong>{user.name}</strong>
              <small>{t(`role.${user.role}`)}</small>
            </div>
            <button type="button" className="icon-btn" onClick={signOut} aria-label={t('signout')} title={t('signout')}>
              <LogOut size={18} />
            </button>
          </div>
        </div>
      </header>
      <main className="page">{user.role === 'admin' ? <SystemStatus onExpired={() => setUser(null)} /> : <Profile user={user} />}</main>
    </div>
  )
}

function SignIn({ meta, onSignedIn }: { meta: Meta; onSignedIn: (u: User) => void }) {
  const t = useT(strings)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ message: string; detail?: string } | null>(null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const u = await api<User>('POST', '/api/auth/login', { username, password })
      setCsrf(u.csrf ?? '')
      onSignedIn(u)
    } catch (err) {
      setError(errorText(t, err))
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center-page">
      <div className="card signin">
        <div className="row" style={{ justifyContent: 'space-between' }}>
          <Brand />
          <Preferences />
        </div>
        <form onSubmit={submit} noValidate>
          <h1>{t('signin.title')}</h1>
          {error && <Banner kind="error" title={error.message} />}
          <Field label={t('signin.username')}>
            {(id) => <Input id={id} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />}
          </Field>
          <Field label={t('signin.password')}>
            {(id) => <Password id={id} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />}
          </Field>
          <Button variant="primary" type="submit" busy={busy} disabled={!username || !password}>
            {t('signin.submit')}
          </Button>
          {meta.ldap_enabled && <p className="hint">{t('signin.ldap')}</p>}
        </form>
      </div>
      <p className="hint">{meta.version}</p>
    </div>
  )
}

function Pill({ state, t }: { state: 'ok' | 'error' | 'off'; t: (k: string) => string }) {
  return <span className={`pill pill-${state}`}>{t(`state.${state}`)}</span>
}

function SystemStatus({ onExpired }: { onExpired: () => void }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const [s, setS] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ message: string; detail?: string } | null>(null)

  const load = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      setS(await api<Status>('GET', '/api/system'))
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onExpired()
      setError(errorText(t, e))
    } finally {
      setBusy(false)
    }
  }, [t, onExpired])

  useEffect(() => {
    void load()
  }, [load])

  const ob = s?.openbao
  const pg = s?.postgres
  const ld = s?.ldap
  const total = s ? Object.values(s.users).reduce((a, b) => a + b, 0) : 0

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('status.title')}</h1>
          <p className="muted">{t('status.subtitle')}</p>
        </div>
        <Button onClick={load} busy={busy}>
          {!busy && <RefreshCw size={16} />}
          {t('status.refresh')}
        </Button>
      </div>
      {error && (
        <Banner kind="error" title={error.message}>
          {error.detail}
        </Banner>
      )}
      {s && ob && pg && ld && (
        <div className="cards">
          <section className="card status-card" aria-label="OpenBao">
            <header>
              <h2>OpenBao</h2>
              <Pill state={ob.token_ok && ob.mount_ok ? 'ok' : 'error'} t={t} />
            </header>
            <Rows
              rows={[
                [t('field.address'), ob.addr],
                [t('field.version'), ob.version],
                [t('field.mount'), ob.mount],
                [t('field.auth'), ob.auth === 'approle' ? 'AppRole' : t('field.auth.token')],
                [t('field.policies'), ob.policies?.join(', ')],
                [t('field.tokenExpires'), formatDate(ob.token_expires, locale)],
              ]}
            />
            {ob.error && <Banner kind="error" title={ob.error} />}
          </section>
          <section className="card status-card" aria-label="PostgreSQL">
            <header>
              <h2>PostgreSQL</h2>
              <Pill state={pg.ok && !pg.persist.error ? 'ok' : 'error'} t={t} />
            </header>
            <Rows
              rows={[
                [t('field.database'), pg.where],
                [t('field.version'), pg.info.version],
                [t('field.savedAt'), formatDate(pg.info.saved_at, locale)],
              ]}
            />
            {(pg.error || pg.persist.error) && <Banner kind="error" title={pg.error || pg.persist.error} />}
          </section>
          <section className="card status-card" aria-label="LDAP / AD">
            <header>
              <h2>LDAP / AD</h2>
              <Pill state={!ld.enabled ? 'off' : ld.ok ? 'ok' : 'error'} t={t} />
            </header>
            {ld.enabled ? (
              <Rows
                rows={[
                  [t('field.directory'), ld.kind === 'ad' ? 'Active Directory' : 'OpenLDAP'],
                  [t('field.address'), ld.url],
                  [t('field.tls'), ld.tls === 'ldaps' ? 'LDAPS' : ld.tls === 'starttls' ? 'StartTLS' : t('field.tls.none')],
                  [t('field.baseDN'), ld.base_dn],
                  [t('field.adminGroup'), ld.admin_group],
                ]}
              />
            ) : (
              <p className="muted">{t('ldap.off')}</p>
            )}
            {ld.error && <Banner kind="error" title={ld.error} />}
          </section>
          <section className="card status-card" aria-label={t('settings.title')}>
            <header>
              <h2>{t('settings.title')}</h2>
            </header>
            <Rows
              rows={[
                [t('field.theme'), t(`theme.${s.settings.default_theme}`)],
                [t('field.locale'), t(`lang.${s.settings.default_locale}`)],
                [t('field.setupAt'), formatDate(s.settings.setup_at, locale)],
                [t('field.setupBy'), s.settings.setup_by],
                [t('field.users'), t('field.users.value', { total, local: s.users.local ?? 0, ldap: s.users.ldap ?? 0 })],
                [t('field.appVersion'), s.version],
              ]}
            />
          </section>
        </div>
      )}
    </>
  )
}

function Profile({ user }: { user: User }) {
  const t = useT(strings)
  const { locale } = useLocale()
  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('profile.title', { name: user.name })}</h1>
          <p className="muted">{t('profile.subtitle')}</p>
        </div>
      </div>
      <section className="card status-card" style={{ maxWidth: 560 }}>
        <Rows
          rows={[
            [t('signin.username'), user.username],
            [t('field.email'), user.email],
            [t('field.source'), t(`source.${user.source}`)],
            [t('field.role'), t(`role.${user.role}`)],
            [t('field.lastLogin'), formatDate(user.last_login_at, locale)],
          ]}
        />
      </section>
    </>
  )
}

const strings: Dict = {
  en: {
    'signin.title': 'Sign in',
    'signin.username': 'Username',
    'signin.password': 'Password',
    'signin.submit': 'Sign in',
    'signin.ldap': 'Use your corporate (LDAP / AD) account or a local Umbrella account.',
    'signout': 'Sign out',
    'role.admin': 'Administrator',
    'role.user': 'User',
    'source.local': 'Local account',
    'source.ldap': 'LDAP / AD',
    'state.ok': 'Connected',
    'state.error': 'Problem',
    'state.off': 'Off',
    'status.title': 'System status',
    'status.subtitle': 'Connections configured by the setup wizard',
    'status.refresh': 'Refresh',
    'settings.title': 'Defaults',
    'ldap.off': 'Directory sign-in is not configured. Only local accounts can sign in.',
    'field.address': 'Address',
    'field.version': 'Version',
    'field.mount': 'KV mount',
    'field.auth': 'Sign-in method',
    'field.auth.token': 'Token',
    'field.policies': 'Policies',
    'field.tokenExpires': 'Token expires',
    'field.database': 'Database',
    'field.savedAt': 'Last saved',
    'field.directory': 'Directory',
    'field.tls': 'Encryption',
    'field.tls.none': 'None',
    'field.baseDN': 'Search base',
    'field.adminGroup': 'Administrators group',
    'field.theme': 'Theme',
    'field.locale': 'Language',
    'field.setupAt': 'Set up at',
    'field.setupBy': 'Set up by',
    'field.users': 'Users',
    'field.users.value': '{total} (local {local}, LDAP / AD {ldap})',
    'field.appVersion': 'Umbrella version',
    'field.email': 'E-mail',
    'field.source': 'Account',
    'field.role': 'Role',
    'field.lastLogin': 'Last sign-in',
    'profile.title': 'Hello, {name}',
    'profile.subtitle': 'Your Umbrella account',
  },
  ru: {
    'signin.title': 'Вход',
    'signin.username': 'Логин',
    'signin.password': 'Пароль',
    'signin.submit': 'Войти',
    'signin.ldap': 'Используйте корпоративную учётную запись (LDAP / AD) или локальную учётную запись Umbrella.',
    'signout': 'Выйти',
    'role.admin': 'Администратор',
    'role.user': 'Пользователь',
    'source.local': 'Локальная учётная запись',
    'source.ldap': 'LDAP / AD',
    'state.ok': 'Подключено',
    'state.error': 'Проблема',
    'state.off': 'Отключено',
    'status.title': 'Состояние системы',
    'status.subtitle': 'Подключения, настроенные мастером',
    'status.refresh': 'Обновить',
    'settings.title': 'Настройки по умолчанию',
    'ldap.off': 'Вход через каталог не настроен. Войти можно только локальными учётными записями.',
    'field.address': 'Адрес',
    'field.version': 'Версия',
    'field.mount': 'Хранилище KV',
    'field.auth': 'Способ входа',
    'field.auth.token': 'Токен',
    'field.policies': 'Политики',
    'field.tokenExpires': 'Токен действует до',
    'field.database': 'База данных',
    'field.savedAt': 'Последнее сохранение',
    'field.directory': 'Каталог',
    'field.tls': 'Шифрование',
    'field.tls.none': 'Нет',
    'field.baseDN': 'База поиска',
    'field.adminGroup': 'Группа администраторов',
    'field.theme': 'Тема',
    'field.locale': 'Язык',
    'field.setupAt': 'Настроено',
    'field.setupBy': 'Кем настроено',
    'field.users': 'Пользователи',
    'field.users.value': '{total} (локальных {local}, LDAP / AD {ldap})',
    'field.appVersion': 'Версия Umbrella',
    'field.email': 'Почта',
    'field.source': 'Учётная запись',
    'field.role': 'Роль',
    'field.lastLogin': 'Последний вход',
    'profile.title': 'Здравствуйте, {name}',
    'profile.subtitle': 'Ваша учётная запись Umbrella',
  },
}
