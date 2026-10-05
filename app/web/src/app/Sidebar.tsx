import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { ChevronDown, Settings } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useT } from '../i18n'
import { Link, useRouter } from '../router'
import { spring } from '../ui'
import { navStrings } from './navStrings'
import { visiblePages, type PageDef } from './pages'
import { useSession } from './session'

const COMPACT = '(max-width: 860px)'
const OPEN_KEY = 'umbrella.sidebar.settings'

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

function readOpen() {
  try {
    return window.localStorage.getItem(OPEN_KEY) === '1'
  } catch {
    return false
  }
}

function writeOpen(v: boolean) {
  try {
    window.localStorage.setItem(OPEN_KEY, v ? '1' : '0')
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

function SettingsAccordion({ pages, path }: { pages: PageDef[]; path: string }) {
  const t = useT(navStrings)
  const compact = useCompact()
  const hasActive = pages.some((p) => p.path === path)
  const [open, setOpen] = useState(() => hasActive || readOpen())

  useEffect(() => {
    if (hasActive) setOpen(true)
  }, [hasActive])

  const toggle = () =>
    setOpen((v) => {
      writeOpen(!v)
      return !v
    })
  const shown = open || compact

  return (
    <div className="side-group">
      <button type="button" className={`side-accordion ${hasActive ? 'has-active' : ''}`} aria-expanded={shown} onClick={toggle}>
        <Settings size={17} aria-hidden />
        <span>{t('group.settings')}</span>
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
                <SideLink key={p.id} page={p} active={p.path === path} />
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
            <SideLink key={p.id} page={p} active={p.path === path} />
          ))}
        </div>
      )}
      {org.length > 0 && (
        <div className="side-group">
          <div className="side-heading">{t('group.org')}</div>
          {org.map((p) => (
            <SideLink key={p.id} page={p} active={p.path === path} />
          ))}
        </div>
      )}
      {settings.length > 0 && <SettingsAccordion pages={settings} path={path} />}
    </nav>
  )
}
