import { useState } from 'react'
import { api } from '../../api'
import { PolicyChecklist } from '../../PolicyChecklist'
import { checkPassword } from '../../policy'
import { useT } from '../../i18n'
import { Button, Field, Password } from '../../ui'
import { useSession } from '../session'
import { strings } from '../strings'
import { ProfileCard } from './ProfileCard'
import { useAction } from './useAction'

const empty = { current: '', next: '', confirm: '' }

export function PasswordCard() {
  const t = useT(strings)
  const { user, policy } = useSession()
  const action = useAction()
  const [pw, setPw] = useState(empty)

  const valid = pw.current !== '' && checkPassword(pw.next, user.username, policy).length === 0 && pw.next === pw.confirm

  const save = () =>
    action.run(async () => {
      await api('PUT', '/api/auth/me/password', { current_password: pw.current, new_password: pw.next })
      setPw(empty)
      return t('prefs.password.changed')
    })

  return (
    <ProfileCard
      title={t('profile.password')}
      action={action}
      onSubmit={save}
      footer={
        <Button type="submit" variant="primary" busy={action.busy} disabled={!valid}>
          {t('prefs.password.change')}
        </Button>
      }
    >
      <input type="text" name="username" autoComplete="username" value={user.username} readOnly hidden />
      <Field label={t('prefs.password.current')}>
        {(id) => <Password id={id} value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} autoComplete="current-password" />}
      </Field>
      <Field label={t('prefs.password.new')}>
        {(id) => <Password id={id} value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} autoComplete="new-password" />}
      </Field>
      <Field label={t('prefs.password.confirm')}>
        {(id) => <Password id={id} value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} autoComplete="new-password" />}
      </Field>
      <PolicyChecklist policy={policy} password={pw.next} username={user.username} confirm={pw.confirm} t={t} />
    </ProfileCard>
  )
}
