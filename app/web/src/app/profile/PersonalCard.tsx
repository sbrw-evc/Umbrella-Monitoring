import { useEffect, useState } from 'react'
import { api } from '../../api'
import { useT } from '../../i18n'
import { Banner, Button, Field, Input } from '../../ui'
import { useSession } from '../session'
import { strings } from '../strings'
import { PROFILE_KEYS, profileOf, type ProfileFields, type User } from '../types'
import { AvatarEditor } from './AvatarEditor'
import { ProfileCard } from './ProfileCard'
import { useAction } from './useAction'

const REQUIRED: (keyof ProfileFields)[] = ['last_name', 'first_name']

export function PersonalCard() {
  const t = useT(strings)
  const { user, update } = useSession()
  const action = useAction()
  const local = user.source === 'local'
  const [form, setForm] = useState<ProfileFields>(() => profileOf(user))

  useEffect(() => setForm(profileOf(user)), [user])

  const saved = profileOf(user)
  const changed = PROFILE_KEYS.some((k) => form[k] !== saved[k])

  const save = () =>
    action.run(async () => {
      update(await api<User>('PUT', '/api/auth/me/profile', form))
      return t('prefs.saved')
    })

  return (
    <ProfileCard
      title={t('profile.personal')}
      action={action}
      wide
      onSubmit={local ? save : undefined}
      footer={
        local && (
          <Button type="submit" variant="primary" busy={action.busy} disabled={!changed}>
            {t('save')}
          </Button>
        )
      }
    >
      <AvatarEditor action={action} />
      {!local && <Banner kind="info" title={t('prefs.directory')} />}
      <div className="grid-2">
        {PROFILE_KEYS.map((k) => (
          <Field key={k} label={t(`field.${k}`)} optional={local && !REQUIRED.includes(k) ? t('optional') : undefined}>
            {(id) => (
              <Input
                id={id}
                type={k === 'email' ? 'email' : 'text'}
                value={form[k]}
                disabled={!local}
                readOnly={!local}
                onChange={(e) => setForm({ ...form, [k]: e.target.value })}
              />
            )}
          </Field>
        ))}
      </div>
    </ProfileCard>
  )
}
