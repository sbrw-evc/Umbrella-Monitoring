import { Plus, Search, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, ciTypeLabel, fmtTime, type CI, type Identity, type Owner } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { CiDrawer } from '../components/CiDrawer'
import { CiIcon } from '../components/CmdbGraph'
import { IncidentDrawer } from '../components/IncidentDrawer'
import { Empty, Field, Modal, PageHeader, SevBadge } from '../components/ui'

export const CI_TYPES = ['business_service', 'it_service', 'host', 'database', 'cloud_group', 'deployment', 'network']
const SOURCE_KINDS = ['netbox', 'zabbix', 'prometheus']

export function originLabel(o: string) {
  const v = t(`cis.origin.${o}`)
  return v === `cis.origin.${o}` ? o : v
}

export function ciSources(c: CI): string[] {
  const out = new Set<string>()
  for (const i of c.identities) if (!i.until && SOURCE_KINDS.includes(i.kind)) out.add(i.kind)
  if (out.size === 0) out.add(c.origin)
  return [...out]
}

export function OwnerList({ owners }: { owners?: Owner[] }) {
  if (!owners?.length) return <span className="muted">—</span>
  return (
    <div className="owner-list">
      {owners.map((o, i) => (
        <div key={i} className="owner">
          {o.email ? (
            <a className="link" href={`mailto:${o.email}`}>
              {o.name || o.email}
            </a>
          ) : (
            o.name
          )}
          {o.role && <span className="muted">{o.role}</span>}
        </div>
      ))}
    </div>
  )
}

export function CisPage() {
  const { can, meta } = useApp()
  const { data, reload } = useFetch<{ items: CI[] }>('/api/cis')
  const [q, setQ] = useState('')
  const [type, setType] = useState('')
  const [team, setTeam] = useState('')
  const [origin, setOrigin] = useState('')
  const [noOwners, setNoOwners] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  const [openInc, setOpenInc] = useState<string | null>(null)
  const [edit, setEdit] = useState<CI | 'new' | null>(null)
  useLive(['alert'], reload, 2000)
  const all = data?.items ?? []
  const origins = useMemo(() => [...new Set(all.flatMap(ciSources))].sort(), [all])
  const teams = useMemo(() => [...new Set(all.map((c) => c.team).filter(Boolean))].sort(), [all])
  const needle = q.trim().toLowerCase()
  const items = all
    .filter((c) => !type || c.type === type)
    .filter((c) => !team || c.team === team)
    .filter((c) => !origin || ciSources(c).includes(origin))
    .filter((c) => !noOwners || !c.owners?.length)
    .filter((c) => {
      if (!needle) return true
      const hay = [c.id, c.name, c.description, c.logical_group, ...c.identities.map((i) => i.value), ...(c.owners ?? []).flatMap((o) => [o.name, o.email])]
        .join(' ')
        .toLowerCase()
      return hay.includes(needle)
    })
    .sort((a, b) => a.name.localeCompare(b.name))
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('cis.header.title')}
          sub={t('cis.header.sub')}
          actions={
            can('cmdb.edit') && (
              <button className="btn btn-primary" onClick={() => setEdit('new')}>
                <Plus size={15} /> {t('cis.header.create')}
              </button>
            )
          }
        />
        <div className="filterbar">
          <div className="search">
            <Search size={15} />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('cis.filters.search')} />
          </div>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">{t('cis.filters.allTypes')}</option>
            {CI_TYPES.map((x) => (
              <option key={x} value={x}>
                {ciTypeLabel(x)}
              </option>
            ))}
          </select>
          <select value={team} onChange={(e) => setTeam(e.target.value)}>
            <option value="">{t('cis.filters.allTeams')}</option>
            {teams.map((x) => (
              <option key={x}>{x}</option>
            ))}
          </select>
          <select value={origin} onChange={(e) => setOrigin(e.target.value)}>
            <option value="">{t('cis.filters.allOrigins')}</option>
            {origins.map((x) => (
              <option key={x} value={x}>
                {originLabel(x)}
              </option>
            ))}
          </select>
          <label className="check">
            <input type="checkbox" checked={noOwners} onChange={(e) => setNoOwners(e.target.checked)} /> {t('cis.filters.noOwners')}
          </label>
          <div className="filterbar-spacer" />
          <span className="muted">{t('cis.filters.count', { n: items.length })}</span>
        </div>
        <div className="card card-flush">
          {items.length === 0 ? (
            <Empty>{t('cis.table.empty')}</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('cis.table.id')}</th>
                  <th>{t('cis.table.name')}</th>
                  <th>{t('cis.table.type')}</th>
                  <th>{t('cis.table.team')}</th>
                  <th>{t('cis.table.owners')}</th>
                  <th>{t('cis.table.sources')}</th>
                  <th>{t('cis.table.ids')}</th>
                  <th>{t('cis.table.state')}</th>
                  <th>{t('cis.table.updated')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((c) => (
                  <tr key={c.id} className="row-click" onClick={() => setOpen(c.id)}>
                    <td className="mono nowrap">{c.id}</td>
                    <td className="cell-title">
                      <div className="title-line">{c.name}</div>
                      {c.description && <div className="sub-line">{c.description}</div>}
                    </td>
                    <td className="nowrap">
                      <span className="type-cell">
                        <CiIcon type={c.type} size={14} /> {ciTypeLabel(c.type)}
                      </span>
                    </td>
                    <td>{c.team || <span className="muted">—</span>}</td>
                    <td>
                      <OwnerList owners={c.owners} />
                    </td>
                    <td>
                      <div className="tags">
                        {ciSources(c).map((s) => (
                          <span key={s} className="tag">
                            {originLabel(s)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="num">{c.identities.filter((i) => !i.until).length}</td>
                    <td>
                      <SevBadge sev={c.status} />
                    </td>
                    <td className="nowrap">{fmtTime(c.updated_at ?? c.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {open && (
        <CiDrawer
          id={open}
          onClose={() => setOpen(null)}
          onOpenCi={setOpen}
          onOpenIncident={(id) => {
            setOpen(null)
            setOpenInc(id)
          }}
          onEdit={(ci) => setEdit(ci)}
          onChanged={reload}
        />
      )}
      {openInc && <IncidentDrawer id={openInc} onClose={() => setOpenInc(null)} onOpen={setOpenInc} />}
      {edit && (
        <CiForm
          ci={edit === 'new' ? null : edit}
          all={all}
          teams={[...new Set([...teams, ...(meta?.teams.map((x) => x.id) ?? [])])]}
          onClose={() => setEdit(null)}
          onDone={(deleted) => {
            setEdit(null)
            if (deleted) setOpen(null)
            reload()
          }}
        />
      )}
    </div>
  )
}

function parseLabels(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const i = line.indexOf('=')
    if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return out
}

export function CiForm({ ci, all, teams, onClose, onDone }: { ci: CI | null; all: CI[]; teams: string[]; onClose: () => void; onDone: (deleted?: boolean) => void }) {
  const { toast } = useApp()
  const [name, setName] = useState(ci?.name ?? '')
  const [type, setType] = useState(ci?.type ?? 'host')
  const [team, setTeam] = useState(ci?.team ?? '')
  const [description, setDescription] = useState(ci?.description ?? '')
  const [group, setGroup] = useState(ci?.logical_group ?? '')
  const [parent, setParent] = useState('')
  const [owners, setOwners] = useState<Owner[]>(ci?.owners ?? [])
  const [ids, setIds] = useState<Identity[]>((ci?.identities ?? []).filter((i) => !i.until))
  const [labels, setLabels] = useState(Object.entries(ci?.labels ?? {}).map(([k, v]) => `${k}=${v}`).join('\n'))
  const [error, setError] = useState('')
  const synced = ci ? ciSources(ci).filter((s) => SOURCE_KINDS.includes(s)) : []
  const save = async () => {
    setError('')
    const body = {
      name,
      type,
      team,
      description,
      logical_group: group,
      owners: owners.filter((o) => o.name || o.email),
      identities: ids.filter((i) => i.kind && i.value),
      labels: parseLabels(labels),
    }
    try {
      if (ci) {
        await api.put(`/api/cis/${ci.id}`, body)
        toast(t('cis.toasts.saved'))
      } else {
        await api.post('/api/cis', { ...body, parent })
        toast(t('cis.toasts.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const remove = async () => {
    if (!ci || !window.confirm(t('cis.confirm.delete', { name: ci.name }))) return
    await api.del(`/api/cis/${ci.id}`)
    toast(t('cis.toasts.deleted'))
    onDone(true)
  }
  const setOwner = (i: number, k: keyof Owner, v: string) => setOwners((list) => list.map((o, n) => (n === i ? { ...o, [k]: v } : o)))
  const setId = (i: number, k: 'kind' | 'value', v: string) => setIds((list) => list.map((o, n) => (n === i ? { ...o, [k]: v } : o)))
  return (
    <Modal
      wide
      title={ci ? t('cis.form.editTitle', { name: ci.name }) : t('cis.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {ci && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('cis.form.delete')}
              </button>
            </div>
          )}
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button className="btn btn-primary" onClick={save} disabled={!name}>
            {ci ? t('common.actions.save') : t('common.actions.create')}
          </button>
        </>
      }
    >
      {synced.length > 0 && <p className="hint hint-warn">{t('cis.form.synced', { source: synced.map(originLabel).join(', ') })}</p>}
      <div className="row3">
        <Field label={t('cis.form.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus={!ci} />
        </Field>
        <Field label={t('cis.form.type')}>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            {CI_TYPES.map((k) => (
              <option key={k} value={k}>
                {ciTypeLabel(k)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('cis.form.team')}>
          <input value={team} onChange={(e) => setTeam(e.target.value)} list="ci-teams" />
          <datalist id="ci-teams">
            {teams.map((x) => (
              <option key={x} value={x} />
            ))}
          </datalist>
        </Field>
      </div>
      <div className="row2">
        <Field label={t('cis.form.description')}>
          <input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label={t('cis.form.group')} help={t('cis.form.groupHelp')}>
          <input value={group} onChange={(e) => setGroup(e.target.value)} />
        </Field>
      </div>
      {!ci && (
        <Field label={t('cis.form.parent')} help={t('cis.form.parentHelp')}>
          <select value={parent} onChange={(e) => setParent(e.target.value)}>
            <option value="">—</option>
            {all.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name} ({ciTypeLabel(c.type)})
              </option>
            ))}
          </select>
        </Field>
      )}
      <div className="section">
        <h4>{t('cis.form.owners')}</h4>
        <div className="list-edit">
          {owners.map((o, i) => (
            <div key={i} className="list-edit-row">
              <input value={o.name} placeholder={t('cis.form.ownerName')} onChange={(e) => setOwner(i, 'name', e.target.value)} />
              <input value={o.email ?? ''} placeholder={t('cis.form.ownerEmail')} onChange={(e) => setOwner(i, 'email', e.target.value)} />
              <input value={o.role ?? ''} placeholder={t('cis.form.ownerRole')} onChange={(e) => setOwner(i, 'role', e.target.value)} />
              <button type="button" className="icon-btn icon-btn-sm" onClick={() => setOwners((l) => l.filter((_, n) => n !== i))}>
                <X size={14} />
              </button>
            </div>
          ))}
          <div>
            <button type="button" className="btn btn-sm" onClick={() => setOwners((l) => [...l, { name: '' }])}>
              <Plus size={13} /> {t('cis.form.addOwner')}
            </button>
          </div>
        </div>
      </div>
      <div className="section">
        <h4>{t('cis.form.ids')}</h4>
        <p className="hint">{t('cis.form.idsHelp')}</p>
        <div className="list-edit">
          {ids.map((o, i) => (
            <div key={i} className="list-edit-row two">
              <input value={o.kind} placeholder={t('cis.form.idKind')} onChange={(e) => setId(i, 'kind', e.target.value)} />
              <input className="mono" value={o.value} placeholder={t('cis.form.idValue')} onChange={(e) => setId(i, 'value', e.target.value)} />
              <button type="button" className="icon-btn icon-btn-sm" onClick={() => setIds((l) => l.filter((_, n) => n !== i))}>
                <X size={14} />
              </button>
            </div>
          ))}
          <div>
            <button type="button" className="btn btn-sm" onClick={() => setIds((l) => [...l, { kind: 'hostname', value: '', since: '' }])}>
              <Plus size={13} /> {t('cis.form.addId')}
            </button>
          </div>
        </div>
      </div>
      <Field label={t('cis.form.labels')} help={t('cis.form.labelsHelp')}>
        <textarea className="mono" rows={3} value={labels} onChange={(e) => setLabels(e.target.value)} />
      </Field>
      {error && <div className="form-error">{error}</div>}
    </Modal>
  )
}
