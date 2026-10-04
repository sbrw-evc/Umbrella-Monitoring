export type Theme = 'light' | 'dark'
export type Locale = 'en' | 'ru'

export type Meta = {
  mode: 'setup' | 'ready'
  version: string
  default_theme: Theme
  default_locale: Locale
  ldap_enabled?: boolean
}

export class ApiError extends Error {
  status: number
  code: string
  detail?: string

  constructor(status: number, code: string, detail?: string) {
    super(detail ? `${code}: ${detail}` : code)
    this.status = status
    this.code = code
    this.detail = detail
  }
}

let csrf = ''

export function setCsrf(v: string) {
  csrf = v
}

export async function api<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        ...(csrf && method !== 'GET' ? { 'X-CSRF-Token': csrf } : {}),
        ...headers,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    throw new ApiError(0, 'network', e instanceof Error ? e.message : String(e))
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data.error ?? `http_${res.status}`, data.detail)
  return data as T
}
