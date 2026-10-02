import { Globe, LogIn, Moon, Sun } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { api, type Me } from '../api'
import { Logo } from '../components/Layout'
import { useApp } from '../context'
import { t } from '../i18n'

// SignIn is the whole screen while nobody is signed in, and the forced
// password change after an administrator reset.
export function SignIn({ mustChange }: { mustChange: boolean }) {
  const { theme, setTheme, locale, locales, setLocale } = useApp()
  return (
    <div className="signin">
      <div className="signin-tools">
        <label className="signin-lang" title={t('common.layout.language')}>
          <Globe size={15} />
          <select value={locale.id} onChange={(e) => setLocale(e.target.value)}>
            {locales.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </label>
        <button className="top-icon" onClick={() => setTheme(theme.base === 'dark' ? 'light' : 'dark')} title={t(theme.base === 'dark' ? 'common.layout.themeLight' : 'common.layout.themeDark')}>
          {theme.base === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
        </button>
      </div>
      <div className="signin-card card">
        <div className="signin-brand">
          <Logo size={40} />
          <div>
            <b>{t('common.app.brand')}</b>
            <small>{t('common.app.tagline')}</small>
          </div>
        </div>
        {mustChange ? <ChangeForm /> : <LoginForm />}
      </div>
    </div>
  )
}

function LoginForm() {
  const { signedIn } = useApp()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      signedIn(await api.post<Me>('/api/auth/login', { username, password }))
    } catch (err) {
      setError((err as Error).message || t('auth.failed'))
      setPassword('')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form className="signin-form" onSubmit={submit}>
      <h2>{t('auth.title')}</h2>
      <div className="muted">{t('auth.sub')}</div>
      <label className="field">
        <span className="field-label">{t('auth.username')}</span>
        <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
      </label>
      <label className="field">
        <span className="field-label">{t('auth.password')}</span>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
      </label>
      {error && <div className="form-error shake">{error}</div>}
      <button className="btn btn-primary signin-submit" disabled={busy || !username || !password}>
        <LogIn size={16} />
        {busy ? t('auth.signingIn') : t('auth.signIn')}
      </button>
    </form>
  )
}

export function PasswordFields({ onDone, submitLabel }: { onDone: () => void; submitLabel?: string }) {
  const { toast } = useApp()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (next !== repeat) {
      setError(t('auth.mismatch'))
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.post('/api/auth/password', { current, new: next })
      toast(t('auth.changed'))
      onDone()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <form className="signin-form" onSubmit={submit}>
      <label className="field">
        <span className="field-label">{t('auth.current')}</span>
        <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" autoFocus required />
      </label>
      <label className="field">
        <span className="field-label">{t('auth.newPassword')}</span>
        <input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" required />
        <span className="field-help">{t('auth.policy')}</span>
      </label>
      <label className="field">
        <span className="field-label">{t('auth.repeat')}</span>
        <input type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} autoComplete="new-password" required />
      </label>
      {error && <div className="form-error shake">{error}</div>}
      <button className="btn btn-primary signin-submit" disabled={busy || !current || !next}>
        {submitLabel ?? t('auth.change')}
      </button>
    </form>
  )
}

function ChangeForm() {
  const { refreshMe, logout } = useApp()
  return (
    <>
      <h2>{t('auth.changeTitle')}</h2>
      <div className="muted">{t('auth.changeSub')}</div>
      <PasswordFields onDone={refreshMe} />
      <button className="btn btn-ghost signin-other" onClick={logout}>
        {t('auth.other')}
      </button>
    </>
  )
}
