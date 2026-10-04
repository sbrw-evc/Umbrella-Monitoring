import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import type { Theme } from './api'

const KEY = 'umbrella.theme'

type Ctx = { theme: Theme; setTheme: (t: Theme, persist?: boolean) => void }

const ThemeContext = createContext<Ctx>({ theme: 'light', setTheme: () => {} })

export function savedTheme(): Theme | null {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'dark' ? v : null
  } catch {
    return null
  }
}

export function applyTheme(t: Theme) {
  document.documentElement.dataset.theme = t
  document.documentElement.style.colorScheme = t
}

export function ThemeProvider({ initial, children }: { initial: Theme; children: ReactNode }) {
  const [theme, set] = useState<Theme>(initial)
  applyTheme(theme)
  const setTheme = useCallback((t: Theme, persist = true) => {
    set(t)
    applyTheme(t)
    if (!persist) return
    try {
      localStorage.setItem(KEY, t)
    } catch {
      return
    }
  }, [])
  return <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  return useContext(ThemeContext)
}
