import { useEffect, useState } from 'react'
import { ExternalLink, Link2, Link2Off, Plus, Search, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Button, formatDate, Input } from '../../ui'
import { StatusPill } from '../cis/Badges'
import type { CIList } from '../cis/types'
import { useSession } from '../session'
import { OptionSelect } from './Pickers'
import { blockedDependencies } from './ServiceEditor'
import { strings } from './strings'
import type { Service, ServiceList } from './types'

const MAX_RESULTS = 8

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return v
}

type Props = { service: Service; editable: boolean; onChanged: (s: Service) => void }

// CIBindings lists the configuration items of the service and binds or unbinds them.
export function CIBindings({ service, editable, onChanged }: Props) {
  const t = useT(strings)
  const { can } = useSession()
  const act = useAction()
  const [query, setQuery] = useState('')
  const [adding, setAdding] = useState(false)
  const q = useDebounced(query.trim(), 250)
  const found = useResource<CIList>(adding ? `/api/cis${q ? `?q=${encodeURIComponent(q)}` : ''}` : '', 0)
  const free = (found.data?.items ?? []).filter((ci) => !service.ci_ids.includes(ci.id))
  const ciLabel = (kind: string) => t(`svc.ci.kind.${kind}`)

  const bind = (id: string) => void act.run(async () => onChanged(await api<Service>('POST', `/api/services/${service.id}/cis`, { ids: [id] })))
  const unbind = (id: string) => void act.run(async () => onChanged(await api<Service>('DELETE', `/api/services/${service.id}/cis/${id}`)))

  return (
    <section className="svc-bind" aria-label={t('svc.cis')}>
      <header className="svc-bind-head">
        <h3>
          {t('svc.cis')}
          <span className="muted">{service.cis.length}</span>
        </h3>
        {editable && can('cis:view') && !adding && (
          <Button variant="ghost" onClick={() => setAdding(true)}>
            <Plus size={16} />
            {t('svc.cis.bind')}
          </Button>
        )}
      </header>
      {service.netbox && <p className="muted svc-bind-note">{t('svc.cis.netboxNote', { slug: service.netbox.slug })}</p>}
      {service.cis.length === 0 ? (
        <p className="muted">{t('svc.cis.none')}</p>
      ) : (
        <ul className="svc-bind-list">
          <AnimatePresence initial={false}>
            {service.cis.map((ci) => (
              <motion.li key={ci.id} layout initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, height: 0 }}>
                <span className={`svc-bind-name ${ci.deleted ? 'muted' : ''}`}>{ci.deleted ? t('svc.cis.deleted') : ci.name}</span>
                {!ci.deleted && <span className="muted svc-bind-kind">{ciLabel(ci.kind)}</span>}
                {ci.netbox && <span className="pill ci-source ci-source-netbox">NetBox</span>}
                {!ci.deleted && <StatusPill status={ci.status} />}
                {editable && (
                  <button type="button" className="icon-btn" disabled={act.busy} onClick={() => unbind(ci.id)} aria-label={t('svc.cis.unbind', { name: ci.name || ci.id })}>
                    <X size={15} />
                  </button>
                )}
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
      )}
      <AnimatePresence initial={false}>
        {adding && (
          <motion.div className="svc-bind-add" initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }}>
            <div className="svc-bind-search">
              <label className="svc-search">
                <Search size={16} aria-hidden />
                <Input type="search" autoFocus value={query} placeholder={t('svc.cis.search')} aria-label={t('svc.cis.search')} onChange={(e) => setQuery(e.target.value)} />
              </label>
              <Button variant="ghost" onClick={() => setAdding(false)}>
                {t('svc.cis.done')}
              </Button>
            </div>
            {found.data && free.length === 0 && <p className="muted">{t('svc.cis.noMatch')}</p>}
            <ul className="svc-bind-list">
              {free.slice(0, MAX_RESULTS).map((ci) => (
                <li key={ci.id}>
                  <span className="svc-bind-name">{ci.name}</span>
                  <span className="muted svc-bind-kind">{ciLabel(ci.kind)}</span>
                  {ci.netbox && <span className="pill ci-source ci-source-netbox">NetBox</span>}
                  <StatusPill status={ci.status} />
                  <Button variant="ghost" disabled={act.busy} onClick={() => bind(ci.id)}>
                    <Plus size={15} />
                    {t('svc.cis.add')}
                  </Button>
                </li>
              ))}
            </ul>
            {free.length > MAX_RESULTS && <p className="muted">{t('svc.cis.more', { n: free.length - MAX_RESULTS })}</p>}
            <ErrorBanner error={found.error} strings={strings} />
          </motion.div>
        )}
      </AnimatePresence>
      <ErrorBanner error={act.error} strings={strings} />
    </section>
  )
}

// DependencyBindings adds and removes the services this one depends on.
export function DependencyBindings({ service, editable, onChanged }: Props) {
  const t = useT(strings)
  const act = useAction()
  const all = useResource<ServiceList>(editable ? '/api/services' : '', 0)
  const services = all.data?.services ?? []
  const blocked = blockedDependencies(services, service.id)
  const options = services.filter((s) => !blocked.has(s.id) && !service.depends_on.includes(s.id)).map((s) => ({ id: s.id, label: s.name }))
  const add = (id: string) => id && void act.run(async () => onChanged(await api<Service>('POST', `/api/services/${service.id}/dependencies`, { ids: [id] })))
  const remove = (id: string) => void act.run(async () => onChanged(await api<Service>('DELETE', `/api/services/${service.id}/dependencies/${id}`)))
  return (
    <section className="svc-bind" aria-label={t('svc.field.dependencies')}>
      <header className="svc-bind-head">
        <h3>
          {t('svc.field.dependencies')}
          <span className="muted">{service.dependencies.length}</span>
        </h3>
      </header>
      {service.dependencies.length === 0 && <p className="muted">{t('svc.deps.none')}</p>}
      <ul className="svc-bind-list">
        {service.dependencies.map((d) => (
          <li key={d.id}>
            <span className="svc-bind-name">{d.name}</span>
            {editable && (
              <button type="button" className="icon-btn" disabled={act.busy} onClick={() => remove(d.id)} aria-label={t('svc.remove', { name: d.name })}>
                <X size={15} />
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && options.length > 0 && (
        <OptionSelect value="" options={options} placeholder={t('svc.field.dependencies.add')} onChange={add} disabled={act.busy} />
      )}
      <ErrorBanner error={act.error ?? all.error} strings={strings} />
    </section>
  )
}

// NetBoxLink links the service to its NetBox tag or removes the link.
export function NetBoxLink({ service, editable, onChanged }: Props) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const act = useAction()
  const nb = service.netbox
  const link = () => void act.run(async () => onChanged(await api<Service>('POST', `/api/services/${service.id}/netbox`)))
  const unlink = () => void act.run(async () => onChanged(await api<Service>('DELETE', `/api/services/${service.id}/netbox`)))
  return (
    <section className="svc-bind" aria-label="NetBox">
      <header className="svc-bind-head">
        <h3>NetBox</h3>
        {editable &&
          (nb ? (
            <Button variant="ghost" busy={act.busy} onClick={unlink}>
              <Link2Off size={16} />
              {t('svc.netbox.unlink')}
            </Button>
          ) : (
            <Button variant="ghost" busy={act.busy} onClick={link}>
              <Link2 size={16} />
              {t('svc.netbox.link')}
            </Button>
          ))}
      </header>
      {nb ? (
        <div className="svc-netbox">
          <a href={nb.url} target="_blank" rel="noopener noreferrer">
            {t('svc.netbox.tag')} <code>{nb.slug}</code>
            <ExternalLink size={13} aria-hidden />
          </a>
          {nb.synced_at && (
            <span className="muted">
              {t('svc.netbox.synced')}: {formatDate(nb.synced_at, locale, timezone)}
            </span>
          )}
        </div>
      ) : (
        <p className="muted">{t('svc.netbox.none')}</p>
      )}
      <ErrorBanner error={act.error} strings={strings} />
    </section>
  )
}
