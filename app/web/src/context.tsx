import { createContext, Fragment, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, setUser, type Meta } from './api'
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
import { applyTheme, BUILT_IN_THEMES, loadActiveTheme, loadCustomThemes, saveActiveTheme, saveCustomThemes, type Theme } from './theme'

// Live updates from /api/ws. Pages subscribe to message types and refetch.
type Listener = (type: string, data: unknown) => void

interface AppState {
  meta: Meta | null
  team: string // "all" or team id: scopes what the user sees
  setTeam: (t: string) => void
  user: string
  setUserName: (u: string) => void
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
  const [user, setUserState] = useState(() => load('umb.user', 'Дежурный инженер'))
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

  setUser(user)

  useEffect(() => {
    api.get<Meta>('/api/meta').then(setMeta).catch(() => setMeta(null))
  }, [])

  useEffect(() => {
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
    }
  }, [])

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
  const setUserName = (u: string) => {
    setUserState(u)
    setUser(u)
    save('umb.user', u)
  }

  const setTheme = (id: string) => {
    setThemeId(id)
    saveActiveTheme(id)
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
    setLocaleId(id)
    saveActiveLocale(id)
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
    user,
    setUserName,
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
      <Fragment key={`${locale.id}:${localeRev}`}>{children}</Fragment>
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
