import { Fragment, useCallback, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Building2, ChevronDown, LayoutGrid, Settings, Workflow, type LucideIcon } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useT } from '../i18n'
import { Link, useRouter } from '../router'
import { spring } from '../ui'
import { navStrings } from './navStrings'
import { GROUPS, owns, visiblePages, type Group, type PageDef } from './pages'
import { useSession } from './session'

const COMPACT = '(max-width: 860px)'
const openKey = (group: Group) => `umbrella.sidebar.${group}`
const collapsedKey = (user: string) => `umbrella.sidebar.collapsed.${user}`
const GROUP_ICONS: Record<Group, LucideIcon> = { overview: LayoutGrid, automation: Workflow, org: Building2, settings: Settings }
// Groups shown open the first time, before the user has opened or closed them.
const OPEN_BY_DEFAULT: Group[] = ['overview']

function useCompact() {
  return useSyncExternalStore(
    (notify) => {
      const mq = window.matchMedia(COMPACT)
      mq.addEventListener('change', notify)
      return () => mq.removeEventListener('change', notify)
    },
    () => window.matchMedia(COMPACT).matches,
  )
}

function readOpen(group: Group) {
  try {
    const v = window.localStorage.getItem(openKey(group))
    return v === null ? OPEN_BY_DEFAULT.includes(group) : v === '1'
  } catch {
    return OPEN_BY_DEFAULT.includes(group)
  }
}

function readCollapsed(user: string) {
  try {
    return window.localStorage.getItem(collapsedKey(user)) === '1'
  } catch {
    return false
  }
}

// useCollapsed keeps the collapsed state of the sidebar per user in this browser.
export function useCollapsed(user: string) {
  const [collapsed, setCollapsed] = useState(() => readCollapsed(user))
  useEffect(() => setCollapsed(readCollapsed(user)), [user])
  const toggle = useCallback(
    () =>
      setCollapsed((v) => {
        try {
          window.localStorage.setItem(collapsedKey(user), v ? '0' : '1')
        } catch {
          // The choice then lasts until the page is reloaded.
        }
        return !v
      }),
    [user],
  )
  return [collapsed, toggle] as const
}

// Tip is the label shown next to an icon of the collapsed sidebar. It is drawn in a portal
// because the sidebar scrolls and would clip it.
function Tip({ anchor, children }: { anchor: HTMLElement | null; children: ReactNode }) {
  if (!anchor) return null
  const r = anchor.getBoundingClientRect()
  return createPortal(
    <motion.div
      role="tooltip"
      className="side-tip"
      style={{ top: r.top + r.height / 2, left: r.right + 10, y: '-50%' }}
      initial={{ opacity: 0, x: -4 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ duration: 0.12 }}
    >
      {children}
    </motion.div>,
    document.body,
  )
}

function writeOpen(group: Group, v: boolean) {
  try {
    window.localStorage.setItem(openKey(group), v ? '1' : '0')
  } catch {
    return
  }
}

function SideLink({ page, active, rail }: { page: PageDef; active: boolean; rail?: boolean }) {
  const t = useT(navStrings)
  const compact = useCompact()
  const [tip, setTip] = useState<HTMLElement | null>(null)
  const label = t(`page.${page.id}`)
  const show = (e: { currentTarget: HTMLElement }) => rail && !compact && setTip(e.currentTarget)
  const hide = () => setTip(null)
  useEffect(() => {
    if (!rail) setTip(null)
  }, [rail])
  return (
    <Link
      to={page.path}
      className={`side-link ${active ? 'active' : ''}`}
      aria-current={active ? 'page' : undefined}
      aria-label={rail ? label : undefined}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
      onClick={hide}
    >
      {active && <motion.span layoutId="side-pill" className="side-pill" transition={spring} />}
      <page.icon size={17} aria-hidden />
      <span className="side-label">{label}</span>
      {rail && <Tip anchor={tip}>{label}</Tip>}
    </Link>
  )
}

function GroupAccordion({ group, icon: Icon, pages, path }: { group: Group; icon: LucideIcon; pages: PageDef[]; path: string }) {
  const t = useT(navStrings)
  const compact = useCompact()
  const hasActive = pages.some((p) => owns(p, path))
  const [open, setOpen] = useState(() => hasActive || readOpen(group))

  useEffect(() => {
    if (hasActive) setOpen(true)
  }, [hasActive])

  const toggle = () =>
    setOpen((v) => {
      writeOpen(group, !v)
      return !v
    })
  const shown = open || compact

  return (
    <div className="side-group">
      <button type="button" className={`side-accordion ${hasActive ? 'has-active' : ''}`} aria-expanded={shown} onClick={toggle}>
        <Icon size={17} aria-hidden />
        <span>{t(`group.${group}`)}</span>
        <motion.span className="side-chevron" animate={{ rotate: shown ? 180 : 0 }} transition={spring}>
          <ChevronDown size={16} />
        </motion.span>
      </button>
      <AnimatePresence initial={false}>
        {shown && (
          <motion.div
            className="side-children"
            initial={compact ? false : { height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.22, ease: [0.22, 1, 0.36, 1] }}
          >
            <div className="side-children-inner">
              {pages.map((p) => (
                <SideLink key={p.id} page={p} active={owns(p, path)} />
              ))}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

export function Sidebar({ collapsed }: { collapsed: boolean }) {
  const t = useT(navStrings)
  const { path } = useRouter()
  const { can } = useSession()
  const nav = useRef<HTMLElement>(null)
  const pages = visiblePages(can)
  const groups = GROUPS.map((g) => ({ group: g, pages: pages.filter((p) => p.group === g) })).filter((g) => g.pages.length > 0)

  useEffect(() => {
    const el = nav.current
    const active = el?.querySelector<HTMLElement>('.side-link.active')
    if (!el || !active || el.scrollWidth <= el.clientWidth) return
    const box = el.getBoundingClientRect()
    const item = active.getBoundingClientRect()
    el.scrollBy({ left: item.left - box.left - box.width / 2 + item.width / 2, behavior: 'smooth' })
  }, [path, collapsed])

  return (
    <nav ref={nav} id="app-sidebar" className={`sidebar ${collapsed ? 'collapsed' : ''}`} aria-label={t('nav.label')}>
      {collapsed
        ? groups.map(({ group, pages }, i) => (
            <Fragment key={group}>
              {i > 0 && <span className="side-sep" aria-hidden />}
              <div className="side-group" role="group" aria-label={t(`group.${group}`)}>
                {pages.map((p) => (
                  <SideLink key={p.id} page={p} active={owns(p, path)} rail />
                ))}
              </div>
            </Fragment>
          ))
        : groups.map(({ group, pages }) => <GroupAccordion key={group} group={group} icon={GROUP_ICONS[group]} pages={pages} path={path} />)}
    </nav>
  )
}
