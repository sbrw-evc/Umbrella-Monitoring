import { useEffect, useState } from 'react'
import { Check, Copy, ExternalLink, Plus, Search, X } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Button, Field, formatDate, Input, Modal, Segmented, Select, Switch, Textarea, TimezoneSelect } from '../../ui'
import { SummaryCard } from '../profile/SummaryCard'
import { useSession } from '../session'
import { strings as incStrings } from '../incidents/strings'
import { strings } from './strings'
import '../maintenance/maintenance.css'
import './tv.css'

type Ref = { id: string; name: string; missing?: boolean }
type Severity = 'critical' | 'error' | 'warning' | 'info'
type Board = {
  id: string
  name: string
  slug: string
  severities: Severity[]
  show_acknowledged: boolean
  show_maintenance: boolean
  allowed_sources: string[]
  refresh: number
  locale: '' | 'en' | 'ru'
  timezone: string
  theme: 'dark' | 'light'
  disabled: boolean
  cis: Ref[]
  services: Ref[]
  teams: Ref[]
  last_seen?: { at: string; ip: string }
}
type Page = { boards: Board[]; your_ip: string }
type Kind = 'services' | 'cis' | 'teams'

const SEVERITIES: Severity[] = ['critical', 'error', 'warning', 'info']
const KINDS: Kind[] = ['services', 'cis', 'teams']
const REFRESH = [5, 10, 15, 30, 60, 120, 300, 600]
const both = { en: { ...incStrings.en, ...strings.en }, ru: { ...incStrings.ru, ...strings.ru } }

const boardURL = (slug: string) => `${window.location.origin}/tv/${slug}`
const lines = (v: string) =>
  v
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean)

export function TvBoardsPage() {
  const t = useT(both)
  const { can } = useSession()
  const editor = can('tv:edit')
  const [epoch, setEpoch] = useState(0)
  const page = useResource<Page>('/api/tv-boards', epoch)
  const [editing, setEditing] = useState<Board | 'new' | null>(null)
  const act = useAction()
  const reload = () => setEpoch((e) => e + 1)
  useEffect(() => {
    const id = window.setInterval(() => document.visibilityState === 'visible' && setEpoch((e) => e + 1), 30_000)
    return () => window.clearInterval(id)
  }, [])
  const run = (fn: () => Promise<unknown>) =>
    act.run(async () => {
      await fn()
      reload()
    })
  const boards = page.data?.boards ?? []

  return (
    <div className="stack">
      {editor && (
        <div className="row">
          <Button variant="primary" onClick={() => setEditing('new')}>
            <Plus size={16} aria-hidden />
            {t('tv.new')}
          </Button>
        </div>
      )}
      <ErrorBanner error={page.error ?? act.error} strings={both} />
      {page.data && boards.length === 0 && <p className="muted card mw-empty">{t('tv.empty')}</p>}
      <div className="tv-cards">
        {boards.map((b) => (
          <BoardCard key={b.id} b={b} editor={editor} busy={act.busy} onEdit={() => setEditing(b)} run={run} />
        ))}
      </div>
      {page.data && <Proxies editor={editor} yourIP={page.data.your_ip} onSaved={reload} />}
      <Editor
        value={editing}
        yourIP={page.data?.your_ip ?? ''}
        onClose={() => setEditing(null)}
        onSaved={() => {
          setEditing(null)
          reload()
        }}
      />
    </div>
  )
}

function CopyLink({ slug }: { slug: string }) {
  const t = useT(both)
  const [done, setDone] = useState(false)
  return (
    <span className="tv-link">
      <code className="tv-url" title={boardURL(slug)}>
        {boardURL(slug)}
      </code>
      <button
        type="button"
        className="icon-btn"
        aria-label={t('tv.copy')}
        title={done ? t('tv.copied') : t('tv.copy')}
        onClick={() =>
          void navigator.clipboard?.writeText(boardURL(slug)).then(() => {
            setDone(true)
            window.setTimeout(() => setDone(false), 1500)
          })
        }
      >
        {done ? <Check size={15} /> : <Copy size={15} />}
      </button>
      <a className="icon-btn" href={`/tv/${slug}`} target="_blank" rel="noreferrer" aria-label={t('tv.open')} title={t('tv.open')}>
        <ExternalLink size={15} />
      </a>
    </span>
  )
}

function BoardCard({ b, editor, busy, onEdit, run }: { b: Board; editor: boolean; busy: boolean; onEdit: () => void; run: (fn: () => Promise<unknown>) => void }) {
  const t = useT(both)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const scope = [...b.services, ...b.cis, ...b.teams]
  const counts = [
    b.services.length > 0 && t('tv.services', { n: b.services.length }),
    b.cis.length > 0 && t('tv.cis', { n: b.cis.length }),
    b.teams.length > 0 && t('tv.teams', { n: b.teams.length }),
  ].filter(Boolean)
  const extra = [b.show_acknowledged && t('tv.extra.acked'), b.show_maintenance && t('tv.extra.mw')].filter(Boolean)
  const lang = b.locale === 'ru' ? 'русский' : b.locale === 'en' ? 'English' : t('tv.default.short')
  const zone = b.timezone || t('tv.default.short')
  return (
    <SummaryCard
      title={b.name}
      badge={<span className={`pill ${b.disabled ? 'pill-off' : 'pill-ok'}`}>{b.disabled ? t('tv.off') : t('tv.on')}</span>}
      rows={[
        [t('tv.link'), <CopyLink key="link" slug={b.slug} />],
        [
          t('tv.row.scope'),
          scope.length === 0 ? (
            t('tv.row.scope.all')
          ) : (
            <span key="scope" title={scope.map((r) => r.name).join(', ')}>
              {scope
                .slice(0, 4)
                .map((r) => r.name)
                .join(', ')}
              {scope.length > 4 && <span className="muted"> +{scope.length - 4}</span>}
              <div className="muted tv-sub">{counts.join(' · ')}</div>
            </span>
          ),
        ],
        [t('tv.row.severity'), b.severities.length ? b.severities.map((s) => t(`inc.sev.${s}`)).join(', ') : t('tv.row.severity.all')],
        [t('tv.row.extra'), extra.length ? extra.join(', ') : t('tv.row.extra.none')],
        [t('tv.row.sources'), b.allowed_sources.join(', ')],
        [t('tv.row.screen'), t('tv.screen', { n: b.refresh, locale: lang, zone, theme: t(`tv.theme.${b.theme}`).toLowerCase() })],
        [t('tv.row.seen'), b.last_seen ? t('tv.seen', { time: formatDate(b.last_seen.at, locale, timezone), ip: b.last_seen.ip }) : t('tv.row.seen.never')],
      ]}
      footer={
        editor && (
          <div className="row tv-actions">
            <Button onClick={onEdit}>{t('tv.edit')}</Button>
            <Button variant="ghost" busy={busy} onClick={() => window.confirm(t('tv.rotate.confirm', { name: b.name })) && run(() => api('POST', `/api/tv-boards/${b.id}/rotate`))}>
              {t('tv.rotate')}
            </Button>
            <Button variant="ghost" busy={busy} onClick={() => window.confirm(t('tv.delete.confirm', { name: b.name })) && run(() => api('DELETE', `/api/tv-boards/${b.id}`))}>
              {t('tv.delete')}
            </Button>
          </div>
        )
      }
    />
  )
}

function Proxies({ editor, yourIP, onSaved }: { editor: boolean; yourIP: string; onSaved: () => void }) {
  const t = useT(both)
  const [epoch, setEpoch] = useState(0)
  const current = useResource<{ trusted_proxies: string[] }>('/api/tv-boards/settings', epoch)
  const [text, setText] = useState('')
  const [saved, setSaved] = useState(false)
  const save = useAction()
  useEffect(() => {
    if (current.data) setText(current.data.trusted_proxies.join('\n'))
  }, [current.data])
  const list = current.data?.trusted_proxies ?? []
  return (
    <SummaryCard
      title={t('tv.proxies.title')}
      text={
        <>
          {t('tv.proxies.text')} {yourIP && t('tv.yourip', { ip: yourIP })}
        </>
      }
      rows={[[t('tv.proxies.label'), list.length ? list.join(', ') : t('tv.proxies.none')]]}
    >
      {editor && (
        <div className="stack tv-proxies">
          <Field label={t('tv.proxies.label')}>
            {(id) => <Textarea id={id} rows={3} value={text} placeholder="10.0.0.5" onChange={(e) => setText(e.target.value)} />}
          </Field>
          <ErrorBanner error={current.error ?? save.error} strings={both} />
          <div className="row">
            <Button
              busy={save.busy}
              onClick={() =>
                void save.run(async () => {
                  await api('PUT', '/api/tv-boards/settings', { trusted_proxies: lines(text) })
                  setSaved(true)
                  window.setTimeout(() => setSaved(false), 2000)
                  setEpoch((e) => e + 1)
                  onSaved()
                })
              }
            >
              {saved ? t('tv.proxies.saved') : t('tv.proxies.save')}
            </Button>
          </div>
        </div>
      )}
    </SummaryCard>
  )
}

type Draft = {
  name: string
  services: Ref[]
  cis: Ref[]
  teams: Ref[]
  severities: Severity[]
  show_acknowledged: boolean
  show_maintenance: boolean
  sources: string
  refresh: number
  locale: '' | 'en' | 'ru'
  timezone: string
  theme: 'dark' | 'light'
  disabled: boolean
}

function draftOf(v: Board | 'new' | null): Draft {
  if (v && v !== 'new')
    return {
      name: v.name,
      services: v.services,
      cis: v.cis,
      teams: v.teams,
      severities: v.severities,
      show_acknowledged: v.show_acknowledged,
      show_maintenance: v.show_maintenance,
      sources: v.allowed_sources.join('\n'),
      refresh: v.refresh,
      locale: v.locale,
      timezone: v.timezone,
      theme: v.theme,
      disabled: v.disabled,
    }
  return {
    name: '',
    services: [],
    cis: [],
    teams: [],
    severities: [],
    show_acknowledged: true,
    show_maintenance: false,
    sources: '',
    refresh: 15,
    locale: '',
    timezone: '',
    theme: 'dark',
    disabled: false,
  }
}

function Editor({ value, yourIP, onClose, onSaved }: { value: Board | 'new' | null; yourIP: string; onClose: () => void; onSaved: () => void }) {
  const t = useT(both)
  const [d, setD] = useState<Draft>(() => draftOf(value))
  const [q, setQ] = useState('')
  const save = useAction()
  useEffect(() => {
    setD(draftOf(value))
    setQ('')
    save.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])
  const found = useResource<Record<Kind, Ref[]>>(value && q.trim() ? `/api/tv-boards/targets?q=${encodeURIComponent(q.trim())}` : '', 0)
  const editing = value && value !== 'new' ? value : null

  const submit = () =>
    save.run(async () => {
      const body = {
        name: d.name,
        service_ids: d.services.map((r) => r.id),
        ci_ids: d.cis.map((r) => r.id),
        team_ids: d.teams.map((r) => r.id),
        severities: d.severities,
        show_acknowledged: d.show_acknowledged,
        show_maintenance: d.show_maintenance,
        allowed_sources: lines(d.sources),
        refresh: d.refresh,
        locale: d.locale,
        timezone: d.timezone,
        theme: d.theme,
        disabled: d.disabled,
      }
      await api(editing ? 'PUT' : 'POST', editing ? `/api/tv-boards/${editing.id}` : '/api/tv-boards', body)
      onSaved()
    })
  const add = (kind: Kind, r: Ref) => !d[kind].some((x) => x.id === r.id) && setD({ ...d, [kind]: [...d[kind], r] })
  const drop = (kind: Kind, id: string) => setD({ ...d, [kind]: d[kind].filter((x) => x.id !== id) })
  const toggleSev = (s: Severity) => setD({ ...d, severities: d.severities.includes(s) ? d.severities.filter((x) => x !== s) : [...d.severities, s] })
  const mine = yourIP && !lines(d.sources).includes(yourIP)
  const refreshLabel = (n: number) => (n < 60 ? t('tv.refresh.s', { n }) : t('tv.refresh.m', { n: n / 60 }))
  const results = found.data && q.trim() ? found.data : null

  return (
    <Modal
      open={value !== null}
      title={editing ? t('tv.dialog.edit') : t('tv.dialog.new')}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>{t('tv.cancel')}</Button>
          <Button variant="primary" busy={save.busy} onClick={() => void submit()}>
            {t('tv.save')}
          </Button>
        </>
      }
    >
      <div className="stack mw-editor tv-editor">
        <Field label={t('tv.name')}>{(id) => <Input id={id} value={d.name} placeholder={t('tv.name.ph')} onChange={(e) => setD({ ...d, name: e.target.value })} />}</Field>
        <Field label={t('tv.scope')} hint={t('tv.scope.hint')}>
          {(id) => (
            <div className="mw-search">
              <Search size={16} className="mw-search-icon" aria-hidden />
              <Input id={id} value={q} placeholder={t('tv.search')} onChange={(e) => setQ(e.target.value)} />
            </div>
          )}
        </Field>
        {results && (
          <div className="mw-results">
            {KINDS.every((k) => results[k].length === 0) && <p className="muted">{t('tv.nothing')}</p>}
            {KINDS.map(
              (k) =>
                results[k].length > 0 && (
                  <div key={k}>
                    <div className="mw-results-head">{t(`tv.found.${k}`)}</div>
                    {results[k].map((r) => (
                      <button key={r.id} type="button" className="mw-result" disabled={d[k].some((x) => x.id === r.id)} onClick={() => add(k, r)}>
                        <Plus size={14} aria-hidden /> {r.name}
                      </button>
                    ))}
                  </div>
                ),
            )}
          </div>
        )}
        {KINDS.some((k) => d[k].length > 0) && (
          <div className="mw-chosen">
            {KINDS.map((k) =>
              d[k].map((r) => (
                <span key={k + r.id} className={`mw-tag mw-tag-${k}`}>
                  {r.name}
                  {r.missing && <span className="muted"> ({t('tv.missing')})</span>}
                  <button type="button" aria-label={t('tv.remove', { name: r.name })} onClick={() => drop(k, r.id)}>
                    <X size={13} />
                  </button>
                </span>
              )),
            )}
          </div>
        )}
        <Field label={t('tv.severities')} hint={t('tv.severities.hint')}>
          {() => (
            <span className="mw-chips">
              {SEVERITIES.map((s) => (
                <button key={s} type="button" className={`mw-chip ${d.severities.includes(s) ? 'on' : ''}`} aria-pressed={d.severities.includes(s)} onClick={() => toggleSev(s)}>
                  {t(`inc.sev.${s}`)}
                </button>
              ))}
            </span>
          )}
        </Field>
        <Switch checked={d.show_acknowledged} onChange={(v) => setD({ ...d, show_acknowledged: v })} label={t('tv.show.acked')} hint={t('tv.show.acked.hint')} />
        <Switch checked={d.show_maintenance} onChange={(v) => setD({ ...d, show_maintenance: v })} label={t('tv.show.mw')} hint={t('tv.show.mw.hint')} />
        <Field
          label={t('tv.sources')}
          hint={
            <>
              {t('tv.sources.hint')}
              {mine && (
                <>
                  {' '}
                  <button type="button" className="cn-link" onClick={() => setD({ ...d, sources: [...lines(d.sources), yourIP].join('\n') })}>
                    {t('tv.sources.mine', { ip: yourIP })}
                  </button>
                </>
              )}
            </>
          }
        >
          {(id) => <Textarea id={id} rows={3} value={d.sources} placeholder={t('tv.sources.ph')} onChange={(e) => setD({ ...d, sources: e.target.value })} />}
        </Field>
        <div className="tv-editor-grid">
          <Field label={t('tv.refresh')} hint={t('tv.refresh.hint')}>
            {(id) => (
              <Select id={id} value={String(d.refresh)} onChange={(e) => setD({ ...d, refresh: Number(e.target.value) })}>
                {!REFRESH.includes(d.refresh) && <option value={String(d.refresh)}>{refreshLabel(d.refresh)}</option>}
                {REFRESH.map((n) => (
                  <option key={n} value={String(n)}>
                    {refreshLabel(n)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t('tv.locale')}>
            {(id) => (
              <Select id={id} value={d.locale} onChange={(e) => setD({ ...d, locale: e.target.value as Draft['locale'] })}>
                <option value="">{t('tv.locale.default')}</option>
                <option value="ru">Русский</option>
                <option value="en">English</option>
              </Select>
            )}
          </Field>
          <Field label={t('tv.timezone')}>
            {(id) => <TimezoneSelect id={id} value={d.timezone} defaultLabel={t('tv.timezone.default')} onChange={(v) => setD({ ...d, timezone: v })} />}
          </Field>
          <Field label={t('tv.theme')}>
            {() => (
              <Segmented
                label={t('tv.theme')}
                value={d.theme}
                onChange={(v) => setD({ ...d, theme: v })}
                options={[
                  { value: 'dark', label: t('tv.theme.dark') },
                  { value: 'light', label: t('tv.theme.light') },
                ]}
              />
            )}
          </Field>
        </div>
        {editing && <Switch checked={d.disabled} onChange={(v) => setD({ ...d, disabled: v })} label={t('tv.disabled')} hint={t('tv.disabled.hint')} />}
        <ErrorBanner error={save.error} strings={both} />
      </div>
    </Modal>
  )
}
