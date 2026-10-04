import { useCallback, useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { api, setCsrf, type Meta } from '../api'
import { defaultPolicy } from '../policy'
import { useT } from '../i18n'
import { RouterProvider, useRouter } from '../router'
import { ProfilePage } from './profile/ProfilePage'
import { PATHS, pageFor } from './routes'
import { SessionProvider, useSession, type Session } from './session'
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
        policy: meta.password_policy ?? defaultPolicy,
        update: setUser,
        expire,
      },
    [user, defaultTz, meta.password_policy, expire],
  )

  if (!checked) return <div className="center-page boot">{t('loading')}</div>
  if (!session) return <SignIn meta={meta} onSignedIn={setUser} />

  return (
    <SessionProvider value={session}>
      <TopBar onSignOut={signOut} />
      <Pages onDefaults={setDefaultTz} />
    </SessionProvider>
  )
}

function Pages({ onDefaults }: { onDefaults: (tz: string) => void }) {
  const { path, navigate } = useRouter()
  const { user } = useSession()
  const page = pageFor(path, user)

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
          {page === 'status' ? <SystemStatus onDefaults={onDefaults} /> : <ProfilePage />}
        </motion.main>
      )}
    </AnimatePresence>
  )
}
