export type ProfileFields = {
  last_name: string
  first_name: string
  middle_name: string
  title: string
  department: string
  manager: string
  email: string
}

export type User = ProfileFields & {
  id: string
  username: string
  name: string
  source: 'local' | 'ldap'
  role: 'admin' | 'user'
  timezone: string
  last_login_at?: string
  csrf?: string
  gravatar?: string
  has_avatar?: boolean
  avatar_version?: string
  avatar_source?: 'upload' | 'ldap'
}

export const PROFILE_KEYS: (keyof ProfileFields)[] = ['last_name', 'first_name', 'middle_name', 'title', 'department', 'manager', 'email']

export function profileOf(u: ProfileFields): ProfileFields {
  return Object.fromEntries(PROFILE_KEYS.map((k) => [k, u[k] ?? ''])) as ProfileFields
}

export function fullName(u: User) {
  return [u.last_name, u.first_name, u.middle_name].filter(Boolean).join(' ') || u.name
}
