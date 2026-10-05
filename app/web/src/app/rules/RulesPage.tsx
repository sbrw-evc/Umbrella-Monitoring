import { useEffect, useState } from 'react'
import { Plus } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { useAction, useResource } from '../../connections/useRequest'
import { useLocale, useT } from '../../i18n'
import { Banner, Button, Field, formatDate, Input, Modal, Segmented, Select, Switch, Textarea } from '../../ui'
import { useSession } from '../session'
import { strings } from './strings'
import '../connectors/connectors.css'
import './rules.css'

type Method = 'red' | 'use'
type Rule = {
  id: string
  name: string
  method: Method
  signal: string
  source_id: string
  query: string
  ci_label: string
  op: string
  threshold: number
  for: string
  interval: string
  severity: string
  title: string
  enabled: boolean
  last_error?: string
  series: number
  pending: number
  firing: number
  source_name?: string
  last_eval_at?: string
}
type Source = { id: string; name: string; url: string; credential_id?: string; credential_name?: string; skip_verify: boolean; rules: number }
type View = { rules: Rule[]; sources: Source[]; templates: Rule[]; ops: string[] }
type Preview = { series: { ci: string; labels: Record<string, string>; value: number; match: boolean; title: string }[]; total: number; matched: number; error?: string }
type Credential = { id: string; name: string; type: string }

const SEVERITIES = ['info', 'warning', 'error', 'critical']

export function RulesPage() {
  const t = useT(strings)
  const { can } = useSession()
  const editor = can('rules:edit')
  const [tab, setTab] = useState<'rules' | 'sources'>('rules')
  const [epoch, setEpoch] = useState(0)
  const view = useResource<View>('/api/rules', epoch)
  const [rule, setRule] = useState<Rule | 'new' | null>(null)
  const [source, setSource] = useState<Source | 'new' | null>(null)
  const reload = () => setEpoch((e) => e + 1)
  useEffect(() => {
    const id = window.setInterval(() => document.visibilityState === 'visible' && setEpoch((e) => e + 1), 15_000)
    return () => window.clearInterval(id)
  }, [])
  const v = view.data
  return (
    <div className="stack">
      <div className="row rl-head">
        <Segmented
          label={t('rl.tab.rules')}
          value={tab}
          onChange={setTab}
          options={[
            { value: 'rules', label: `${t('rl.tab.rules')} (${v?.rules.length ?? 0})` },
            { value: 'sources', label: `${t('rl.tab.sources')} (${v?.sources.length ?? 0})` },
          ]}
        />
        {editor && tab === 'rules' && (
          <Button variant="primary" disabled={!v?.sources.length} onClick={() => setRule('new')}>
            <Plus size={16} aria-hidden />
            {t('rl.new')}
          </Button>
        )}
        {editor && tab === 'sources' && (
          <Button variant="primary" onClick={() => setSource('new')}>
            <Plus size={16} aria-hidden />
            {t('ms.new')}
          </Button>
        )}
      </div>
      <ErrorBanner error={view.error} strings={strings} />
      {v && tab === 'rules' && <RulesTable v={v} editor={editor} onOpen={setRule} onSources={() => setTab('sources')} />}
      {v && tab === 'sources' && <SourcesTable v={v} editor={editor} onOpen={setSource} />}
      {v && <RuleEditor value={rule} v={v} onClose={() => setRule(null)} onSaved={() => (setRule(null), reload())} />}
      <SourceEditor value={source} onClose={() => setSource(null)} onSaved={() => (setSource(null), reload())} />
    </div>
  )
}

function RuleState({ r }: { r: Rule }) {
  const t = useT(strings)
  if (!r.enabled) return <span className="pill pill-off">{t('rl.off')}</span>
  if (r.last_error)
    return (
      <span className="pill pill-error" title={r.last_error}>
        {t('rl.error')}
      </span>
    )
  if (r.firing > 0) return <span className="pill pill-error">{t('rl.firing', { n: r.firing })}</span>
  if (r.pending > 0) return <span className="pill pill-warn">{t('rl.pending', { n: r.pending })}</span>
  if (!r.last_eval_at) return <span className="pill pill-off">{t('rl.never')}</span>
  return <span className="pill pill-ok">{t('rl.ok', { n: r.series })}</span>
}

function RulesTable({ v, editor, onOpen, onSources }: { v: View; editor: boolean; onOpen: (r: Rule) => void; onSources: () => void }) {
  const t = useT(strings)
  if (v.sources.length === 0)
    return (
      <Banner kind="info" title={t('rl.empty.sources')}>
        <button type="button" className="cn-link" onClick={onSources}>
          {t('ms.new')}
        </button>
      </Banner>
    )
  if (v.rules.length === 0) return <p className="muted card rl-empty">{t('rl.empty')}</p>
  return (
    <div className="card cn-table-card">
      <table className="cn-table rl-table">
        <thead>
          <tr>
            <th>{t('rl.col.method')}</th>
            <th>{t('rl.col.name')}</th>
            <th>{t('rl.col.cond')}</th>
            <th>{t('rl.col.state')}</th>
            <th>{t('rl.col.source')}</th>
          </tr>
        </thead>
        <tbody>
          {v.rules.map((r) => (
            <tr key={r.id}>
              <td>
                <span className={`pill rl-method rl-${r.method}`} title={t(`rl.method.${r.method}.hint`)}>
                  {t(`rl.method.${r.method}`)}
                </span>
              </td>
              <td className="rl-name">
                {editor ? (
                  <button type="button" className="cn-link cn-name" onClick={() => onOpen(r)}>
                    {r.name}
                  </button>
                ) : (
                  <span className="cn-name">{r.name}</span>
                )}
                <div className="muted rl-sub">
                  <code>{r.signal}</code> · {t(`sev.${r.severity}`)}
                </div>
              </td>
              <td className="rl-cond">
                <code>
                  {r.op} {r.threshold}
                </code>
                {r.for !== '0s' && <span className="muted"> {t('rl.for', { d: r.for })}</span>}
              </td>
              <td>
                <RuleState r={r} />
                {r.last_error && <div className="rl-sub inc-warn">{r.last_error}</div>}
              </td>
              <td>{r.source_name}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function SourcesTable({ v, editor, onOpen }: { v: View; editor: boolean; onOpen: (s: Source) => void }) {
  const t = useT(strings)
  if (v.sources.length === 0) return <p className="muted card rl-empty">{t('rl.empty.sources')}</p>
  return (
    <div className="card cn-table-card">
      <table className="cn-table rl-table">
        <thead>
          <tr>
            <th>{t('ms.name')}</th>
            <th>{t('ms.url')}</th>
            <th>{t('ms.cred')}</th>
            <th className="num">{t('ms.rules')}</th>
          </tr>
        </thead>
        <tbody>
          {v.sources.map((s) => (
            <tr key={s.id}>
              <td className="rl-name">
                {editor ? (
                  <button type="button" className="cn-link cn-name" onClick={() => onOpen(s)}>
                    {s.name}
                  </button>
                ) : (
                  <span className="cn-name">{s.name}</span>
                )}
              </td>
              <td>
                <code>{s.url}</code>
              </td>
              <td>{s.credential_name ?? <span className="muted">{t('ms.cred.none')}</span>}</td>
              <td className="num">{s.rules}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const EMPTY: Omit<Rule, 'id' | 'series' | 'pending' | 'firing'> = {
  name: '',
  method: 'use',
  signal: '',
  source_id: '',
  query: '',
  ci_label: 'instance',
  op: '>',
  threshold: 0,
  for: '5m',
  interval: '30s',
  severity: 'warning',
  title: '',
  enabled: true,
}

type RuleDraft = typeof EMPTY & { threshold_text: string }

function ruleDraft(r: Partial<Rule>, sourceID: string): RuleDraft {
  const d = { ...EMPTY, ...r, source_id: r.source_id || sourceID }
  return { ...d, threshold_text: String(d.threshold) }
}

function ruleBody(d: RuleDraft) {
  return {
    name: d.name,
    method: d.method,
    signal: d.signal,
    source_id: d.source_id,
    query: d.query,
    ci_label: d.ci_label,
    op: d.op,
    threshold: Number(d.threshold_text.replace(',', '.')),
    for: d.for,
    interval: d.interval,
    severity: d.severity,
    title: d.title,
    enabled: d.enabled,
  }
}

function RuleEditor({ value, v, onClose, onSaved }: { value: Rule | 'new' | null; v: View; onClose: () => void; onSaved: () => void }) {
  const t = useT(strings)
  const { locale } = useLocale()
  const { timezone } = useSession()
  const editing = value && value !== 'new' ? value : null
  const [d, setD] = useState<RuleDraft>(() => ruleDraft(editing ?? {}, v.sources[0]?.id ?? ''))
  const [template, setTemplate] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const save = useAction()
  const check = useAction()
  useEffect(() => {
    setD(ruleDraft(editing ?? {}, v.sources[0]?.id ?? ''))
    setTemplate('')
    setPreview(null)
    save.clear()
    check.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])
  const set = (p: Partial<RuleDraft>) => setD({ ...d, ...p })
  const pick = (i: string) => {
    setTemplate(i)
    if (i === '') return
    const tpl = v.templates[Number(i)]
    setD(ruleDraft({ ...tpl, source_id: d.source_id }, d.source_id))
    setPreview(null)
  }
  const submit = () =>
    save.run(async () => {
      await api(editing ? 'PUT' : 'POST', editing ? `/api/rules/${editing.id}` : '/api/rules', ruleBody(d))
      onSaved()
    })
  const runPreview = () => check.run(async () => setPreview(await api<Preview>('POST', '/api/rules/preview', ruleBody(d))))
  const remove = () =>
    editing &&
    window.confirm(t('rl.delete.confirm', { name: editing.name })) &&
    save.run(async () => {
      await api('DELETE', `/api/rules/${editing.id}`)
      onSaved()
    })

  return (
    <Modal
      open={value !== null}
      title={editing ? `${t('rl.dialog.edit')} · ${editing.name}` : t('rl.dialog.new')}
      onClose={onClose}
      footer={
        <>
          {editing && (
            <Button variant="ghost" busy={save.busy} onClick={() => void remove()}>
              {t('rl.delete')}
            </Button>
          )}
          <Button busy={check.busy} onClick={() => void runPreview()}>
            {t('rl.preview')}
          </Button>
          <Button onClick={onClose}>{t('rl.cancel')}</Button>
          <Button variant="primary" busy={save.busy} onClick={() => void submit()}>
            {t('rl.save')}
          </Button>
        </>
      }
    >
      <div className="stack rl-editor">
        {!editing && (
          <Field label={t('rl.template')}>
            {(id) => (
              <Select id={id} value={template} onChange={(e) => pick(e.target.value)}>
                <option value="">{t('rl.template.none')}</option>
                {v.templates.map((tpl, i) => (
                  <option key={i} value={String(i)}>
                    {`${tpl.method.toUpperCase()} · ${tpl.name}`}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        )}
        <div className="rl-grid">
          <Field label={t('rl.name')}>{(id) => <Input id={id} value={d.name} onChange={(e) => set({ name: e.target.value })} />}</Field>
          <Field label={t('rl.method')}>
            {(id) => (
              <Select id={id} value={d.method} onChange={(e) => set({ method: e.target.value as Method })}>
                <option value="red">{`RED · ${t('rl.method.red.hint')}`}</option>
                <option value="use">{`USE · ${t('rl.method.use.hint')}`}</option>
              </Select>
            )}
          </Field>
        </div>
        <Field label={t('rl.source')}>
          {(id) => (
            <Select id={id} value={d.source_id} onChange={(e) => set({ source_id: e.target.value })}>
              {v.sources.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t('rl.query')} hint={t('rl.query.hint')}>
          {(id) => <Textarea id={id} rows={3} className="rl-query" value={d.query} spellCheck={false} onChange={(e) => set({ query: e.target.value })} />}
        </Field>
        <div className="rl-grid rl-grid-4">
          <Field label={t('rl.op')}>
            {(id) => (
              <Select id={id} value={d.op} onChange={(e) => set({ op: e.target.value })}>
                {v.ops.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t('rl.threshold')}>{(id) => <Input id={id} inputMode="decimal" value={d.threshold_text} onChange={(e) => set({ threshold_text: e.target.value })} />}</Field>
          <Field label={t('rl.forField')} hint={t('rl.forField.hint')}>
            {(id) => <Input id={id} value={d.for} onChange={(e) => set({ for: e.target.value })} />}
          </Field>
          <Field label={t('rl.interval')} hint={t('rl.interval.hint')}>
            {(id) => <Input id={id} value={d.interval} onChange={(e) => set({ interval: e.target.value })} />}
          </Field>
        </div>
        <div className="rl-grid">
          <Field label={t('rl.severity')}>
            {(id) => (
              <Select id={id} value={d.severity} onChange={(e) => set({ severity: e.target.value })}>
                {SEVERITIES.map((s) => (
                  <option key={s} value={s}>
                    {t(`sev.${s}`)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t('rl.cilabel')} hint={t('rl.cilabel.hint')}>
            {(id) => <Input id={id} value={d.ci_label} onChange={(e) => set({ ci_label: e.target.value })} />}
          </Field>
        </div>
        <div className="rl-grid">
          <Field label={t('rl.signal')} hint={t('rl.signal.hint')}>
            {(id) => <Input id={id} value={d.signal} onChange={(e) => set({ signal: e.target.value })} />}
          </Field>
          <Field label={t('rl.title')} hint={t('rl.title.hint')}>
            {(id) => <Input id={id} value={d.title} onChange={(e) => set({ title: e.target.value })} />}
          </Field>
        </div>
        <Switch checked={d.enabled} onChange={(enabled) => set({ enabled })} label={t('rl.enabled')} />
        {editing?.last_eval_at && <p className="hint">{t('rl.lastEval', { at: formatDate(editing.last_eval_at, locale, timezone) })}</p>}
        <ErrorBanner error={check.error ?? save.error} strings={strings} />
        {preview && <PreviewTable p={preview} />}
      </div>
    </Modal>
  )
}

function PreviewTable({ p }: { p: Preview }) {
  const t = useT(strings)
  if (p.error) return <Banner kind="error" title={p.error} />
  return (
    <div className="stack rl-preview">
      <Banner kind={p.matched > 0 ? 'warn' : 'ok'} title={t('rl.preview.result', { total: p.total, matched: p.matched })}>
        {p.series.length < p.total && t('rl.preview.more', { n: p.series.length })}
      </Banner>
      {p.series.length > 0 && (
        <table className="cn-table compact">
          <tbody>
            {p.series.map((s, i) => (
              <tr key={i} className={s.match ? 'rl-match' : undefined}>
                <td>{s.ci || '—'}</td>
                <td className="num">{s.value}</td>
                <td className="muted">{s.title}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

type SourceDraft = { name: string; url: string; credential_id: string; skip_verify: boolean }

function SourceEditor({ value, onClose, onSaved }: { value: Source | 'new' | null; onClose: () => void; onSaved: () => void }) {
  const t = useT(strings)
  const { can } = useSession()
  const editing = value && value !== 'new' ? value : null
  const blank: SourceDraft = { name: '', url: '', credential_id: '', skip_verify: false }
  const [d, setD] = useState<SourceDraft>(blank)
  const [ok, setOk] = useState('')
  const creds = useResource<Credential[]>(value && can('credentials:view') ? '/api/credentials' : '', 0)
  const save = useAction()
  const test = useAction()
  useEffect(() => {
    setD(editing ? { name: editing.name, url: editing.url, credential_id: editing.credential_id ?? '', skip_verify: editing.skip_verify } : blank)
    setOk('')
    save.clear()
    test.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])
  const usable = (creds.data ?? []).filter((c) => c.type === 'bearer' || c.type === 'basic' || c.type === 'header')
  const submit = () =>
    save.run(async () => {
      await api(editing ? 'PUT' : 'POST', editing ? `/api/metric-sources/${editing.id}` : '/api/metric-sources', d)
      onSaved()
    })
  const check = () =>
    test.run(async () => {
      setOk('')
      const r = await api<{ series: number }>('POST', '/api/metric-sources/test', d)
      setOk(t('ms.test.ok', { n: r.series }))
    })
  const remove = () =>
    editing &&
    window.confirm(t('ms.delete.confirm', { name: editing.name })) &&
    save.run(async () => {
      await api('DELETE', `/api/metric-sources/${editing.id}`)
      onSaved()
    })
  return (
    <Modal
      open={value !== null}
      title={editing ? t('ms.dialog.edit') : t('ms.dialog.new')}
      onClose={onClose}
      footer={
        <>
          {editing && (
            <Button variant="ghost" busy={save.busy} onClick={() => void remove()}>
              {t('rl.delete')}
            </Button>
          )}
          <Button busy={test.busy} onClick={() => void check()}>
            {t('ms.test')}
          </Button>
          <Button onClick={onClose}>{t('rl.cancel')}</Button>
          <Button variant="primary" busy={save.busy} onClick={() => void submit()}>
            {t('rl.save')}
          </Button>
        </>
      }
    >
      <div className="stack">
        <Field label={t('ms.name')}>{(id) => <Input id={id} value={d.name} placeholder="Prometheus" onChange={(e) => setD({ ...d, name: e.target.value })} />}</Field>
        <Field label={t('ms.url')} hint={t('ms.url.hint')}>
          {(id) => <Input id={id} value={d.url} placeholder="http://prometheus:9090" onChange={(e) => setD({ ...d, url: e.target.value })} />}
        </Field>
        <Field label={t('ms.cred')} hint={t('ms.cred.hint')}>
          {(id) => (
            <Select id={id} value={d.credential_id} onChange={(e) => setD({ ...d, credential_id: e.target.value })}>
              <option value="">{t('ms.cred.none')}</option>
              {editing?.credential_id && !usable.some((c) => c.id === editing.credential_id) && (
                <option value={editing.credential_id}>{editing.credential_name ?? editing.credential_id}</option>
              )}
              {usable.map((c) => (
                <option key={c.id} value={c.id}>
                  {`${c.name} (${c.type})`}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {d.url.startsWith('https') && <Switch checked={d.skip_verify} onChange={(skip_verify) => setD({ ...d, skip_verify })} label={t('ms.skip')} />}
        {ok && <Banner kind="ok" title={ok} />}
        <ErrorBanner error={test.error ?? save.error} strings={strings} />
      </div>
    </Modal>
  )
}
