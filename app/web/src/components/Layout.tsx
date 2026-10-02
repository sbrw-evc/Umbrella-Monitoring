import {
  Activity,
  AlertTriangle,
  BookOpen,
  Cable,
  ChevronDown,
  ChevronsLeft,
  ChevronsRight,
  ClipboardList,
  Gauge,
  LayoutDashboard,
  ListTree,
  Network,
  RadioTower,
  Search,
  Settings,
  ShieldCheck,
  Siren,
  Users,
  Wrench,
} from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useApp } from '../context'

interface Item {
  to: string
  title: string
  icon: ReactNode
}
interface Group {
  id: string
  title: string
  icon: ReactNode
  items: Item[]
}

// Menu groups follow the operator's path: watch, collect, process, admin.
const GROUPS: Group[] = [
  {
    id: 'overview',
    title: 'Обзор',
    icon: <LayoutDashboard size={18} />,
    items: [
      { to: '/ops', title: 'Оперативный центр', icon: <Gauge size={16} /> },
      { to: '/incidents', title: 'Инциденты', icon: <Siren size={16} /> },
      { to: '/cmdb', title: 'Карта CMDB', icon: <Network size={16} /> },
    ],
  },
  {
    id: 'collect',
    title: 'Сбор данных',
    icon: <Cable size={18} />,
    items: [
      { to: '/connectors', title: 'Коннекторы', icon: <Cable size={16} /> },
      { to: '/events', title: 'События', icon: <RadioTower size={16} /> },
      { to: '/parse-errors', title: 'Ошибки разбора', icon: <AlertTriangle size={16} /> },
    ],
  },
  {
    id: 'process',
    title: 'Обработка',
    icon: <ListTree size={18} />,
    items: [
      { to: '/rules', title: 'Правила RED/USE', icon: <BookOpen size={16} /> },
      { to: '/maintenance', title: 'Окна обслуживания', icon: <Wrench size={16} /> },
    ],
  },
  {
    id: 'admin',
    title: 'Администрирование',
    icon: <Settings size={18} />,
    items: [
      { to: '/selfcheck', title: 'Самоконтроль', icon: <Activity size={16} /> },
      { to: '/audit', title: 'Журнал аудита', icon: <ClipboardList size={16} /> },
      { to: '/roles', title: 'Роли и группы', icon: <ShieldCheck size={16} /> },
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

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <rect width="32" height="32" rx="7" fill="#1b2228" />
      <path d="M5 16a11 11 0 0 1 22 0z" fill="#f5c518" />
      <path d="M16 16v8a2.5 2.5 0 0 1-5 0" fill="none" stroke="#f5c518" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  )
}

export function Layout({ children }: { children: ReactNode }) {
  const { meta, team, setTeam, user, setUserName, connected } = useApp()
  const [collapsed, setCollapsed] = useState(() => loadBool('umb.menu.collapsed', false))
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({ overview: true, collect: true, process: true, admin: true })
  const [teamOpen, setTeamOpen] = useState(false)
  const [userOpen, setUserOpen] = useState(false)
  const [search, setSearch] = useState('')
  const nav = useNavigate()

  const toggle = () => {
    setCollapsed((c) => {
      try {
        localStorage.setItem('umb.menu.collapsed', c ? '0' : '1')
      } catch {
        /* ignore */
      }
      return !c
    })
  }
  const teamName = team === 'all' ? 'Все команды' : meta?.teams.find((t) => t.id === team)?.name ?? team

  return (
    <div className={`app ${collapsed ? 'app-collapsed' : ''}`}>
      <nav className="sidebar">
        <div className="sb-logo" onClick={() => nav('/ops')}>
          <Logo />
          {!collapsed && (
            <div>
              <div className="sb-brand">Umbrella</div>
              <div className="sb-brand-sub">зонтичный мониторинг</div>
            </div>
          )}
        </div>

        <div className="sb-team">
          <button className="sb-team-btn" onClick={() => setTeamOpen((o) => !o)} title={teamName}>
            <Users size={16} />
            {!collapsed && (
              <>
                <span className="sb-team-name">{teamName}</span>
                <ChevronDown size={14} />
              </>
            )}
          </button>
          {teamOpen && (
            <div className="sb-popup">
              <div className="sb-popup-title">Область видимости</div>
              {[{ id: 'all', name: 'Все команды' }, ...(meta?.teams ?? [])].map((t) => (
                <button
                  key={t.id}
                  className={`sb-popup-item ${t.id === team ? 'active' : ''}`}
                  onClick={() => {
                    setTeam(t.id)
                    setTeamOpen(false)
                  }}
                >
                  {t.name}
                </button>
              ))}
            </div>
          )}
        </div>

        {!collapsed && (
          <form
            className="sb-search"
            onSubmit={(e) => {
              e.preventDefault()
              nav(`/incidents?q=${encodeURIComponent(search)}&view=all`)
            }}
          >
            <Search size={14} />
            <input placeholder="Поиск инцидентов" value={search} onChange={(e) => setSearch(e.target.value)} />
          </form>
        )}

        <div className="sb-menu">
          {GROUPS.map((g) => (
            <div key={g.id} className="sb-group">
              <button className="sb-group-head" onClick={() => setOpenGroups((o) => ({ ...o, [g.id]: !o[g.id] }))} title={g.title}>
                {g.icon}
                {!collapsed && (
                  <>
                    <span>{g.title}</span>
                    <ChevronDown size={14} className={`sb-chev ${openGroups[g.id] ? '' : 'sb-chev-closed'}`} />
                  </>
                )}
              </button>
              {(openGroups[g.id] || collapsed) &&
                g.items.map((it) => (
                  <NavLink key={it.to} to={it.to} className={({ isActive }) => `sb-item ${isActive ? 'sb-item-active' : ''}`} title={it.title}>
                    {it.icon}
                    {!collapsed && <span>{it.title}</span>}
                  </NavLink>
                ))}
            </div>
          ))}
        </div>

        <div className="sb-bottom">
          <div className="sb-conn" title={connected ? 'Живые обновления подключены' : 'Нет соединения с сервером'}>
            <span className={`dot ${connected ? 'dot-ok' : 'dot-critical'}`} />
            {!collapsed && <span>{connected ? 'Онлайн' : 'Нет связи'}</span>}
          </div>
          <div className="sb-user-wrap">
            <button className="sb-user" onClick={() => setUserOpen((o) => !o)} title={user}>
              <span className="avatar">{user.slice(0, 1)}</span>
              {!collapsed && (
                <span className="sb-user-text">
                  <span>{user}</span>
                  <small>демо-вход · OIDC позже</small>
                </span>
              )}
            </button>
            {userOpen && (
              <div className="sb-popup sb-popup-up">
                <div className="sb-popup-title">Войти как</div>
                {(meta?.users ?? []).map((u) => (
                  <button
                    key={u}
                    className={`sb-popup-item ${u === user ? 'active' : ''}`}
                    onClick={() => {
                      setUserName(u)
                      setUserOpen(false)
                    }}
                  >
                    {u}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button className="sb-collapse" onClick={toggle} title={collapsed ? 'Развернуть меню' : 'Свернуть меню'}>
            {collapsed ? <ChevronsRight size={16} /> : <ChevronsLeft size={16} />}
            {!collapsed && <span>Свернуть</span>}
          </button>
        </div>
      </nav>
      <main className="main">{children}</main>
    </div>
  )
}
