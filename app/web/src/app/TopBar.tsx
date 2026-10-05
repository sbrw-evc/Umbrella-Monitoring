import { Link } from '../router'
import { Brand } from '../ui'
import { PATHS } from './routes'
import { UserMenu } from './UserMenu'

export function TopBar({ onSignOut }: { onSignOut: () => void }) {
  return (
    <header className="topbar">
      <div className="topbar-start">
        <Link to={PATHS.status} className="brand-link">
          <Brand />
        </Link>
      </div>
      <UserMenu onSignOut={onSignOut} />
    </header>
  )
}
