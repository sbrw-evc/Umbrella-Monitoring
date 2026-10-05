import { useCallback, useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { api, ApiError, setCsrf, type Meta } from '../api'
import { defaultPolicy, type PasswordPolicy } from '../policy'
import { useT } from '../i18n'
import { RouterProvider, SCROLL_ROOT_ID, useRouter } from '../router'
import { ExpiredPassword } from './profile/ExpiredPassword'
import { PasswordExpiryNotice } from './profile/PasswordExpiryNotice'
import { ProfilePage } from './profile/ProfilePage'
import { PATHS, pageFor } from './routes'
import { SettingsPage } from './settings/SettingsPage'
import { SessionProvider, useSession, type Session } from './session'
import { Sidebar } from './Sidebar'
import { SignIn } from './SignIn'
import { strings } from './strings'
import { SystemStatus } from './SystemStatus'
import { TopBar } from './TopBar'
import type { User } from './types'

export default function MainApp({ meta }: { meta: Meta }) {
  return (
    <RouterProvider>
      <Shell meta={meta} />
    </RouterProvider>
  )
}

function Shell({ meta }: { meta: Meta }) {
  const [user, setUser] = useState<User | null>(null)
  const [checked, setChecked] = useState(false)
  const [defaultTz, setDefaultTz] = useState(meta.default_timezone || 'UTC')
  const [policy, setPolicy] = useState<PasswordPolicy>(meta.password_policy ?? defaultPolicy)
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

  const expire = useCallback(() => {
    setCsrf('')
    setUser(null)
  }, [])

  const refresh = useCallback(async () => {
    try {
      const next = await api<User>('GET', '/api/auth/me')
      setUser((prev) => (prev && JSON.stringify(prev) === JSON.stringify(next) ? prev : next))
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) expire()
    }
  }, [expire])

  const signOut = useCallback(async () => {
    try {
      await api('POST', '/api/auth/logout')
    } catch {
      return
    } finally {
      expire()
    }
  }, [expire])

  const session = useMemo<Session | null>(
    () =>
      user && {
        user,
        timezone: user.timezone || defaultTz,
        defaultTz,
        policy,
        update: setUser,
        refresh,
        setPolicy,
        expire,
      },
    [user, defaultTz, policy, refresh, expire],
  )

  if (!checked) return <div className="center-page boot">{t('loading')}</div>
  if (!session) return <SignIn meta={meta} onSignedIn={setUser} />

  if (session.user.password_expired) {
    return (
      <SessionProvider value={session}>
        <ExpiredPassword onSignOut={signOut} />
      </SessionProvider>
    )
  }

  return (
    <SessionProvider value={session}>
      <div className="app-shell">
        <TopBar onSignOut={signOut} />
        <div className={`app-body ${session.user.role === 'admin' ? '' : 'no-sidebar'}`}>
          {session.user.role === 'admin' && <Sidebar />}
          <div id={SCROLL_ROOT_ID} className="app-main">
            <Pages onDefaults={setDefaultTz} />
          </div>
        </div>
      </div>
    </SessionProvider>
  )
}

function Pages({ onDefaults }: { onDefaults: (tz: string) => void }) {
  const { path, navigate } = useRouter()
  const { user, refresh } = useSession()
  const page = pageFor(path, user)

  useEffect(() => {
    if (user.source === 'local') void refresh()
  }, [path, user.source, refresh])

  useEffect(() => {
    if (!page) navigate(PATHS.status, { replace: true })
  }, [page, navigate])

  return (
    <AnimatePresence mode="wait" initial={false}>
      {page && (
        <motion.main
          key={page}
          className="page"
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={{ duration: 0.2, ease: [0.22, 1, 0.36, 1] }}
        >
          {page !== 'profile' && <PasswordExpiryNotice link />}
          {page === 'status' && <SystemStatus onDefaults={onDefaults} />}
          {page === 'settings' && <SettingsPage />}
          {page === 'profile' && <ProfilePage />}
        </motion.main>
      )}
    </AnimatePresence>
  )
}
