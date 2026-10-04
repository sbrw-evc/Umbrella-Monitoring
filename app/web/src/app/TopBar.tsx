import { motion } from 'motion/react'
import { useT } from '../i18n'
import { Link, useRouter } from '../router'
import { Brand, spring } from '../ui'
import { PATHS, pageFor, type Page } from './routes'
import { useSession } from './session'
import { strings } from './strings'
import { UserMenu } from './UserMenu'

const NAV: { page: Page; label: string }[] = [
  { page: 'status', label: 'status.title' },
  { page: 'profile', label: 'nav.profile' },
]

function Nav() {
  const t = useT(strings)
  const { path } = useRouter()
  const { user } = useSession()
  const active = pageFor(path, user)
  return (
    <nav className="topnav" aria-label={t('nav.label')}>
      {NAV.map((n) => (
        <Link key={n.page} to={PATHS[n.page]} className={active === n.page ? 'active' : ''} aria-current={active === n.page ? 'page' : undefined}>
          {active === n.page && <motion.span layoutId="topnav-pill" className="topnav-pill" transition={spring} />}
          <span className="topnav-label">{t(n.label)}</span>
        </Link>
      ))}
    </nav>
  )
}

export function TopBar({ onSignOut }: { onSignOut: () => void }) {
  const { user } = useSession()
  return (
    <header className="topbar">
      <div className="topbar-start">
        <Link to={PATHS.status} className="brand-link">
          <Brand />
        </Link>
        {user.role === 'admin' && <Nav />}
      </div>
      <UserMenu onSignOut={onSignOut} />
    </header>
  )
}
