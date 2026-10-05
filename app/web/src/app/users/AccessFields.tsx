import { useMemo } from 'react'
import { useT } from '../../i18n'
import { Field, Select } from '../../ui'
import { useSession } from '../session'
import { roleLabel } from '../types'
import { adminOf, teamOptions, type Refs } from './model'
import { strings } from './strings'

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
