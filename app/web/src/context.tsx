import { flushSync } from 'react-dom'
import { createContext, Fragment, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, AUTH_EVENT, setCSRF, type Me, type Meta, type Perm } from './api'
import {
  BUILT_IN_LOCALES,
  loadActiveLocale,
  loadCustomLocales,
  saveActiveLocale,
  saveCustomLocales,
  setActiveLocale,
  t as translate,
  type Locale,
  type T,
} from './i18n'
import { transition } from './motion'
import { SignIn } from './pages/SignIn'
import { applyTheme, BUILT_IN_THEMES, loadActiveTheme, loadCustomThemes, saveActiveTheme, saveCustomThemes, type Theme } from './theme'

// Live updates from /api/ws. Pages subscribe to message types and refetch.
type Listener = (type: string, data: unknown) => void

interface AppState {
  meta: Meta | null
  team: string // "all" or team id: scopes what the user sees
  setTeam: (t: string) => void
  // signed-in user; null while signed out
  me: Me | null
  can: (p: Perm) => boolean
  signedIn: (m: Me) => void
  refreshMe: () => Promise<void>
  logout: () => void
  connected: boolean
  subscribe: (l: Listener) => () => void
  toast: (text: string, kind?: 'ok' | 'error') => void
  // appearance and language
  themes: Theme[]
  theme: Theme
  setTheme: (id: string) => void
  addTheme: (t: Theme) => void
  removeTheme: (id: string) => void
  locales: Locale[]
  locale: Locale
  setLocale: (id: string) => void
  addLocale: (l: Locale) => void
  removeLocale: (id: string) => void
  t: T
}

const Ctx = createContext<AppState | null>(null)

function load(key: string, def: string): string {
  try {
    return localStorage.getItem(key) ?? def
  } catch {
    return def
  }
}

function save(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* storage unavailable */
  }
}

export function AppProvider({ children }: { children: ReactNode }) {
  const [meta, setMeta] = useState<Meta | null>(null)
  const [team, setTeamState] = useState(() => load('umb.team', 'all'))
  // undefined: still checking the session; null: signed out
  const [me, setMe] = useState<Me | null | undefined>(undefined)
  const [connected, setConnected] = useState(false)
  const [toasts, setToasts] = useState<{ id: number; text: string; kind: string }[]>([])
  const listeners = useRef(new Set<Listener>())

  const [customThemes, setCustomThemes] = useState(loadCustomThemes)
  const [themeId, setThemeId] = useState(loadActiveTheme)
  const themes = useMemo(() => [...BUILT_IN_THEMES, ...customThemes], [customThemes])
  const theme = themes.find((x) => x.id === themeId) ?? BUILT_IN_THEMES[0]
  useEffect(() => applyTheme(theme), [theme])

  const [customLocales, setCustomLocales] = useState(loadCustomLocales)
  const [localeId, setLocaleId] = useState(loadActiveLocale)
  const [localeRev, setLocaleRev] = useState(0)
  const locales = useMemo(() => {
    // An uploaded file with a built-in id (ru, en) overrides that language.
    const ids = new Set(customLocales.map((l) => l.id))
    return [...BUILT_IN_LOCALES.filter((l) => !ids.has(l.id)), ...customLocales]
  }, [customLocales])
  const locale = locales.find((x) => x.id === localeId) ?? BUILT_IN_LOCALES[0]
  setActiveLocale(locale)

  const applyMe = useCallback((m: Me | null) => {
    setCSRF(m?.csrf ?? '')
    setMe(m)
  }, [])
  const refreshMe = useCallback(
    () =>
      api
        .get<Me>('/api/auth/me')
        .then(applyMe)
        .catch(() => applyMe(null)),
    [applyMe],
  )
  useEffect(() => {
    refreshMe()
    // A 401 or "change your password" from any call re-checks the session.
    const onAuth = () => refreshMe()
    window.addEventListener(AUTH_EVENT, onAuth)
    return () => window.removeEventListener(AUTH_EVENT, onAuth)
  }, [refreshMe])
  const logout = useCallback(() => {
    api
      .post('/api/auth/logout')
      .catch(() => undefined)
      .finally(() => applyMe(null))
  }, [applyMe])
  const perms = useMemo(() => new Set(me?.permissions ?? []), [me])
  const can = useCallback((p: Perm) => perms.has(p), [perms])
  const ready = !!me && !me.user.must_change_password

  useEffect(() => {
    if (!ready) return
    api.get<Meta>('/api/meta').then(setMeta).catch(() => setMeta(null))
  }, [ready])

  useEffect(() => {
    if (!ready) return
    let ws: WebSocket | null = null
    let stopped = false
    let retry: number | undefined
    const open = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      ws = new WebSocket(`${proto}://${location.host}/api/ws`)
      ws.onopen = () => setConnected(true)
      ws.onclose = () => {
        setConnected(false)
        if (!stopped) retry = window.setTimeout(open, 3000)
      }
      ws.onmessage = (m) => {
        try {
          const msg = JSON.parse(m.data)
          listeners.current.forEach((l) => l(msg.type, msg.data))
        } catch {
          /* ignore malformed */
        }
      }
    }
    open()
    return () => {
      stopped = true
      window.clearTimeout(retry)
      ws?.close()
      setConnected(false)
    }
  }, [ready])

  const subscribe = useCallback((l: Listener) => {
    listeners.current.add(l)
    return () => {
      listeners.current.delete(l)
    }
  }, [])

  const toast = useCallback((text: string, kind: 'ok' | 'error' = 'ok') => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, text, kind }])
    window.setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000)
  }, [])

  const setTeam = (t: string) => {
    setTeamState(t)
    save('umb.team', t)
  }

  const setTheme = (id: string) => {
    if (id === themeId) return
    const next = themes.find((x) => x.id === id)
    saveActiveTheme(id)
    transition('theme', () => {
      if (next) applyTheme(next)
      flushSync(() => setThemeId(id))
    })
  }
  const addTheme = (th: Theme) => {
    const next = [...customThemes.filter((x) => x.id !== th.id), th]
    setCustomThemes(next)
    saveCustomThemes(next)
  }
  const removeTheme = (id: string) => {
    const next = customThemes.filter((x) => x.id !== id)
    setCustomThemes(next)
    saveCustomThemes(next)
    if (themeId === id) setTheme('light')
  }
  const setLocale = (id: string) => {
    if (id === localeId) return
    saveActiveLocale(id)
    transition('locale', () => flushSync(() => setLocaleId(id)))
  }
  const addLocale = (l: Locale) => {
    const next = [...customLocales.filter((x) => x.id !== l.id), l]
    setCustomLocales(next)
    saveCustomLocales(next)
    setLocaleRev((r) => r + 1)
  }
  const removeLocale = (id: string) => {
    const next = customLocales.filter((x) => x.id !== id)
    setCustomLocales(next)
    saveCustomLocales(next)
    setLocaleRev((r) => r + 1)
    if (localeId === id && !BUILT_IN_LOCALES.some((l) => l.id === id)) setLocale('ru')
  }

  const value: AppState = {
    meta,
    team,
    setTeam,
    me: me ?? null,
    can,
    signedIn: applyMe,
    refreshMe,
    logout,
    connected,
    subscribe,
    toast,
    themes,
    theme,
    setTheme,
    addTheme,
    removeTheme,
    locales,
    locale,
    setLocale,
    addLocale,
    removeLocale,
    t: translate,
  }

  // The key remounts the tree on a language change so every label, date and
  // memoized list is rebuilt in the new language.
  return (
    <Ctx.Provider value={value}>
      <Fragment key={`${locale.id}:${localeRev}`}>
        {me === undefined ? <div className="boot" /> : ready ? children : <SignIn mustChange={!!me} />}
      </Fragment>
      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`}>
            {t.text}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}

export function useApp(): AppState {
  const c = useContext(Ctx)
  if (!c) throw new Error('AppProvider missing')
  return c
}

// useLive calls refresh (debounced) whenever a message of one of the types
// arrives.
export function useLive(types: string[], refresh: () => void, delay = 600) {
  const { subscribe } = useApp()
  const ref = useRef(refresh)
  ref.current = refresh
  const key = types.join(',')
  useEffect(() => {
    let t: number | undefined
    const set = new Set(key.split(','))
    const off = subscribe((type) => {
      if (!set.has(type)) return
      window.clearTimeout(t)
      t = window.setTimeout(() => ref.current(), delay)
    })
    return () => {
      off()
      window.clearTimeout(t)
    }
  }, [key, subscribe, delay])
}

// useFetch loads url and exposes reload.
export function useFetch<T>(url: string | null) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const reload = useCallback(() => {
    if (!url) return
    setLoading(true)
    api
      .get<T>(url)
      .then((d) => {
        setData(d)
        setError(null)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [url])
  useEffect(() => {
    reload()
  }, [reload])
  return { data, error, loading, reload, setData }
}
