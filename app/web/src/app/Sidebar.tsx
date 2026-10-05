import { useEffect, useRef } from 'react'
import { Activity, type LucideIcon } from 'lucide-react'
import { motion } from 'motion/react'
import { useT } from '../i18n'
import { Link, useRouter } from '../router'
import { spring } from '../ui'
import { PATHS } from './routes'
import { SECTIONS, sectionPath } from './settings/sections'
import { strings as settingsStrings } from './settings/strings'
import { strings } from './strings'

type Item = { to: string; label: string; icon: LucideIcon }
type Group = { heading?: string; items: Item[] }

export function Sidebar() {
  const t = useT(strings)
  const ts = useT(settingsStrings)
  const { path } = useRouter()
  const nav = useRef<HTMLElement>(null)

  useEffect(() => {
    const el = nav.current
    const active = el?.querySelector<HTMLElement>('.side-link.active')
    if (!el || !active || el.scrollWidth <= el.clientWidth) return
    el.scrollTo({ left: active.offsetLeft - el.clientWidth / 2 + active.offsetWidth / 2, behavior: 'smooth' })
  }, [path])

  const groups: Group[] = [
    { items: [{ to: PATHS.status, label: t('status.title'), icon: Activity }] },
    { heading: t('nav.settings'), items: SECTIONS.map((s) => ({ to: sectionPath(s), label: ts(s.label), icon: s.icon })) },
  ]
  return (
    <nav ref={nav} className="sidebar" aria-label={t('nav.label')}>
      {groups.map((g, i) => (
        <div key={i} className="side-group">
          {g.heading && <div className="side-heading">{g.heading}</div>}
          {g.items.map((item) => {
            const active = item.to === path
            return (
              <Link key={item.to} to={item.to} className={`side-link ${active ? 'active' : ''}`} aria-current={active ? 'page' : undefined}>
                {active && <motion.span layoutId="side-pill" className="side-pill" transition={spring} />}
                <item.icon size={17} aria-hidden />
                <span>{item.label}</span>
              </Link>
            )
          })}
        </div>
      ))}
    </nav>
  )
}
