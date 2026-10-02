import {
  Activity,
  AlertTriangle,
  BookOpen,
  Cable,
  Check,
  ChevronDown,
  ChevronsLeft,
  ChevronsRight,
  ClipboardList,
  Gauge,
  Globe,
  Grid3x3,
  Moon,
  Network,
  RadioTower,
  Search,
  Settings,
  ShieldCheck,
  Siren,
  Sun,
  Users,
  Wrench,
} from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../context'
import { t } from '../i18n'

interface Item {
  to: string
  key: string
  icon: ReactNode
}
interface Group {
  id: string
  items: Item[]
}

// Menu groups follow the operator's path: watch, collect, process, admin.
// Titles are i18n keys (menu.groups.<id>, menu.items.<key>).
const GROUPS: Group[] = [
  {
    id: 'overview',
    items: [
      { to: '/ops', key: 'ops', icon: <Gauge size={18} /> },
      { to: '/incidents', key: 'incidents', icon: <Siren size={18} /> },
      { to: '/heatmap', key: 'heatmap', icon: <Grid3x3 size={18} /> },
      { to: '/cmdb', key: 'cmdb', icon: <Network size={18} /> },
    ],
  },
  {
    id: 'collect',
    items: [
      { to: '/connectors', key: 'connectors', icon: <Cable size={18} /> },
      { to: '/events', key: 'events', icon: <RadioTower size={18} /> },
      { to: '/parse-errors', key: 'parseErrors', icon: <AlertTriangle size={18} /> },
    ],
  },
  {
    id: 'process',
    items: [
      { to: '/rules', key: 'rules', icon: <BookOpen size={18} /> },
      { to: '/maintenance', key: 'maintenance', icon: <Wrench size={18} /> },
    ],
  },
  {
    id: 'admin',
    items: [
      { to: '/selfcheck', key: 'selfcheck', icon: <Activity size={18} /> },
      { to: '/audit', key: 'audit', icon: <ClipboardList size={18} /> },
      { to: '/roles', key: 'roles', icon: <ShieldCheck size={18} /> },
    ],
  },
]

function loadBool(key: string, def: boolean) {
  try {
    const v = localStorage.getItem(key)
    return v === null ? def : v === '1'
  } catch {
    return def
  }
}

export function Logo({ size = 30 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden className="logo">
      <rect width="32" height="32" rx="9" fill="var(--rail-logo-background)" />
      <path d="M6 16.5a10 10 0 0 1 20 0z" fill="var(--rail-logo-mark)" />
      <path d="M16 16.5v7a2.4 2.4 0 0 1-4.8 0" fill="none" stroke="var(--rail-logo-mark)" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  )
}

// usePopup closes a popup on an outside click or Escape.
function usePopup() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])
  return { open, setOpen, ref }
}

export function Layout({ children }: { children: ReactNode }) {
  const { meta, team, setTeam, user, setUserName, connected, theme, themes, setTheme, locale, locales, setLocale } = useApp()
  const [expanded, setExpanded] = useState(() => loadBool('umb.rail.expanded', false))
  const [search, setSearch] = useState('')
  const teamPop = usePopup()
  const langPop = usePopup()
  const userPop = usePopup()
  const nav = useNavigate()
  const loc = useLocation()
  const editor = /^\/connectors\/[^/]+$/.test(loc.pathname)

  const toggle = () => {
    setExpanded((x) => {
      try {
        localStorage.setItem('umb.rail.expanded', x ? '0' : '1')
      } catch {
        /* ignore */
      }
      return !x
    })
  }
  const teamName = team === 'all' ? t('common.words.allTeams') : (meta?.teams.find((x) => x.id === team)?.name ?? team)
  // The quick switch flips between the built-in light and dark themes; custom
  // themes are picked in settings.
  const nextTheme = theme.base === 'dark' ? 'light' : 'dark'
  const themeName = (id: string, name: string) => (id === 'light' || id === 'dark' ? t(`settings.themes.builtIn.${id}`) : name)

  return (
    <div className={`app ${expanded ? 'app-expanded' : ''}`}>
      <nav className="rail">
        <button className="rail-logo" onClick={() => nav('/ops')} title={t('common.app.brand')}>
          <Logo />
          {expanded && (
            <span className="rail-brand">
              <b>{t('common.app.brand')}</b>
              <small>{t('common.app.tagline')}</small>
            </span>
          )}
        </button>

        <div className="rail-menu">
          {GROUPS.map((g) => (
            <div key={g.id} className="rail-group">
              {expanded ? <div className="rail-group-title">{t(`menu.groups.${g.id}`)}</div> : <div className="rail-sep" />}
              {g.items.map((it) => (
                <NavLink key={it.to} to={it.to} className={({ isActive }) => `rail-item ${isActive ? 'rail-item-active' : ''}`} data-tip={expanded ? undefined : t(`menu.items.${it.key}`)}>
                  {it.icon}
                  {expanded && <span>{t(`menu.items.${it.key}`)}</span>}
                </NavLink>
              ))}
            </div>
          ))}
        </div>

        <div className="rail-bottom">
          <NavLink to="/settings" className={({ isActive }) => `rail-item ${isActive ? 'rail-item-active' : ''}`} data-tip={expanded ? undefined : t('menu.items.settings')}>
            <Settings size={18} />
            {expanded && <span>{t('menu.items.settings')}</span>}
          </NavLink>
          <button className="rail-item" onClick={toggle} data-tip={expanded ? undefined : t('common.layout.expandMenu')}>
            {expanded ? <ChevronsLeft size={18} /> : <ChevronsRight size={18} />}
            {expanded && <span>{t('common.layout.collapse')}</span>}
          </button>
        </div>
      </nav>

      <div className="shell">
        <header className="topbar">
          <form
            className="top-search"
            onSubmit={(e) => {
              e.preventDefault()
              nav(`/incidents?q=${encodeURIComponent(search)}&view=all`)
            }}
          >
            <Search size={16} />
            <input placeholder={t('common.layout.searchIncidents')} value={search} onChange={(e) => setSearch(e.target.value)} />
          </form>
          <div className="top-spacer" />

          <div className="pop-wrap" ref={teamPop.ref}>
            <button className="top-btn" onClick={() => teamPop.setOpen((o) => !o)} title={t('common.layout.scope')}>
              <Users size={16} />
              <span className="top-btn-text">{teamName}</span>
              <ChevronDown size={14} />
            </button>
            {teamPop.open && (
              <div className="popup popup-right">
                <div className="popup-title">{t('common.layout.scope')}</div>
                {[{ id: 'all', name: t('common.words.allTeams') }, ...(meta?.teams ?? [])].map((x) => (
                  <button
                    key={x.id}
                    className={`popup-item ${x.id === team ? 'active' : ''}`}
                    onClick={() => {
                      setTeam(x.id)
                      teamPop.setOpen(false)
                    }}
                  >
                    <span>{x.name}</span>
                    {x.id === team && <Check size={14} />}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="pop-wrap" ref={langPop.ref}>
            <button className="top-btn" onClick={() => langPop.setOpen((o) => !o)} title={t('common.layout.language')}>
              <Globe size={16} />
              <span className="top-btn-text">{locale.id.toUpperCase()}</span>
            </button>
            {langPop.open && (
              <div className="popup popup-right">
                <div className="popup-title">{t('common.layout.language')}</div>
                {locales.map((l) => (
                  <button
                    key={l.id}
                    className={`popup-item ${l.id === locale.id ? 'active' : ''}`}
                    onClick={() => {
                      langPop.setOpen(false)
                      setLocale(l.id)
                    }}
                  >
                    <span>{l.name}</span>
                    {l.id === locale.id && <Check size={14} />}
                  </button>
                ))}
              </div>
            )}
          </div>

          <button
            className="top-icon"
            onClick={() => setTheme(themes.some((x) => x.id === nextTheme) ? nextTheme : 'light')}
            title={`${themeName(theme.id, theme.name)} → ${t(nextTheme === 'dark' ? 'common.layout.themeDark' : 'common.layout.themeLight')}`}
          >
            {theme.base === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
          </button>

          <span className="top-conn" title={connected ? t('common.layout.onlineHint') : t('common.layout.offlineHint')}>
            <span className={`dot ${connected ? 'dot-ok' : 'dot-critical'}`} />
            <span className="top-conn-text">{connected ? t('common.layout.online') : t('common.layout.offline')}</span>
          </span>

          <div className="pop-wrap" ref={userPop.ref}>
            <button className="top-user" onClick={() => userPop.setOpen((o) => !o)} title={user}>
              <span className="avatar">{user.slice(0, 1)}</span>
            </button>
            {userPop.open && (
              <div className="popup popup-right">
                <div className="popup-user">
                  <b>{user}</b>
                  <small>{t('common.layout.demoSignIn')}</small>
                </div>
                <div className="popup-title">{t('common.layout.signInAs')}</div>
                {(meta?.users ?? []).map((u) => (
                  <button
                    key={u}
                    className={`popup-item ${u === user ? 'active' : ''}`}
                    onClick={() => {
                      setUserName(u)
                      userPop.setOpen(false)
                    }}
                  >
                    <span>{u}</span>
                    {u === user && <Check size={14} />}
                  </button>
                ))}
              </div>
            )}
          </div>
        </header>
        <main className={`main ${editor ? 'main-fixed' : ''}`}>{children}</main>
      </div>
    </div>
  )
}
