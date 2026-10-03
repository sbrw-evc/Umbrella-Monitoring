import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api, ApiError, type TeamView, type User } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { Empty, Field, Modal, PageHeader } from '../components/ui'

export function TeamsPage() {
  const { can, toast, reloadMeta } = useApp()
  const { data, reload } = useFetch<{ items: TeamView[] }>('/api/teams')
  const users = useFetch<{ items: User[] }>(can('users.admin') ? '/api/users' : null).data?.items ?? []
  const [edit, setEdit] = useState<TeamView | 'new' | null>(null)
  const [preset, setPreset] = useState('')
  useLive(['alert'], reload, 3000)
  const admin = can('users.admin')
  const userName = (id: string) => {
    const u = users.find((x) => x.id === id)
    return u ? u.name || u.username : id
  }
  const items = data?.items ?? []
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('teams.header.title')}
          sub={t('teams.header.sub')}
          actions={
            admin && (
              <button
                className="btn btn-primary"
                onClick={() => {
                  setPreset('')
                  setEdit('new')
                }}
              >
                <Plus size={15} /> {t('teams.header.add')}
              </button>
            )
          }
        />
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('teams.table.empty')}</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('teams.table.team')}</th>
                  <th>{t('teams.table.members')}</th>
                  <th>{t('teams.table.contacts')}</th>
                  <th>{t('teams.table.usage')}</th>
                  <th>{t('teams.table.alerts')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((tm) => (
                  <tr key={tm.id} className={admin && tm.managed ? 'row-click' : ''} onClick={() => admin && tm.managed && setEdit(tm)}>
                    <td className="cell-title">
                      <div className="title-line">{tm.name}</div>
                      <div className="sub-line">
                        <span className="mono">{tm.id}</span>
                        {tm.description && ` · ${tm.description}`}
                        {!tm.managed && <span className="tag">{t('teams.table.unmanaged')}</span>}
                      </div>
                    </td>
                    <td>
                      <div className="team-members">
                        {(tm.members ?? []).map((m) => (
                          <span key={m} className={`tag ${(tm.leads ?? []).includes(m) ? 'tag-ok' : ''}`}>
                            {userName(m)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>
                      {tm.email && (
                        <a className="link" href={`mailto:${tm.email}`} onClick={(e) => e.stopPropagation()}>
                          {tm.email}
                        </a>
                      )}
                      {tm.chat && <div className="sub-line">{tm.chat}</div>}
                    </td>
                    <td className="num nowrap">
                      {tm.cis} · {tm.connectors} · {tm.rules}
                    </td>
                    <td className="num">
                      {tm.open_alerts || ''}
                      {!tm.managed && admin && (
                        <button
                          className="btn btn-sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            setPreset(tm.id)
                            setEdit('new')
                          }}
                        >
                          {t('teams.table.adopt')}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {edit && (
        <TeamModal
          team={edit === 'new' ? null : edit}
          preset={preset}
          users={users}
          onClose={() => setEdit(null)}
          onDone={(msg) => {
            setEdit(null)
            toast(msg)
            reload()
            reloadMeta()
          }}
        />
      )}
    </div>
  )
}

function TeamModal({ team, preset, users, onClose, onDone }: { team: TeamView | null; preset: string; users: User[]; onClose: () => void; onDone: (msg: string) => void }) {
  const [id, setId] = useState(team?.id ?? preset)
  const [name, setName] = useState(team?.name ?? preset)
  const [description, setDescription] = useState(team?.description ?? '')
  const [email, setEmail] = useState(team?.email ?? '')
  const [chat, setChat] = useState(team?.chat ?? '')
  const [members, setMembers] = useState<string[]>(team?.members ?? [])
  const [leads, setLeads] = useState<string[]>(team?.leads ?? [])
  const [error, setError] = useState('')
  const people = users.filter((u) => !u.service)
  const toggle = (list: string[], set: (v: string[]) => void, v: string) => set(list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  const save = async () => {
    setError('')
    const body = { id, name, description, email, chat, members, leads }
    try {
      if (team) {
        await api.put(`/api/teams/${team.id}`, body)
        onDone(t('teams.toasts.saved'))
      } else {
        await api.post('/api/teams', body)
        onDone(t('teams.toasts.created'))
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const remove = async () => {
    if (!team || !window.confirm(t('teams.confirm.delete', { name: team.name }))) return
    try {
      await api.del(`/api/teams/${team.id}`)
    } catch (e) {
      const used = team.cis + team.connectors + team.rules
      if (!(e instanceof ApiError) || e.status !== 409 || !window.confirm(t('teams.confirm.detach', { n: used }))) {
        setError((e as Error).message)
        return
      }
      await api.del(`/api/teams/${team.id}?detach=1`)
    }
    onDone(t('teams.toasts.deleted'))
  }
  return (
    <Modal
      wide
      title={team ? t('teams.form.editTitle', { name: team.name }) : t('teams.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {team && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('teams.form.delete')}
              </button>
            </div>
          )}
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={!id || !name}>
            {team ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      <div className="row2">
        <Field label={t('teams.form.id')} help={t('teams.form.idHelp')}>
          <input className="mono" value={id} onChange={(e) => setId(e.target.value.toLowerCase())} disabled={!!team} autoFocus={!team} />
        </Field>
        <Field label={t('teams.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      </div>
      <Field label={t('teams.form.description')}>
        <input value={description} onChange={(e) => setDescription(e.target.value)} />
      </Field>
      <div className="row2">
        <Field label={t('teams.form.email')}>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label={t('teams.form.chat')}>
          <input value={chat} onChange={(e) => setChat(e.target.value)} placeholder="#ops" />
        </Field>
      </div>
      <div className="row2">
        <Field label={t('teams.form.members')}>
          <div className="chips">
            {people.map((u) => (
              <button type="button" key={u.id} className={`chip ${members.includes(u.id) ? 'chip-on' : ''}`} onClick={() => toggle(members, setMembers, u.id)}>
                {u.name || u.username}
              </button>
            ))}
          </div>
        </Field>
        <Field label={t('teams.form.leads')}>
          <div className="chips">
            {people.map((u) => (
              <button
                type="button"
                key={u.id}
                className={`chip ${leads.includes(u.id) ? 'chip-on' : ''}`}
                onClick={() => {
                  toggle(leads, setLeads, u.id)
                  if (!members.includes(u.id)) setMembers([...members, u.id])
                }}
              >
                {u.name || u.username}
              </button>
            ))}
          </div>
        </Field>
      </div>
      {error && <div className="form-error">{error}</div>}
    </Modal>
  )
}
