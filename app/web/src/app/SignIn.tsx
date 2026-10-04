import { useState, type FormEvent } from 'react'
import { api, setCsrf, type Meta } from '../api'
import { errorText, useT } from '../i18n'
import { Banner, Brand, Button, Field, Input, Password, Preferences } from '../ui'
import { strings } from './strings'
import type { User } from './types'

export function SignIn({ meta, onSignedIn }: { meta: Meta; onSignedIn: (u: User) => void }) {
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
