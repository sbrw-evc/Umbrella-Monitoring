import { BellRing, Copy, KeyRound, Plus, Search, Send, ShieldCheck, Trash2, Users } from 'lucide-react'
import { useMemo, useState } from 'react'
import {
  api,
  ciTypeLabel,
  fmtTime,
  sevLabel,
  SEVERITIES,
  type APIToken,
  type Channel,
  type ChannelType,
  type CI,
  type Delivery,
  type NotifyEvent,
  type Perm,
  type Role,
  type Severity,
  type User,
} from '../api'
import { roleName } from '../components/Layout'
import { Empty, Field, Modal, PageHeader, SevBadge, Tabs } from '../components/ui'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'

const permLabel = (p: string) => t(`roles.perm.${p.replace('.', '_')}`)

// useServices lists the CIs users and channels can be bound to.
function useServices() {
  const { can } = useApp()
  const { data } = useFetch<{ items: CI[] }>(can('cmdb.view') ? '/api/cis' : null)
  return useMemo(() => (data?.items ?? []).filter((c) => c.type === 'business_service' || c.type === 'it_service').sort((a, b) => a.type.localeCompare(b.type) || a.name.localeCompare(b.name)), [data])
}

// Chips is a multi-select of toggle chips.
function Chips<T extends string>({ items, value, onChange, disabled }: { items: { id: T; title: string; hint?: string }[]; value: T[]; onChange: (v: T[]) => void; disabled?: boolean }) {
  return (
    <div className="chips">
      {items.map((it) => {
        const on = value.includes(it.id)
        return (
          <button
            type="button"
            key={it.id}
            className={`chip ${on ? 'chip-on' : ''}`}
            title={it.hint}
            disabled={disabled}
            onClick={() => onChange(on ? value.filter((x) => x !== it.id) : [...value, it.id])}
          >
            {it.title}
          </button>
        )
      })}
    </div>
  )
}

function confirmDo(text: string) {
  return window.confirm(text)
}

// ---- users ----

export function UsersPage() {
  const { data, reload } = useFetch<{ items: User[] }>('/api/users')
  const roles = useFetch<{ items: Role[] }>('/api/roles').data?.items ?? []
  const services = useServices()
  const [q, setQ] = useState('')
  const [edit, setEdit] = useState<User | 'new' | null>(null)
  useLive(['users'], reload)
  const svcName = (id: string) => services.find((c) => c.id === id)?.name ?? id
  const roleOf = (id: string) => roles.find((r) => r.id === id)
  const items = (data?.items ?? []).filter((u) => !q || `${u.username} ${u.name ?? ''} ${u.email ?? ''}`.toLowerCase().includes(q.toLowerCase()))
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('users.header.title')}
          sub={t('users.header.sub')}
          actions={
            <button className="btn btn-primary" onClick={() => setEdit('new')}>
              <Plus size={15} /> {t('users.header.add')}
            </button>
          }
        />
        <div className="filterbar">
          <div className="search">
            <Search size={15} />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('users.filters.search')} />
          </div>
        </div>
        <div className="card card-flush">
          <table className="table">
            <thead>
              <tr>
                <th>{t('users.table.user')}</th>
                <th>{t('users.table.roles')}</th>
                <th>{t('users.table.services')}</th>
                <th>{t('users.table.state')}</th>
                <th>{t('users.table.lastLogin')}</th>
              </tr>
            </thead>
            <tbody className="stagger">
              {items.map((u) => {
                const all = u.roles.some((r) => roleOf(r)?.all_services)
                const locked = u.locked_until && new Date(u.locked_until) > new Date()
                return (
                  <tr key={u.id} className="row-click" onClick={() => setEdit(u)}>
                    <td>
                      <div className="user-cell">
                        <span className="avatar avatar-sm">{(u.name || u.username).slice(0, 1).toUpperCase()}</span>
                        <div>
                          <b>{u.name || u.username}</b>
                          <div className="muted mono">
                            {u.username}
                            {u.email ? ` · ${u.email}` : ''}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td>
                      <div className="tags">
                        {u.roles.map((r) => (
                          <span key={r} className="tag">
                            {roleName(r, roleOf(r)?.name)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>
                      {all ? (
                        <span className="muted">{t('users.allServices')}</span>
                      ) : (u.business_services ?? []).length === 0 ? (
                        <span className="muted">{t('users.noServices')}</span>
                      ) : (
                        <div className="tags">
                          {(u.business_services ?? []).map((id) => (
                            <span key={id} className="tag tag-link">
                              {svcName(id)}
                            </span>
                          ))}
                        </div>
                      )}
                    </td>
                    <td>
                      <div className="tags">
                        {u.disabled ? (
                          <span className="tag tag-red">{t('users.state.disabled')}</span>
                        ) : locked ? (
                          <span className="tag tag-warn">{t('users.state.locked')}</span>
                        ) : (
                          <span className="tag tag-ok">{t('users.state.active')}</span>
                        )}
                        {u.service && <span className="tag">{t('users.state.service')}</span>}
                        {u.must_change_password && !u.service && <span className="tag tag-warn">{t('users.state.mustChange')}</span>}
                      </div>
                    </td>
                    <td className="nowrap muted">{fmtTime(u.last_login_at)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
      {edit && (
        <UserModal
          user={edit === 'new' ? null : edit}
          roles={roles}
          services={services}
          onClose={() => setEdit(null)}
          onDone={() => {
            setEdit(null)
            reload()
          }}
        />
      )}
    </div>
  )
}

function UserModal({ user, roles, services, onClose, onDone }: { user: User | null; roles: Role[]; services: CI[]; onClose: () => void; onDone: () => void }) {
  const { toast, me } = useApp()
  const [username, setUsername] = useState(user?.username ?? '')
  const [name, setName] = useState(user?.name ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [password, setPassword] = useState('')
  const [service, setService] = useState(user?.service ?? false)
  const [userRoles, setRoles] = useState<string[]>(user?.roles ?? ['viewer'])
  const [bound, setBound] = useState<string[]>(user?.business_services ?? [])
  const [disabled, setDisabled] = useState(user?.disabled ?? false)
  const [error, setError] = useState('')
  const [sub, setSub] = useState<'' | 'reset' | 'tokens'>('')
  const scoped = userRoles.length > 0 && !userRoles.some((r) => roles.find((x) => x.id === r)?.all_services)

  const save = async () => {
    setError('')
    try {
      if (user) {
        await api.put(`/api/users/${user.id}`, {
          name,
          email,
          roles: userRoles,
          business_services: bound,
          disabled,
        })
        toast(t('users.actions.saved'))
      } else {
        await api.post('/api/users', {
          username,
          name,
          email,
          roles: userRoles,
          business_services: bound,
          service,
          password: service ? '' : password,
        })
        toast(t('users.actions.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const remove = async () => {
    if (!user || !confirmDo(t('users.actions.confirmDelete', { name: user.username }))) return
    try {
      await api.del(`/api/users/${user.id}`)
      toast(t('users.actions.deleted'))
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  if (user && sub === 'reset') return <ResetModal user={user} onClose={() => setSub('')} />
  if (user && sub === 'tokens') return <TokensModal user={user} onClose={() => setSub('')} />
  return (
    <Modal
      title={user ? t('users.form.editTitle', { name: user.username }) : t('users.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {user && user.id !== me?.user.id && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('users.actions.delete')}
              </button>
            </div>
          )}
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={!username || userRoles.length === 0 || (!user && !service && !password)}>
            {user ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      {user && (
        <div className="modal-tools">
          {!user.service && (
            <button className="btn btn-sm" onClick={() => setSub('reset')}>
              <KeyRound size={13} /> {t('users.actions.resetPassword')}
            </button>
          )}
          <button className="btn btn-sm" onClick={() => setSub('tokens')}>
            {t('users.actions.tokens')}
            {user.tokens ? ` · ${user.tokens}` : ''}
          </button>
        </div>
      )}
      <div className="row2">
        <Field label={t('users.form.username')}>
          <input value={username} onChange={(e) => setUsername(e.target.value)} disabled={!!user} autoFocus={!user} />
        </Field>
        <Field label={t('users.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      </div>
      <Field label={t('users.form.email')}>
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      {!user && (
        <label className="check check-wrap">
          <input type="checkbox" checked={service} onChange={(e) => setService(e.target.checked)} />
          {t('users.form.service')}
          <span className="muted">· {t('users.form.serviceHelp')}</span>
        </label>
      )}
      {!user && !service && (
        <Field label={t('users.form.password')} help={`${t('users.form.passwordHelp')}. ${t('auth.policy')}`}>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </Field>
      )}
      <Field label={t('users.form.roles')}>
        <Chips
          items={roles.map((r) => ({
            id: r.id,
            title: roleName(r.id, r.name),
            hint: r.description,
          }))}
          value={userRoles}
          onChange={setRoles}
        />
      </Field>
      <Field label={t('users.form.services')} help={t('users.form.servicesHelp')}>
        {services.length === 0 ? (
          <span className="muted">{t('users.form.noBusiness')}</span>
        ) : (
          <div className={scoped ? '' : 'dimmed'}>
            <Chips
              items={services.map((c) => ({
                id: c.id,
                title: c.name,
                hint: ciTypeLabel(c.type),
              }))}
              value={bound}
              onChange={setBound}
            />
          </div>
        )}
      </Field>
      {user && (
        <label className="check check-wrap">
          <input type="checkbox" checked={disabled} onChange={(e) => setDisabled(e.target.checked)} />
          {t('users.form.disabled')}
        </label>
      )}
      {error && <div className="form-error shake">{error}</div>}
    </Modal>
  )
}

function ResetModal({ user, onClose }: { user: User; onClose: () => void }) {
  const { toast } = useApp()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      await api.post(`/api/users/${user.id}/password`, { password })
      toast(t('users.actions.reset'))
      onClose()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  return (
    <Modal
      title={t('users.reset.title', { name: user.username })}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={submit} disabled={!password}>
            {t('users.actions.resetPassword')}
          </button>
        </>
      }
    >
      <Field label={t('users.reset.password')} help={t('auth.policy')}>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" autoFocus />
      </Field>
      {error && <div className="form-error shake">{error}</div>}
    </Modal>
  )
}

export function TokensModal({ user, onClose }: { user: User; onClose: () => void }) {
  const { toast } = useApp()
  const { data, reload } = useFetch<{ items: APIToken[] }>(`/api/users/${user.id}/tokens`)
  const [name, setName] = useState('')
  const [days, setDays] = useState(90)
  const [fresh, setFresh] = useState('')
  const [error, setError] = useState('')
  const create = async () => {
    try {
      const r = await api.post<{ token: string }>(`/api/users/${user.id}/tokens`, { name, days })
      setFresh(r.token)
      setName('')
      reload()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const revoke = async (id: string) => {
    await api.del(`/api/users/${user.id}/tokens/${id}`)
    toast(t('users.tokens.revoked'))
    reload()
  }
  const copy = () => {
    navigator.clipboard?.writeText(fresh).then(
      () => toast(t('users.tokens.copied')),
      () => undefined,
    )
  }
  const items = data?.items ?? []
  return (
    <Modal title={t('users.tokens.title', { name: user.username })} onClose={onClose}>
      <div className="muted">{t('users.tokens.sub')}</div>
      {fresh && (
        <div className="token-fresh pop-in">
          <div>{t('users.tokens.created')}</div>
          <div className="token-row">
            <code className="mono">{fresh}</code>
            <button className="btn btn-sm" onClick={copy}>
              <Copy size={13} /> {t('users.tokens.copy')}
            </button>
          </div>
        </div>
      )}
      <div className="row2 token-form">
        <Field label={t('users.tokens.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="grafana" />
        </Field>
        <Field label={t('users.tokens.days')}>
          <input type="number" min={0} max={3650} value={days} onChange={(e) => setDays(Number(e.target.value))} />
        </Field>
      </div>
      <button className="btn btn-primary" onClick={create} disabled={!name.trim()}>
        <KeyRound size={14} /> {t('users.tokens.create')}
      </button>
      {error && <div className="form-error shake">{error}</div>}
      {items.length === 0 ? (
        <Empty>{t('users.tokens.none')}</Empty>
      ) : (
        <table className="table table-compact">
          <thead>
            <tr>
              <th>{t('users.tokens.name')}</th>
              <th>{t('users.tokens.prefix')}</th>
              <th>{t('users.tokens.createdAt')}</th>
              <th>{t('users.tokens.expires')}</th>
              <th>{t('users.tokens.lastUsed')}</th>
              <th />
            </tr>
          </thead>
          <tbody className="stagger">
            {items.map((x) => (
              <tr key={x.id}>
                <td>{x.name}</td>
                <td className="mono">{x.prefix}…</td>
                <td className="nowrap">{fmtTime(x.created_at)}</td>
                <td className="nowrap">{x.expires_at ? fmtTime(x.expires_at) : '∞'}</td>
                <td className="nowrap">{x.last_used_at ? fmtTime(x.last_used_at) : t('users.tokens.never')}</td>
                <td className="right">
                  <button className="btn btn-sm btn-ghost text-danger" onClick={() => revoke(x.id)}>
                    {t('users.tokens.revoke')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Modal>
  )
}

// ---- roles ----

interface RoleList {
  items: Role[]
  users: Record<string, number>
  permissions: Perm[]
}

export function RolesPage() {
  const { data, reload } = useFetch<RoleList>('/api/roles')
  const [edit, setEdit] = useState<Role | 'new' | null>(null)
  const perms = data?.permissions ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('roles.header.title')}
          sub={t('roles.header.sub')}
          actions={
            <button className="btn btn-primary" onClick={() => setEdit('new')}>
              <Plus size={15} /> {t('roles.header.add')}
            </button>
          }
        />
        <div className="role-grid stagger">
          {(data?.items ?? []).map((r) => (
            <button key={r.id} className="card role-card" onClick={() => setEdit(r)}>
              <div className="role-head">
                <ShieldCheck size={18} />
                <b>{roleName(r.id, r.name)}</b>
                {r.built_in && <span className="tag">{t('roles.builtIn')}</span>}
                <span className="role-users" title={t('roles.table.users')}>
                  <Users size={13} /> {data?.users[r.id] ?? 0}
                </span>
              </div>
              {r.description && <div className="muted role-desc">{r.description}</div>}
              <div className="role-scope">
                {t('roles.table.scope')}: <b>{r.all_services ? t('roles.scope.all') : t('roles.scope.own')}</b>
              </div>
              <div className="perm-dots">
                {perms.map((p) => (
                  <span key={p} className={`perm-dot ${r.permissions.includes(p) ? 'perm-on' : ''}`} title={permLabel(p)} />
                ))}
              </div>
            </button>
          ))}
        </div>
      </div>
      {edit && (
        <RoleModal
          role={edit === 'new' ? null : edit}
          perms={perms}
          onClose={() => setEdit(null)}
          onDone={() => {
            setEdit(null)
            reload()
          }}
        />
      )}
    </div>
  )
}

function RoleModal({ role, perms, onClose, onDone }: { role: Role | null; perms: Perm[]; onClose: () => void; onDone: () => void }) {
  const { toast } = useApp()
  const [id, setId] = useState(role?.id ?? '')
  const [name, setName] = useState(role ? roleName(role.id, role.name) : '')
  const [description, setDescription] = useState(role?.description ?? '')
  const [all, setAll] = useState(role?.all_services ?? true)
  const [sel, setSel] = useState<Perm[]>(role?.permissions ?? ['incidents.view'])
  const [error, setError] = useState('')
  const locked = role?.id === 'admin'
  const save = async () => {
    setError('')
    try {
      const body = { name, description, all_services: all, permissions: sel }
      if (role) {
        await api.put(`/api/roles/${role.id}`, body)
        toast(t('roles.actions.saved'))
      } else {
        await api.post('/api/roles', { id, ...body })
        toast(t('roles.actions.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const remove = async () => {
    if (!role || !confirmDo(t('roles.actions.confirmDelete', { name }))) return
    try {
      await api.del(`/api/roles/${role.id}`)
      toast(t('roles.actions.deleted'))
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const toggle = (p: Perm) => setSel((s) => (s.includes(p) ? s.filter((x) => x !== p) : [...s, p]))
  return (
    <Modal
      title={role ? t('roles.form.editTitle', { name }) : t('roles.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {role && !role.built_in && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('roles.actions.delete')}
              </button>
            </div>
          )}
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={!name || (!role && !id)}>
            {role ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      <div className="row2">
        <Field label={t('roles.form.id')} help={role ? undefined : t('roles.form.idHelp')}>
          <input value={id} onChange={(e) => setId(e.target.value)} disabled={!!role} className="mono" autoFocus={!role} />
        </Field>
        <Field label={t('roles.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      </div>
      <Field label={t('roles.form.description')}>
        <input value={description} onChange={(e) => setDescription(e.target.value)} />
      </Field>
      <label className="check check-wrap">
        <input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} disabled={locked} />
        {t('roles.form.allServices')}
        <span className="muted">· {t('roles.form.allServicesHelp')}</span>
      </label>
      <Field label={t('roles.form.perms')} help={locked ? t('roles.form.adminLocked') : undefined}>
        <div className="perm-grid">
          {perms.map((p) => (
            <label key={p} className={`perm-item ${sel.includes(p) ? 'perm-item-on' : ''}`}>
              <input type="checkbox" checked={sel.includes(p)} onChange={() => toggle(p)} disabled={locked} />
              <span>
                {permLabel(p)}
                <code className="mono muted">{p}</code>
              </span>
            </label>
          ))}
        </div>
      </Field>
      {error && <div className="form-error shake">{error}</div>}
    </Modal>
  )
}

// ---- notifications ----

interface ChannelList {
  items: Channel[]
  events: NotifyEvent[]
  allow_http: boolean
}

const TYPE_ICON: Record<ChannelType, string> = { teams: 'T', zoom: 'Z' }

export function NotificationsPage() {
  const { toast } = useApp()
  const [tab, setTab] = useState<'channels' | 'log'>('channels')
  const { data, reload } = useFetch<ChannelList>('/api/channels')
  const log = useFetch<{ items: Delivery[] }>(tab === 'log' ? '/api/deliveries' : null)
  const services = useServices()
  const [edit, setEdit] = useState<Channel | 'new' | null>(null)
  const [testing, setTesting] = useState('')
  useLive(
    ['delivery', 'alert'],
    () => {
      reload()
      if (tab === 'log') log.reload()
    },
    1500,
  )
  const test = async (c: Channel) => {
    setTesting(c.id)
    try {
      const d = await api.post<Delivery>(`/api/channels/${c.id}/test`)
      if (d.ok) toast(t('notifications.actions.testOk', { status: d.status }))
      else toast(t('notifications.actions.testFail', { error: d.error ?? d.status }), 'error')
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setTesting('')
    }
  }
  const channels = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('notifications.header.title')}
          sub={t('notifications.header.sub')}
          actions={
            <button className="btn btn-primary" onClick={() => setEdit('new')}>
              <Plus size={15} /> {t('notifications.header.add')}
            </button>
          }
        />
        <Tabs
          tabs={[
            { id: 'channels', title: t('notifications.tabs.channels') },
            { id: 'log', title: t('notifications.tabs.log') },
          ]}
          value={tab}
          onChange={setTab}
        />
        {data?.allow_http && <div className="hint hint-warn">{t('notifications.httpWarning')}</div>}
        {tab === 'channels' ? (
          channels.length === 0 ? (
            <div className="card">
              <Empty>
                <BellRing size={28} />
                <div>{t('notifications.empty')}</div>
              </Empty>
            </div>
          ) : (
            <div className="channel-grid stagger">
              {channels.map((c) => (
                <div key={c.id} className={`card channel-card ${c.enabled ? '' : 'channel-off'}`}>
                  <div className="channel-head">
                    <span className={`channel-logo channel-${c.type}`}>{TYPE_ICON[c.type]}</span>
                    <div className="channel-title">
                      <b>{c.name}</b>
                      <small className="muted">
                        {t(`notifications.type.${c.type}`)} · {c.url_hint || c.url_ref || '—'}
                      </small>
                    </div>
                    <span className={`dot ${!c.last_at ? 'dot-info' : c.last_error ? 'dot-critical' : 'dot-ok'}`} />
                  </div>
                  <div className="channel-props">
                    <span>
                      {t('notifications.table.mode')}: <b>{t(`notifications.mode.${c.mode}`)}</b>
                    </span>
                    <span>
                      {t('notifications.table.filter')}:{' '}
                      {t('notifications.filter.from', {
                        sev: sevLabel(c.min_severity),
                      })}
                      ,{' '}
                      {(c.services ?? []).length === 0
                        ? t('notifications.filter.allServices')
                        : t('notifications.filter.services', {
                            n: c.services!.length,
                          })}
                    </span>
                    <div className="tags">
                      {c.events.map((e) => (
                        <span key={e} className="tag">
                          {t(`notifications.event.${e}`)}
                        </span>
                      ))}
                    </div>
                    <span className="muted">
                      {t('notifications.table.delivery')}: {c.sent} / <span className={c.failed ? 'text-danger' : ''}>{c.failed}</span>
                      {c.last_at && ` · ${t('notifications.table.last')} ${fmtTime(c.last_at)}`}
                    </span>
                    {c.last_error && <span className="text-danger channel-error">{c.last_error}</span>}
                  </div>
                  <div className="channel-actions">
                    <button className="btn btn-sm" onClick={() => test(c)} disabled={testing === c.id}>
                      <Send size={13} className={testing === c.id ? 'fly' : ''} /> {t('notifications.actions.test')}
                    </button>
                    <button className="btn btn-sm btn-ghost" onClick={() => setEdit(c)}>
                      {t('common.actions.edit')}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : (
          <div className="card card-flush">
            {(log.data?.items ?? []).length === 0 ? (
              <Empty>{t('notifications.noLog')}</Empty>
            ) : (
              <table className="table table-compact">
                <thead>
                  <tr>
                    <th>{t('notifications.table.time')}</th>
                    <th>{t('notifications.table.channel')}</th>
                    <th>{t('notifications.table.incident')}</th>
                    <th>{t('notifications.table.event')}</th>
                    <th>{t('notifications.table.result')}</th>
                    <th>{t('notifications.table.attempts')}</th>
                  </tr>
                </thead>
                <tbody className="stagger">
                  {log.data!.items.map((d) => (
                    <tr key={d.id}>
                      <td className="nowrap">{fmtTime(d.at)}</td>
                      <td>{d.channel}</td>
                      <td className="mono">{d.alert_id === 'TEST' ? '—' : d.alert_id}</td>
                      <td>{t(`notifications.event.${d.event}`)}</td>
                      <td>
                        {d.ok ? (
                          <span className="tag tag-ok">
                            {t('notifications.result.ok')} {d.status}
                          </span>
                        ) : (
                          <span className="tag tag-red" title={d.error}>
                            {t('notifications.result.fail')} {d.status || ''}
                          </span>
                        )}
                        {d.error && <div className="muted channel-error">{d.error}</div>}
                      </td>
                      <td className="num">{d.attempts}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}
      </div>
      {edit && (
        <ChannelModal
          channel={edit === 'new' ? null : edit}
          events={data?.events ?? ['open', 'escalate', 'ack', 'resolve', 'fallback']}
          services={services}
          onClose={() => setEdit(null)}
          onDone={() => {
            setEdit(null)
            reload()
          }}
        />
      )}
    </div>
  )
}

function ChannelModal({ channel, events, services, onClose, onDone }: { channel: Channel | null; events: NotifyEvent[]; services: CI[]; onClose: () => void; onDone: () => void }) {
  const { toast } = useApp()
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'teams')
  const [url, setUrl] = useState('')
  const [urlRef, setUrlRef] = useState(channel?.url_ref ?? '')
  const [token, setToken] = useState('')
  const [tokenRef, setTokenRef] = useState(channel?.token_ref ?? '')
  const [mode, setMode] = useState(channel?.mode ?? 'always')
  const [minSev, setMinSev] = useState<Severity>(channel?.min_severity ?? 'error')
  const [evs, setEvs] = useState<NotifyEvent[]>(channel?.events ?? ['open', 'escalate', 'resolve'])
  const [svcs, setSvcs] = useState<string[]>(channel?.services ?? [])
  const [enabled, setEnabled] = useState(channel?.enabled ?? true)
  const [error, setError] = useState('')
  const save = async () => {
    setError('')
    const body: Record<string, unknown> = {
      name,
      type,
      mode,
      min_severity: minSev,
      events: evs,
      services: svcs,
      enabled,
      url_ref: urlRef,
      token_ref: tokenRef,
    }
    if (url) body.url = url
    if (token) body.token = token
    try {
      if (channel) {
        await api.put(`/api/channels/${channel.id}`, body)
        toast(t('notifications.actions.saved'))
      } else {
        await api.post('/api/channels', body)
        toast(t('notifications.actions.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const remove = async () => {
    if (!channel || !confirmDo(t('notifications.actions.confirmDelete', { name: channel.name }))) return
    await api.del(`/api/channels/${channel.id}`)
    toast(t('notifications.actions.deleted'))
    onDone()
  }
  return (
    <Modal
      title={channel ? t('notifications.form.editTitle', { name: channel.name }) : t('notifications.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {channel && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('notifications.actions.delete')}
              </button>
            </div>
          )}
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={!name || (!channel && !url && !urlRef) || evs.length === 0}>
            {channel ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      <div className="row2">
        <Field label={t('notifications.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus={!channel} />
        </Field>
        <Field label={t('notifications.form.type')}>
          <div className="seg">
            {(['teams', 'zoom'] as ChannelType[]).map((x) => (
              <button type="button" key={x} className={`seg-btn ${type === x ? 'seg-active' : ''}`} onClick={() => setType(x)} disabled={!!channel}>
                {t(`notifications.type.${x}`)}
              </button>
            ))}
          </div>
        </Field>
      </div>
      <Field label={t('notifications.form.url')} help={type === 'teams' ? t('notifications.form.urlHelpTeams') : t('notifications.form.urlHelpZoom')}>
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder={
            channel?.url_ref
              ? t('notifications.form.urlKeep', {
                  host: channel.url_hint ?? '',
                })
              : 'https://…'
          }
          className="mono"
          autoComplete="off"
        />
      </Field>
      <Field label={t('notifications.form.urlRef')} help={t('notifications.form.urlRefHelp')}>
        <input value={urlRef} onChange={(e) => setUrlRef(e.target.value)} placeholder="openbao://umbrella/notify/teams-ops#url" className="mono" />
      </Field>
      {type === 'zoom' && (
        <div className="row2">
          <Field label={t('notifications.form.token')}>
            <input type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder={channel?.token_ref ? t('notifications.form.tokenKeep') : ''} autoComplete="off" />
          </Field>
          <Field label={t('notifications.form.tokenRef')}>
            <input value={tokenRef} onChange={(e) => setTokenRef(e.target.value)} placeholder="openbao://umbrella/notify/zoom#token" className="mono" />
          </Field>
        </div>
      )}
      <div className="row2">
        <Field label={t('notifications.form.mode')}>
          <select value={mode} onChange={(e) => setMode(e.target.value as Channel['mode'])}>
            <option value="always">{t('notifications.mode.always')}</option>
            <option value="fallback">{t('notifications.mode.fallback')}</option>
          </select>
        </Field>
        <Field label={t('notifications.form.minSeverity')}>
          <select value={minSev} onChange={(e) => setMinSev(e.target.value as Severity)}>
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {sevLabel(s)}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="sev-preview">
        {SEVERITIES.filter((s) => SEVERITIES.indexOf(s) <= SEVERITIES.indexOf(minSev)).map((s) => (
          <SevBadge key={s} sev={s} />
        ))}
      </div>
      <Field label={t('notifications.form.events')}>
        <Chips
          items={events.map((e) => ({
            id: e,
            title: t(`notifications.event.${e}`),
          }))}
          value={evs}
          onChange={setEvs}
        />
      </Field>
      <Field label={t('notifications.form.services')} help={t('notifications.form.servicesHelp')}>
        <Chips
          items={services.map((c) => ({
            id: c.id,
            title: c.name,
            hint: ciTypeLabel(c.type),
          }))}
          value={svcs}
          onChange={setSvcs}
        />
      </Field>
      <label className="check check-wrap">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        {t('notifications.form.enabled')}
      </label>
      {error && <div className="form-error shake">{error}</div>}
    </Modal>
  )
}
