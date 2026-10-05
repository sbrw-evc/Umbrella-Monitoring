import { Link } from '../router'
import { Brand } from '../ui'
import { HOME_PATH } from './pages'
import { UserMenu } from './UserMenu'

export function TopBar({ onSignOut }: { onSignOut: () => void }) {
  return (
    <header className="topbar">
      <div className="topbar-start">
        <Link to={HOME_PATH} className="brand-link">
          <Brand />
        </Link>
      </div>
      <UserMenu onSignOut={onSignOut} />
    </header>
  )
}
