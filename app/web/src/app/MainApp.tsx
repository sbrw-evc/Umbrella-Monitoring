import { useCallback, useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { api, ApiError, setCsrf, type Meta } from '../api'
import { defaultPolicy, type PasswordPolicy } from '../policy'
import { useT } from '../i18n'
import { RouterProvider, SCROLL_ROOT_ID, useRouter } from '../router'
import { ExpiredPassword } from './profile/ExpiredPassword'
import { PasswordExpiryNotice } from './profile/PasswordExpiryNotice'
import { ProfilePage } from './profile/ProfilePage'
import { PageBoundary } from './PageBoundary'
import { PageHead } from './PageHead'
import { resolve, visiblePages } from './pages'
import { SessionProvider, useSession, type Session } from './session'
import { Sidebar, useCollapsed } from './Sidebar'
import { SignIn } from './SignIn'
import { strings } from './strings'
import { TopBar } from './TopBar'
import type { User } from './types'
import { setNotifyOwner, Toaster } from '../notify'
import { ConfirmHost } from '../confirm'

export default function MainApp({ meta }: { meta: Meta }) {
  return (
    <RouterProvider>
      <Shell meta={meta} />
      <Toaster />
      <ConfirmHost />
    </RouterProvider>
  )
}

function Shell({ meta }: { meta: Meta }) {
  const [user, setUser] = useState<User | null>(null)
  const [checked, setChecked] = useState(false)
  const [defaultTz, setDefaultTz] = useState(meta.default_timezone || 'UTC')
  const [policy, setPolicy] = useState<PasswordPolicy>(meta.password_policy ?? defaultPolicy)
  const t = useT(strings)

  useEffect(() => setNotifyOwner(user?.id ?? ''), [user?.id])

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

  const permissions = useMemo(() => new Set(user?.permissions ?? []), [user?.permissions])

  const session = useMemo<Session | null>(
    () =>
      user && {
        user,
        timezone: user.timezone || defaultTz,
        defaultTz,
        policy,
        can: (perm: string) => permissions.has(perm),
        setDefaultTz,
        update: setUser,
        refresh,
        setPolicy,
        expire,
      },
    [user, defaultTz, policy, refresh, expire, permissions],
  )

  if (!checked) return <div className="center-page boot">{t('loading')}</div>
  if (!session) return <SignIn meta={meta} onSignedIn={setUser} />

  const hasSidebar = visiblePages(session.can).length > 0

  if (session.user.password_expired) {
    return (
      <SessionProvider value={session}>
        <ExpiredPassword onSignOut={signOut} />
      </SessionProvider>
    )
  }

  return (
    <SessionProvider value={session}>
      <Frame hasSidebar={hasSidebar} user={session.user.id} onSignOut={signOut} />
    </SessionProvider>
  )
}

function Frame({ hasSidebar, user, onSignOut }: { hasSidebar: boolean; user: string; onSignOut: () => void }) {
  const [collapsed, toggle] = useCollapsed(user)
  return (
    <div className="app-shell">
      <TopBar onSignOut={onSignOut} sidebar={hasSidebar ? { collapsed, toggle } : undefined} />
      <div className={`app-body ${hasSidebar ? (collapsed ? 'side-collapsed' : '') : 'no-sidebar'}`}>
        {hasSidebar && <Sidebar collapsed={collapsed} />}
        <div id={SCROLL_ROOT_ID} className="app-main">
          <Pages />
        </div>
      </div>
    </div>
  )
}

function Pages() {
  const { path, navigate } = useRouter()
  const { user, refresh, can } = useSession()
  const target = resolve(path, can)

  useEffect(() => {
    if (user.source === 'local') void refresh()
  }, [path, user.source, refresh])

  useEffect(() => {
    if (target.kind === 'redirect') navigate(target.to, { replace: true })
  }, [target, navigate])

  if (target.kind === 'redirect') return null
  const key = target.kind === 'page' ? target.page.id : 'profile'

  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.main
        key={key}
        className="page"
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -6 }}
        transition={{ duration: 0.2, ease: [0.22, 1, 0.36, 1] }}
      >
        {target.kind === 'profile' ? (
          <PageBoundary resetKey={path}>
            <ProfilePage />
          </PageBoundary>
        ) : (
          <>
            <PasswordExpiryNotice link />
            {target.page.subtitle && <PageHead page={target.page} />}
            <PageBoundary resetKey={path}>
              <target.page.Component />
            </PageBoundary>
          </>
        )}
      </motion.main>
    </AnimatePresence>
  )
}
