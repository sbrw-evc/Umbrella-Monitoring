import { ArrowLeft, ArrowRight, Check, Database, HardDrive, Plug } from 'lucide-react'
import { useState } from 'react'
import { api, fmtDateTime, type StorageView } from '../api'
import { Logo } from '../components/Layout'
import { Field } from '../components/ui'
import { useApp } from '../context'
import { t } from '../i18n'

type Step = 'look' | 'storage' | 'admin' | 'done'

export interface DbForm {
  kind: 'file' | 'postgres' | 'keep'
  host: string
  port: number
  database: string
  user: string
  password: string
  sslmode: string
  adopt: boolean
}

export function dbFromView(v?: StorageView): DbForm {
  const pg = v?.postgres
  return {
    kind: v?.kind === 'postgres' ? 'postgres' : 'file',
    host: pg?.host ?? '',
    port: pg?.port ?? 5432,
    database: pg?.database ?? 'umbrella',
    user: pg?.user ?? 'umbrella',
    password: '',
    sslmode: pg?.sslmode ?? 'prefer',
    adopt: true,
  }
}

interface TestResult {
  ok: boolean
  message: string
  has_state?: boolean
  saved_at?: string
}

export function DbFields({
  db,
  setDb,
  storage,
  openbao,
  test,
}: {
  db: DbForm
  setDb: (f: (d: DbForm) => DbForm) => void
  storage?: StorageView
  openbao: boolean
  test: () => Promise<TestResult>
}) {
  const [result, setResult] = useState<TestResult | null>(null)
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof DbForm>(k: K, v: DbForm[K]) => {
    setDb((d) => ({ ...d, [k]: v }))
    setResult(null)
  }
  const run = async () => {
    setBusy(true)
    try {
      const r = await test()
      setResult(r)
      if (r.ok) setDb((d) => ({ ...d, adopt: !!r.has_state }))
    } catch (e) {
      setResult({ ok: false, message: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  return (
    <>
      {storage && <p className="hint">{t('setup.storage.current', { kind: storage.kind, where: storage.where })}</p>}
      <div className="choice">
        <label className={`choice-item ${db.kind === 'file' ? 'choice-active' : ''}`}>
          <input type="radio" checked={db.kind === 'file'} onChange={() => set('kind', 'file')} />
          <b>
            <HardDrive size={14} /> {t('setup.storage.file')}
          </b>
          <span>{t('setup.storage.fileHint', { dir: storage?.data_dir || '—' })}</span>
        </label>
        <label className={`choice-item ${db.kind === 'postgres' ? 'choice-active' : ''}`}>
          <input type="radio" checked={db.kind === 'postgres'} disabled={!openbao} onChange={() => set('kind', 'postgres')} />
          <b>
            <Database size={14} /> {t('setup.storage.postgres')}
          </b>
          <span>{openbao ? t('setup.storage.postgresHint') : t('setup.storage.noVault')}</span>
        </label>
      </div>
      {db.kind === 'postgres' && (
        <div className="section">
          <div className="row3">
            <Field label={t('setup.storage.host')}>
              <input value={db.host} onChange={(e) => set('host', e.target.value)} placeholder="db.example.com" />
            </Field>
            <Field label={t('setup.storage.port')}>
              <input type="number" value={db.port} onChange={(e) => set('port', Number(e.target.value))} />
            </Field>
            <Field label={t('setup.storage.database')}>
              <input value={db.database} onChange={(e) => set('database', e.target.value)} />
            </Field>
          </div>
          <div className="row3">
            <Field label={t('setup.storage.user')}>
              <input value={db.user} onChange={(e) => set('user', e.target.value)} autoComplete="off" />
            </Field>
            <Field label={t('setup.storage.password')}>
              <input
                type="password"
                value={db.password}
                onChange={(e) => set('password', e.target.value)}
                placeholder={storage?.password_set ? t('setup.storage.passwordKeep') : ''}
                autoComplete="new-password"
              />
            </Field>
            <Field label={t('setup.storage.sslmode')}>
              <select value={db.sslmode} onChange={(e) => set('sslmode', e.target.value)}>
                {['disable', 'prefer', 'require', 'verify-ca', 'verify-full'].map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </select>
            </Field>
          </div>
          <button type="button" className="btn" onClick={run} disabled={busy || !db.host || !db.database || !db.user}>
            <Plug size={14} /> {t('setup.storage.test')}
          </button>
          {result && (
            <div className="int-line section">
              <span className={result.ok ? 'result-ok' : 'result-bad'}>{result.message}</span>
            </div>
          )}
          {result?.ok &&
            (result.has_state ? (
              <>
                <p className="hint hint-warn">{t('setup.storage.found', { time: fmtDateTime(result.saved_at) })}</p>
                <label className="check">
                  <input type="checkbox" checked={db.adopt} onChange={(e) => setDb((d) => ({ ...d, adopt: e.target.checked }))} /> {t('setup.storage.adopt')}
                </label>
              </>
            ) : (
              <p className="hint">{t('setup.storage.empty')}</p>
            ))}
        </div>
      )}
    </>
  )
}

export function SetupWizard() {
  const { setup, reloadSetup, theme, setTheme, locale, locales, setLocale, me, toast } = useApp()
  const needAdmin = !setup?.admin_exists
  const steps: Step[] = needAdmin ? ['look', 'storage', 'admin', 'done'] : ['look', 'storage', 'done']
  const [step, setStep] = useState<Step>('look')
  const [code, setCode] = useState('')
  const [db, setDb] = useState<DbForm>(() => dbFromView(setup?.storage))
  const [admin, setAdmin] = useState({ username: 'admin', name: '', email: '', password: '', repeat: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [finished, setFinished] = useState<{ admin: string; adopted: boolean } | null>(null)
  const idx = steps.indexOf(step)
  const themeId = theme.base === 'dark' ? 'dark' : 'light'
  const headers: Record<string, string> = setup?.token_required ? { 'X-Umbrella-Setup-Token': code.trim() } : {}
  const post = <T,>(url: string, body: unknown) => (me ? api.post<T>(url, body) : api.postWith<T>(url, body, headers))
  const storageBody = () => (db.kind === 'postgres' ? { ...db } : { kind: 'file' })
  const canNext =
    (step === 'look' && (!setup?.token_required || code.trim().length > 0)) ||
    (step === 'storage' && (db.kind !== 'postgres' || (!!db.host && !!db.database && !!db.user))) ||
    (step === 'admin' && !!admin.username && admin.password.length >= 10 && admin.password === admin.repeat)
  const finish = async () => {
    setBusy(true)
    setError('')
    try {
      const body: Record<string, unknown> = { theme: themeId, locale: locale.id === 'en' ? 'en' : 'ru', storage: storageBody() }
      if (needAdmin) body.admin = { username: admin.username, name: admin.name, email: admin.email, password: admin.password }
      const r = await post<{ admin_created: string; adopted: boolean }>('/api/setup/complete', body)
      toast(t('setup.done.finished'))
      setFinished({ admin: r.admin_created, adopted: r.adopted })
      if (!r.admin_created) await reloadSetup()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  if (finished?.admin) {
    return (
      <div className="signin">
        <div className="signin-card card">
          <div className="signin-brand">
            <Logo size={40} />
            <div>
              <b>{t('setup.done.finished')}</b>
              <small>{finished.adopted ? t('setup.done.adopted') : t('setup.done.signIn')}</small>
            </div>
          </div>
          <button className="btn btn-primary signin-submit" onClick={() => reloadSetup()}>
            {t('setup.done.signIn')}: {finished.admin}
          </button>
        </div>
      </div>
    )
  }
  return (
    <div className="signin">
      <div className="signin-card card setup-card">
        <div className="signin-brand">
          <Logo size={40} />
          <div>
            <b>{t('setup.title')}</b>
            <small>{t('setup.sub', { version: setup?.version ?? '' })}</small>
          </div>
        </div>
        <div className="setup-steps">
          {steps.map((s, i) => (
            <div key={s} className={`setup-step ${s === step ? 'setup-step-active' : ''} ${i < idx ? 'setup-step-done' : ''}`}>
              <span className="setup-step-num">{i < idx ? <Check size={12} /> : i + 1}</span>
              {t(`setup.steps.${s}`)}
            </div>
          ))}
        </div>
        {step === 'look' && (
          <>
            <div className="row2">
              <Field label={t('setup.look.language')}>
                <select value={locale.id} onChange={(e) => setLocale(e.target.value)}>
                  {locales
                    .filter((l) => (setup?.locales ?? ['ru', 'en']).includes(l.id))
                    .map((l) => (
                      <option key={l.id} value={l.id}>
                        {l.name}
                      </option>
                    ))}
                </select>
              </Field>
              <Field label={t('setup.look.theme')}>
                <div className="seg">
                  {(setup?.themes ?? ['light', 'dark']).map((x) => (
                    <button type="button" key={x} className={`seg-btn ${themeId === x ? 'seg-active' : ''}`} onClick={() => setTheme(x)}>
                      {t(`setup.look.themes.${x}`)}
                    </button>
                  ))}
                </div>
              </Field>
            </div>
            <p className="hint">{t('setup.look.hint')}</p>
            {setup?.token_required && (
              <Field label={t('setup.look.code')} help={t('setup.look.codeHelp')}>
                <input className="mono" value={code} onChange={(e) => setCode(e.target.value)} autoFocus autoComplete="off" />
              </Field>
            )}
          </>
        )}
        {step === 'storage' && (
          <>
            <p className="hint">{t('setup.storage.intro')}</p>
            <DbFields db={db} setDb={setDb} storage={setup?.storage} openbao={!!setup?.openbao} test={() => post('/api/setup/database/test', storageBody())} />
          </>
        )}
        {step === 'admin' && (
          <>
            <p className="hint">{t('setup.admin.intro')}</p>
            <div className="row2">
              <Field label={t('setup.admin.username')}>
                <input value={admin.username} onChange={(e) => setAdmin({ ...admin, username: e.target.value })} autoComplete="username" />
              </Field>
              <Field label={t('setup.admin.name')}>
                <input value={admin.name} onChange={(e) => setAdmin({ ...admin, name: e.target.value })} />
              </Field>
            </div>
            <Field label={t('setup.admin.email')}>
              <input type="email" value={admin.email} onChange={(e) => setAdmin({ ...admin, email: e.target.value })} />
            </Field>
            <div className="row2">
              <Field label={t('setup.admin.password')}>
                <input type="password" value={admin.password} onChange={(e) => setAdmin({ ...admin, password: e.target.value })} autoComplete="new-password" />
              </Field>
              <Field label={t('setup.admin.repeat')}>
                <input type="password" value={admin.repeat} onChange={(e) => setAdmin({ ...admin, repeat: e.target.value })} autoComplete="new-password" />
              </Field>
            </div>
            {admin.repeat && admin.password !== admin.repeat && <div className="form-error">{t('setup.admin.mismatch')}</div>}
          </>
        )}
        {step === 'done' && (
          <>
            <p className="hint">{t('setup.done.intro')}</p>
            <div className="props">
              <div className="prop">
                <div className="prop-k">{t('setup.done.language')}</div>
                <div className="prop-v">{locale.name}</div>
              </div>
              <div className="prop">
                <div className="prop-k">{t('setup.done.theme')}</div>
                <div className="prop-v">{t(`setup.look.themes.${themeId}`)}</div>
              </div>
              <div className="prop">
                <div className="prop-k">{t('setup.done.storage')}</div>
                <div className="prop-v">
                  {db.kind === 'postgres' ? `PostgreSQL ${db.user}@${db.host}:${db.port}/${db.database}` : t('setup.storage.file')}
                  {db.kind === 'postgres' && db.adopt ? ` · ${t('setup.done.adopted')}` : ''}
                </div>
              </div>
              <div className="prop">
                <div className="prop-k">{t('setup.done.admin')}</div>
                <div className="prop-v">{needAdmin ? admin.username : t('setup.admin.exists')}</div>
              </div>
            </div>
          </>
        )}
        {error && <div className="form-error shake">{error}</div>}
        <div className="setup-nav">
          {idx > 0 && (
            <button className="btn" onClick={() => setStep(steps[idx - 1])} disabled={busy}>
              <ArrowLeft size={14} /> {t('setup.nav.back')}
            </button>
          )}
          <div className="filterbar-spacer" />
          {step === 'done' ? (
            <button className="btn btn-primary" onClick={finish} disabled={busy}>
              <Check size={14} /> {t('setup.done.finish')}
            </button>
          ) : (
            <button className="btn btn-primary" onClick={() => setStep(steps[idx + 1])} disabled={!canNext}>
              {t('setup.nav.next')} <ArrowRight size={14} />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
