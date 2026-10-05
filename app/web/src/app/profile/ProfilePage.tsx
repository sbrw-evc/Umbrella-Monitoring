import { Avatar } from '../../Avatar'
import { useT } from '../../i18n'
import { useSession } from '../session'
import { strings } from '../strings'
import { fullName } from '../types'
import { AccountCard } from './AccountCard'
import { PasswordCard } from './PasswordCard'
import { PersonalCard } from './PersonalCard'
import { TelegramCard } from './TelegramCard'
import { TimezoneCard } from './TimezoneCard'

export function ProfilePage() {
  const t = useT(strings)
  const { user } = useSession()
  return (
    <>
      <div className="page-head">
        <div className="profile-head">
          <Avatar user={user} size={88} />
          <div>
            <h1>{fullName(user)}</h1>
            <p className="muted">{[user.title, user.department].filter(Boolean).join(' · ') || t('profile.subtitle')}</p>
          </div>
        </div>
      </div>
      <div className="cards">
        <PersonalCard />
        {user.source === 'local' && <PasswordCard />}
        <TimezoneCard />
        <TelegramCard />
        <AccountCard />
      </div>
    </>
  )
}
