import { useEffect, useState } from 'react'
import { api } from '../../api'
import { useT } from '../../i18n'
import { Button, Field, Input } from '../../ui'
import { useSession } from '../session'
import { strings } from '../strings'
import type { User } from '../types'
import { ProfileCard } from './ProfileCard'
import { useAction } from './useAction'

export function TelegramCard() {
  const t = useT(strings)
  const { user, update } = useSession()
  const action = useAction()
  const [chat, setChat] = useState(user.telegram ?? '')

  useEffect(() => setChat(user.telegram ?? ''), [user.telegram])

  const save = () =>
    action.run(async () => {
      update(await api<User>('PUT', '/api/auth/me/preferences', { telegram: chat.trim() }))
      return t('prefs.tg.saved')
    })

  return (
    <ProfileCard
      title={t('prefs.tg.title')}
      action={action}
      onSubmit={save}
      footer={
        <Button type="submit" variant="primary" busy={action.busy} disabled={chat.trim() === (user.telegram ?? '')}>
          {t('save')}
        </Button>
      }
    >
      <Field label={t('prefs.tg.field')} hint={t('prefs.tg.hint')}>
        {(id) => <Input id={id} value={chat} inputMode="text" autoComplete="off" placeholder="123456789" onChange={(e) => setChat(e.target.value)} />}
      </Field>
    </ProfileCard>
  )
}
