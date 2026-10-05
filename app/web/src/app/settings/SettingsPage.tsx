import { useEffect } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useT } from '../../i18n'
import { useRouter } from '../../router'
import { PATHS, matches } from '../routes'
import { SECTIONS, sectionFor, sectionPath } from './sections'
import { strings } from './strings'

export function SettingsPage() {
  const t = useT(strings)
  const { path, navigate } = useRouter()
  const current = sectionFor(path)

  useEffect(() => {
    if (!current && matches(path, PATHS.settings)) navigate(sectionPath(SECTIONS[0]), { replace: true })
  }, [current, path, navigate])

  return (
    <AnimatePresence mode="wait" initial={false}>
      {current && (
        <motion.section
          key={current.id}
          className="settings-content"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
        >
          <div className="page-head">
            <div>
              <h1>{t(current.label)}</h1>
              <p className="muted">{t('settings.subtitle')}</p>
            </div>
          </div>
          <current.Component />
        </motion.section>
      )}
    </AnimatePresence>
  )
}
