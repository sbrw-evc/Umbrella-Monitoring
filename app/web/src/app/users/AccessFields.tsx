import { useMemo } from 'react'
import { useT } from '../../i18n'
import { Field, Select } from '../../ui'
import { MultiPicker } from '../services/Pickers'
import { useSession } from '../session'
import { roleLabel } from '../types'
import { adminOf, teamOptions, type Refs, type ScopeService } from './model'
import { strings } from './strings'
import '../services/services.css'

export type Access = { role_id: string; team_id: string }

export function RoleSelect({
  id,
  refs,
  value,
  onChange,
  allLabel,
}: {
  id: string
  refs: Refs
  value: string
  onChange: (v: string) => void
  allLabel?: string
}) {
  const t = useT(strings)
  const { user } = useSession()
  return (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
      {allLabel !== undefined && <option value="">{allLabel}</option>}
      {refs.roles.map((r) => (
        <option key={r.id} value={r.id} disabled={allLabel === undefined && r.id === 'admin' && !adminOf(user) && value !== r.id}>
          {roleLabel(t, r.id, r.name)}
        </option>
      ))}
    </Select>
  )
}

export function TeamSelect({
  id,
  refs,
  value,
  onChange,
  emptyLabel,
}: {
  id: string
  refs: Refs
  value: string
  onChange: (v: string) => void
  emptyLabel: string
}) {
  const options = useMemo(() => teamOptions(refs.teams), [refs.teams])
  return (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{emptyLabel}</option>
      {options.map((o) => (
        <option key={o.id} value={o.id} title={o.path}>
          {'\u2003'.repeat(o.depth) + o.name}
        </option>
      ))}
    </Select>
  )
}

export function AccessFields({
  refs,
  value,
  onChange,
  roleLocked,
  roleHint,
}: {
  refs: Refs
  value: Access
  onChange: (v: Access) => void
  roleLocked?: boolean
  roleHint?: string
}) {
  const t = useT(strings)
  return (
    <div className="grid-2">
      <Field label={t('usr.field.role')} hint={roleHint}>
        {(id) => (
          <fieldset className="plain-fieldset" disabled={roleLocked}>
            <RoleSelect id={id} refs={refs} value={value.role_id} onChange={(role_id) => onChange({ ...value, role_id })} />
          </fieldset>
        )}
      </Field>
      <Field label={t('usr.field.team')}>
        {(id) => <TeamSelect id={id} refs={refs} value={value.team_id} onChange={(team_id) => onChange({ ...value, team_id })} emptyLabel={t('usr.noTeam')} />}
      </Field>
    </div>
  )
}

/** ScopeField picks the business services whose incidents the user sees; none means all. */
export function ScopeField({
  refs,
  value,
  known,
  admin,
  onChange,
}: {
  refs: Refs
  value: string[]
  known?: ScopeService[]
  admin: boolean
  onChange: (ids: string[]) => void
}) {
  const t = useT(strings)
  const names = useMemo(() => new Map(refs.services.map((s) => [s.id, s.name])), [refs.services])
  const options = useMemo(() => refs.services.map((s) => ({ id: s.id, label: s.name })), [refs.services])
  const labelOf = (id: string) => {
    const name = names.get(id) ?? known?.find((s) => s.id === id)?.name ?? id
    const missing = !names.has(id)
    return { key: id, label: missing ? t('usr.scope.missing', { name }) : name, title: name, muted: missing }
  }
  return (
    <Field label={t('usr.field.scope')} hint={admin ? t('usr.scope.admin') : t('usr.scope.hint')}>
      {(id) => <MultiPicker id={id} selected={value} options={options} labelOf={labelOf} placeholder={t('usr.scope.add')} onChange={onChange} />}
    </Field>
  )
}
