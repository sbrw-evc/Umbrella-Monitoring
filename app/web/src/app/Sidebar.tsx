import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { Building2, ChevronDown, Settings, type LucideIcon } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useT } from '../i18n'
import { Link, useRouter } from '../router'
import { spring } from '../ui'
import { navStrings } from './navStrings'
import { owns, visiblePages, type Group, type PageDef } from './pages'
import { useSession } from './session'

const COMPACT = '(max-width: 860px)'
const openKey = (group: Group) => `umbrella.sidebar.${group}`

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
    return window.localStorage.getItem(openKey(group)) === '1'
  } catch {
    return false
  }
}

function writeOpen(group: Group, v: boolean) {
  try {
    window.localStorage.setItem(openKey(group), v ? '1' : '0')
  } catch {
    return
  }
}

function SideLink({ page, active }: { page: PageDef; active: boolean }) {
  const t = useT(navStrings)
  return (
    <Link to={page.path} className={`side-link ${active ? 'active' : ''}`} aria-current={active ? 'page' : undefined}>
      {active && <motion.span layoutId="side-pill" className="side-pill" transition={spring} />}
      <page.icon size={17} aria-hidden />
      <span>{t(`page.${page.id}`)}</span>
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

export function Sidebar() {
  const t = useT(navStrings)
  const { path } = useRouter()
  const { can } = useSession()
  const nav = useRef<HTMLElement>(null)
  const pages = visiblePages(can)
  const main = pages.filter((p) => p.group === 'main')
  const org = pages.filter((p) => p.group === 'org')
  const settings = pages.filter((p) => p.group === 'settings')

  useEffect(() => {
    const el = nav.current
    const active = el?.querySelector<HTMLElement>('.side-link.active')
    if (!el || !active || el.scrollWidth <= el.clientWidth) return
    const box = el.getBoundingClientRect()
    const item = active.getBoundingClientRect()
    el.scrollBy({ left: item.left - box.left - box.width / 2 + item.width / 2, behavior: 'smooth' })
  }, [path])

  return (
    <nav ref={nav} className="sidebar" aria-label={t('nav.label')}>
      {main.length > 0 && (
        <div className="side-group">
          {main.map((p) => (
            <SideLink key={p.id} page={p} active={owns(p, path)} />
          ))}
        </div>
      )}
      {org.length > 0 && <GroupAccordion group="org" icon={Building2} pages={org} path={path} />}
      {settings.length > 0 && <GroupAccordion group="settings" icon={Settings} pages={settings} path={path} />}
    </nav>
  )
}
