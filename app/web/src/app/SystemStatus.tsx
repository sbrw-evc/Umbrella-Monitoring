import { useCallback, useEffect, useState } from 'react'
import { Pencil, RefreshCw } from 'lucide-react'
import { api, ApiError, type Locale, type Theme } from '../api'
import { defaultPolicy, ruleText, rules, type PasswordPolicy } from '../policy'
import { errorText, useLocale, useT } from '../i18n'
import { Banner, Button, formatDate, Rows, zoneLabel } from '../ui'
import { DefaultsEditor } from './DefaultsEditor'
import { useSession } from './session'
import { strings } from './strings'

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
  settings: { default_theme: Theme; default_locale: Locale; default_timezone: string; password_policy?: PasswordPolicy; setup_at?: string; setup_by?: string }
  users: Record<string, number>
}

function Pill({ state, t }: { state: 'ok' | 'error' | 'off'; t: (k: string) => string }) {
  return <span className={`pill pill-${state}`}>{t(`state.${state}`)}</span>
}

export function SystemStatus({ onDefaults }: { onDefaults: (tz: string) => void }) {
  const t = useT(strings)
  const { timezone: tz, expire: onExpired } = useSession()
  const { locale } = useLocale()
  const [s, setS] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ message: string; detail?: string } | null>(null)
  const [editing, setEditing] = useState(false)
  const closeEditor = useCallback(() => setEditing(false), [])

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
                [t('field.tokenExpires'), formatDate(ob.token_expires, locale, tz)],
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
                [t('field.savedAt'), formatDate(pg.info.saved_at, locale, tz)],
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
                [t('field.timezone'), zoneLabel(s.settings.default_timezone)],
                [
                  t('field.passwordPolicy'),
                  rules(s.settings.password_policy ?? defaultPolicy)
                    .filter((r) => r !== 'username')
                    .map((r) => ruleText(t, r, s.settings.password_policy ?? defaultPolicy))
                    .join('; '),
                ],
                [t('field.setupAt'), formatDate(s.settings.setup_at, locale, tz)],
                [t('field.setupBy'), s.settings.setup_by],
                [t('field.users'), t('field.users.value', { total, local: s.users.local ?? 0, ldap: s.users.ldap ?? 0 })],
                [t('field.appVersion'), s.version],
              ]}
            />
            <div className="card-actions">
              <Button onClick={() => setEditing(true)}>
                <Pencil size={16} />
                {t('defaults.edit')}
              </Button>
            </div>
          </section>
        </div>
      )}
      {s && (
        <DefaultsEditor
          open={editing}
          onClose={closeEditor}
          initial={s.settings}
          onSaved={(next) => {
            setS({ ...s, settings: { ...s.settings, ...next } })
            onDefaults(next.default_timezone)
          }}
        />
      )}
    </>
  )
}
