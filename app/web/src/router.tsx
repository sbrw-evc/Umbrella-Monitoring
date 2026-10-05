import { createContext, useCallback, useContext, useEffect, useMemo, useState, type AnchorHTMLAttributes, type MouseEvent, type ReactNode } from 'react'

type NavigateOptions = { replace?: boolean }

type Router = { path: string; navigate: (to: string, options?: NavigateOptions) => void }

const RouterContext = createContext<Router>({ path: '/', navigate: () => {} })

export const SCROLL_ROOT_ID = 'app-main'

function scrollToTop() {
  const root = document.getElementById(SCROLL_ROOT_ID)
  if (root) root.scrollTo({ top: 0 })
  else window.scrollTo({ top: 0 })
}

function currentPath() {
  return document.location.pathname || '/'
}

export function RouterProvider({ children }: { children: ReactNode }) {
  const [path, setPath] = useState(currentPath)

  useEffect(() => {
    const onPop = () => setPath(currentPath())
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const navigate = useCallback((to: string, { replace = false }: NavigateOptions = {}) => {
    if (to === currentPath() && !replace) return
    if (replace) window.history.replaceState(null, '', to)
    else window.history.pushState(null, '', to)
    setPath(currentPath())
    scrollToTop()
  }, [])

  const value = useMemo(() => ({ path, navigate }), [path, navigate])
  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
}

export function useRouter() {
  return useContext(RouterContext)
}

function plainClick(e: MouseEvent<HTMLAnchorElement>) {
  return (
    e.button === 0 &&
    !e.metaKey &&
    !e.ctrlKey &&
    !e.shiftKey &&
    !e.altKey &&
    !e.defaultPrevented &&
    (!e.currentTarget.target || e.currentTarget.target === '_self')
  )
}

export function Link({ to, onClick, ...rest }: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & { to: string }) {
  const { navigate } = useRouter()
  return (
    <a
      {...rest}
      href={to}
      onClick={(e) => {
        onClick?.(e)
        if (!plainClick(e)) return
        e.preventDefault()
        navigate(to)
      }}
    />
  )
}
