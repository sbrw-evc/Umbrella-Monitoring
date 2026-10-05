import { useEffect, useState, type ReactNode } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { KeyRound, Lock, Trash2, Unlock } from 'lucide-react'
import { api } from '../../api'
import { Avatar } from '../../Avatar'
import { useLocale, useT } from '../../i18n'
import { Banner, Button, formatDate, Modal, Rows, SettingRow } from '../../ui'
import { ProfileFieldsGrid } from '../profile/ProfileFieldsGrid'
import { useAction, type Action } from '../profile/useAction'
import { useSession } from '../session'
import { profileChanged, profileOf, type ProfileFields } from '../types'
import { AccessFields, type Access } from './AccessFields'
import { SourcePill, StatusPill } from './Badges'
import { adminOf, type ManagedUser, type Refs } from './model'
import { freshPassword, NewPasswordFields, usePasswordValid, type NewPassword } from './NewPasswordFields'
import { strings } from './strings'

type Mode = 'edit' | 'password' | 'delete'

const accessOf = (u: ManagedUser): Access => ({ role_id: u.role, team_id: u.team_id ?? '' })

export function UserDialog({
  user,
  refs,
  onClose,
  onChanged,
  onDeleted,
}: {
  user: ManagedUser | null
  refs: Refs
  onClose: () => void
  onChanged: (u: ManagedUser) => void
  onDeleted: (u: ManagedUser) => void
}) {
  const t = useT(strings)
  const [mode, setMode] = useState<Mode>('edit')
  const [flash, setFlash] = useState('')
  const [last, setLast] = useState(user)
  const shown = user ?? last
  const name = shown?.display_name ?? ''
  const titles: Record<Mode, string> = {
    edit: name,
    password: t('usr.password.title', { name }),
    delete: t('usr.delete.title', { name }),
  }

  useEffect(() => {
    if (user) setLast(user)
  }, [user])

  useEffect(() => {
    setMode('edit')
    setFlash('')
  }, [user?.id])

  const switchTo = (m: Mode) => {
    setFlash('')
    setMode(m)
  }

  return (
    <Modal open={!!user} title={titles[mode]} onClose={onClose}>
      {shown && (
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={mode}
            className="stack"
            initial={{ opacity: 0, x: mode === 'edit' ? -12 : 12 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: mode === 'edit' ? 12 : -12 }}
            transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
          >
            {mode === 'edit' && <EditView user={shown} refs={refs} flash={flash} onChanged={onChanged} onMode={switchTo} />}
            {mode === 'password' && (
              <PasswordView
                user={shown}
                onBack={() => switchTo('edit')}
                onDone={(u) => {
                  onChanged(u)
                  setMode('edit')
                  setFlash(t('usr.password.set'))
                }}
              />
            )}
            {mode === 'delete' && <DeleteView user={shown} onBack={() => switchTo('edit')} onDeleted={onDeleted} />}
          </motion.div>
        </AnimatePresence>
      )}
    </Modal>
  )
}

function Outcome({ action, flash }: { action: Action; flash?: string }) {
  const notice = action.notice || (action.error || action.busy ? '' : flash)
  return (
    <>
      {notice && <Banner kind="ok" title={notice} />}
      {action.error && (
        <Banner kind="error" title={action.error.message}>
          {action.error.detail}
        </Banner>
      )}
    </>
  )
}

function Footer({ children }: { children: ReactNode }) {
  return <div className="modal-foot">{children}</div>
}

function useRights(user: ManagedUser) {
  const { can, user: me } = useSession()
  const self = user.id === me.id
  const manageable = !adminOf(user) || adminOf(me)
  const local = user.source === 'local'
  return {
    self,
    manageable,
    edit: manageable && can('users:edit'),
    lock: manageable && !self && can('users:lock'),
    password: manageable && local && can('users:password'),
    remove: manageable && !self && can('users:delete'),
  }
}

function UserSummary({ user }: { user: ManagedUser }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const date = (v?: string) => formatDate(v, locale, timezone)
  return (
    <>
      <div className="usr-summary">
        <Avatar user={user} size={56} />
        <div className="usr-names">
          <strong>{user.display_name}</strong>
          <span className="muted">{user.username}</span>
          <span className="row usr-pills">
            <SourcePill user={user} />
            <StatusPill user={user} />
          </span>
        </div>
      </div>
      <Rows
        rows={[
          [t('usr.field.created'), date(user.created_at)],
          [t('usr.col.lastLogin'), date(user.last_login_at) || t('usr.never')],
          ...(user.source === 'local'
            ? ([
                [t('usr.field.passwordChanged'), date(user.password_changed_at)],
                [t('usr.field.passwordExpires'), date(user.password_expires_at)],
              ] as [string, string][])
            : []),
        ]}
      />
    </>
  )
}

function EditView({
  user,
  refs,
  flash,
  onChanged,
  onMode,
}: {
  user: ManagedUser
  refs: Refs
  flash: string
  onChanged: (u: ManagedUser) => void
  onMode: (m: Mode) => void
}) {
  const t = useT(strings)
  const action = useAction(strings)
  const rights = useRights(user)
  const local = user.source === 'local'
  const [profile, setProfile] = useState<ProfileFields>(() => profileOf(user))
  const [access, setAccess] = useState<Access>(() => accessOf(user))

  useEffect(() => {
    setProfile(profileOf(user))
    setAccess(accessOf(user))
  }, [user])

  const saved = accessOf(user)
  const dirty = (local && profileChanged(profile, profileOf(user))) || access.role_id !== saved.role_id || access.team_id !== saved.team_id

  const save = () =>
    action.run(async () => {
      const body = { ...(local ? { profile } : {}), role_id: access.role_id, team_id: access.team_id }
      onChanged(await api<ManagedUser>('PUT', `/api/users/${encodeURIComponent(user.id)}`, body))
      return t('usr.saved')
    })

  const toggleLock = () =>
    action.run(async () => {
      const verb = user.disabled ? 'unlock' : 'lock'
      onChanged(await api<ManagedUser>('POST', `/api/users/${encodeURIComponent(user.id)}/${verb}`))
      return t(user.disabled ? 'usr.unlocked' : 'usr.locked')
    })

  const roleHint = rights.self ? t('usr.self.role') : !local ? t('usr.ldap.admin') : undefined

  return (
    <>
      <UserSummary user={user} />
      {!rights.manageable && <Banner kind="info" title={t('usr.adminOnly')} />}
      <section className="stack usr-section">
        <h3>{t('usr.section.profile')}</h3>
        {!local && <p className="muted">{t('usr.ldap.profile')}</p>}
        <ProfileFieldsGrid value={profile} onChange={setProfile} editable={local && rights.edit} />
      </section>
      <section className="stack usr-section">
        <h3>{t('usr.section.access')}</h3>
        <fieldset className="plain-fieldset" disabled={!rights.edit}>
          <AccessFields refs={refs} value={access} onChange={setAccess} roleLocked={rights.self} roleHint={roleHint} />
        </fieldset>
      </section>
      {(rights.lock || rights.password || rights.remove) && (
        <section className="stack usr-section">
          <h3>{t('usr.section.actions')}</h3>
          <div className="setting-rows">
            {rights.lock && (
              <SettingRow label={t(user.disabled ? 'usr.unlock' : 'usr.lock')} hint={t(user.disabled ? 'usr.unlock.hint' : 'usr.lock.hint')}>
                <Button onClick={toggleLock} busy={action.busy}>
                  {user.disabled ? <Unlock size={16} aria-hidden /> : <Lock size={16} aria-hidden />}
                  {t(user.disabled ? 'usr.unlock' : 'usr.lock')}
                </Button>
              </SettingRow>
            )}
            {rights.password && (
              <SettingRow label={t('usr.password')} hint={t('usr.password.hint')}>
                <Button onClick={() => onMode('password')}>
                  <KeyRound size={16} aria-hidden />
                  {t('usr.password')}
                </Button>
              </SettingRow>
            )}
            {rights.remove && (
              <SettingRow label={t('usr.delete')} hint={t('usr.delete.hint')}>
                <Button className="usr-danger" onClick={() => onMode('delete')}>
                  <Trash2 size={16} aria-hidden />
                  {t('usr.delete')}
                </Button>
              </SettingRow>
            )}
          </div>
        </section>
      )}
      <Outcome action={action} flash={flash} />
      {rights.edit && (
        <Footer>
          <Button variant="primary" onClick={save} busy={action.busy} disabled={!dirty}>
            {t('usr.save')}
          </Button>
        </Footer>
      )}
    </>
  )
}

function PasswordView({ user, onBack, onDone }: { user: ManagedUser; onBack: () => void; onDone: (u: ManagedUser) => void }) {
  const t = useT(strings)
  const action = useAction(strings)
  const { refresh, user: me } = useSession()
  const [value, setValue] = useState<NewPassword>(freshPassword)
  const valid = usePasswordValid(value, user.username)

  const submit = () =>
    action.run(async () => {
      onDone(await api<ManagedUser>('PUT', `/api/users/${encodeURIComponent(user.id)}/password`, value))
      if (user.id === me.id) await refresh()
    })

  return (
    <>
      <NewPasswordFields value={value} onChange={setValue} username={user.username} />
      <Outcome action={action} />
      <Footer>
        <Button variant="ghost" onClick={onBack}>
          {t('usr.back')}
        </Button>
        <Button variant="primary" onClick={submit} busy={action.busy} disabled={!valid}>
          {t('usr.password')}
        </Button>
      </Footer>
    </>
  )
}

function DeleteView({ user, onBack, onDeleted }: { user: ManagedUser; onBack: () => void; onDeleted: (u: ManagedUser) => void }) {
  const t = useT(strings)
  const action = useAction(strings)
  const remove = () =>
    action.run(async () => {
      await api('DELETE', `/api/users/${encodeURIComponent(user.id)}`)
      onDeleted(user)
    })
  return (
    <>
      <Banner kind="warn" title={t('usr.delete.text', { login: user.username })}>
        {user.source === 'ldap' && t('usr.delete.ldap')}
      </Banner>
      <Outcome action={action} />
      <Footer>
        <Button variant="ghost" onClick={onBack}>
          {t('usr.back')}
        </Button>
        <Button className="usr-danger-solid" onClick={remove} busy={action.busy}>
          <Trash2 size={16} aria-hidden />
          {t('usr.delete.confirm')}
        </Button>
      </Footer>
    </>
  )
}
