import { FlaskConical, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtTime, methodLabel, SEVERITIES, sevLabel, type Method, type Rule, type Severity } from '../api'
import { useApp, useFetch, useLive } from '../context'
import { t } from '../i18n'
import { Empty, Field, Modal, PageHeader, SevBadge, Toggle } from '../components/ui'

interface RuleList {
  items: Rule[]
  sources: { id: string; name: string; url: string }[]
  templates: Rule[]
  ops: string[]
}

interface Preview {
  series: { ci: string; ci_id?: string; labels: Record<string, string>; value: number; match: boolean; title: string }[]
  matched: number
  error?: string
}

const blank = (source: string): Rule => ({
  id: '',
  name: '',
  method: 'use',
  signal: '',
  source_id: source,
  query: '',
  ci_label: 'host',
  op: '>',
  threshold: 0,
  for: '1m',
  interval: '30s',
  severity: 'warning',
  title: '',
  enabled: true,
})

export function RulesPage() {
  const { can, toast } = useApp()
  const { data, reload } = useFetch<RuleList>('/api/rules')
  const [edit, setEdit] = useState<Rule | null>(null)
  const [tplOpen, setTplOpen] = useState(false)
  useLive(['alert'], reload, 3000)
  const editable = can('rules.edit')
  if (!data) return <Empty>{t('common.words.loading')}</Empty>
  const source = data.sources[0]?.id ?? ''
  const sourceName = (id: string) => data.sources.find((s) => s.id === id)?.name ?? id
  const toggle = async (r: Rule, on: boolean) => {
    try {
      await api.put(`/api/rules/${r.id}`, { ...r, enabled: on, state: undefined })
      toast(on ? t('rules.toasts.enabled') : t('rules.toasts.disabled'))
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <div className="page">
      <div className="page-main">
        <PageHeader
          title={t('rules.header.title')}
          sub={t('rules.header.sub')}
          actions={
            editable &&
            data.sources.length > 0 && (
              <>
                <button className="btn" onClick={() => setTplOpen(true)}>
                  {t('rules.header.fromTemplate')}
                </button>
                <button className="btn btn-primary" onClick={() => setEdit(blank(source))}>
                  <Plus size={15} /> {t('rules.header.create')}
                </button>
              </>
            )
          }
        />
        {data.sources.length === 0 && (
          <div className="card">
            <p className="hint">{t('rules.empty.noSource')}</p>
            <Link className="btn" to="/integrations">
              {t('rules.empty.toIntegrations')}
            </Link>
          </div>
        )}
        <div className="card card-flush">
          {data.items.length === 0 ? (
            <Empty>{t('rules.empty.none')}</Empty>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('rules.table.method')}</th>
                  <th>{t('rules.table.rule')}</th>
                  <th>{t('rules.table.condition')}</th>
                  <th>{t('rules.table.severity')}</th>
                  <th>{t('rules.table.state')}</th>
                  <th>{t('rules.table.evaluated')}</th>
                  <th>{t('rules.table.enabled')}</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((r) => (
                  <tr key={r.id} className="row-click" onClick={() => setEdit(r)}>
                    <td>
                      <span className={`tag tag-${r.method}`}>{methodLabel(r.method)}</span>
                    </td>
                    <td className="cell-title">
                      <div className="title-line">{r.name}</div>
                      <div className="sub-line mono">
                        {r.id} · {r.signal} · {sourceName(r.source_id)}
                      </div>
                    </td>
                    <td>
                      <div className="query" title={r.query}>
                        {r.query}
                      </div>
                      <div className="sub-line mono">
                        {r.op} {r.threshold} · for {r.for} · every {r.interval}
                      </div>
                    </td>
                    <td>
                      <SevBadge sev={r.severity} />
                    </td>
                    <td className="nowrap">
                      {!r.enabled ? (
                        <span className="muted">{t('rules.table.off')}</span>
                      ) : r.last_error ? (
                        <span className="text-danger" title={r.last_error}>
                          {t('rules.form.lastError', { error: r.last_error.slice(0, 60) })}
                        </span>
                      ) : (
                        <>
                          {(r.firing ?? 0) > 0 && <span className="pill pill-open">{t('rules.table.firing', { n: r.firing ?? 0 })}</span>}{' '}
                          {(r.pending ?? 0) > 0 && <span className="pill pill-muted">{t('rules.table.pending', { n: r.pending ?? 0 })}</span>}{' '}
                          <span className="muted">{t('rules.table.series', { n: r.series ?? 0 })}</span>
                        </>
                      )}
                    </td>
                    <td className="nowrap">{r.last_eval_at ? fmtTime(r.last_eval_at) : <span className="muted">{t('rules.table.never')}</span>}</td>
                    <td onClick={(e) => e.stopPropagation()}>
                      <Toggle on={r.enabled} onChange={(v) => toggle(r, v)} disabled={!editable} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {tplOpen && (
        <Modal title={t('rules.header.fromTemplate')} onClose={() => setTplOpen(false)} wide>
          <div className="type-pick">
            {data.templates.map((tpl, i) => (
              <button
                key={i}
                type="button"
                className="type-pick-item"
                onClick={() => {
                  setTplOpen(false)
                  setEdit({ ...tpl, id: '', source_id: source })
                }}
              >
                <b>
                  {methodLabel(tpl.method)} · {tpl.name}
                </b>
                <span className="mono">
                  {tpl.signal} {tpl.op} {tpl.threshold}
                </span>
              </button>
            ))}
          </div>
        </Modal>
      )}
      {edit && (
        <RuleModal
          rule={edit}
          sources={data.sources}
          ops={data.ops}
          editable={editable}
          onClose={() => setEdit(null)}
          onDone={() => {
            setEdit(null)
            reload()
          }}
        />
      )}
    </div>
  )
}

function RuleModal({ rule, sources, ops, editable, onClose, onDone }: { rule: Rule; sources: RuleList['sources']; ops: string[]; editable: boolean; onClose: () => void; onDone: () => void }) {
  const { toast, meta } = useApp()
  const [r, setR] = useState<Rule>(rule)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof Rule>(k: K, v: Rule[K]) => setR((x) => ({ ...x, [k]: v }))
  const body = () => ({ ...r, state: undefined, threshold: Number(r.threshold) })
  const save = async () => {
    setError('')
    try {
      if (r.id) {
        await api.put(`/api/rules/${r.id}`, body())
        toast(t('rules.toasts.saved'))
      } else {
        await api.post('/api/rules', body())
        toast(t('rules.toasts.created'))
      }
      onDone()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const check = async () => {
    setBusy(true)
    try {
      setPreview(await api.post<Preview>('/api/rules/preview', body()))
    } catch (e) {
      setPreview({ series: [], matched: 0, error: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const evaluate = async () => {
    const out = await api.post<Rule>(`/api/rules/${r.id}/evaluate`)
    setR((x) => ({ ...x, ...out }))
    toast(t('rules.toasts.evaluated'))
  }
  const remove = async () => {
    if (!window.confirm(t('rules.confirm.delete', { name: r.name }))) return
    await api.del(`/api/rules/${r.id}`)
    toast(t('rules.toasts.deleted'))
    onDone()
  }
  const firing = Object.values(r.state ?? {}).filter((s) => s.firing)
  return (
    <Modal
      wide
      title={r.id ? t('rules.form.editTitle', { name: rule.name }) : t('rules.form.createTitle')}
      onClose={onClose}
      footer={
        <>
          {r.id && editable && (
            <div className="modal-foot-left">
              <button className="btn btn-ghost text-danger" onClick={remove}>
                <Trash2 size={14} /> {t('rules.form.delete')}
              </button>
              <button className="btn btn-ghost" onClick={evaluate}>
                <RefreshCw size={14} /> {t('rules.form.evaluate')}
              </button>
            </div>
          )}
          <button className="btn" onClick={check} disabled={busy || !r.query || !r.source_id}>
            <FlaskConical size={14} /> {t('rules.form.check')}
          </button>
          <button className="btn" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          {editable && (
            <button className="btn btn-primary" onClick={save} disabled={!r.name || !r.query || !r.source_id}>
              {r.id ? t('common.actions.save') : t('common.actions.create')}
            </button>
          )}
        </>
      }
    >
      <div className="row3">
        <Field label={t('rules.form.name')}>
          <input value={r.name} onChange={(e) => set('name', e.target.value)} autoFocus={!r.id} />
        </Field>
        <Field label={t('rules.form.method')}>
          <div className="seg">
            {(['red', 'use'] as Method[]).map((m) => (
              <button type="button" key={m} className={`seg-btn ${r.method === m ? 'seg-active' : ''}`} onClick={() => set('method', m)}>
                {methodLabel(m)}
              </button>
            ))}
          </div>
        </Field>
        <Field label={t('rules.form.signal')} help={t('rules.form.signalHelp')}>
          <input className="mono" value={r.signal} onChange={(e) => set('signal', e.target.value)} placeholder={`${r.method}.errors`} />
        </Field>
      </div>
      <Field label={t('rules.form.source')}>
        <select value={r.source_id} onChange={(e) => set('source_id', e.target.value)}>
          {sources.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name} ({s.url})
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('rules.form.query')} help={t('rules.form.queryHelp')}>
        <textarea className="mono" rows={3} value={r.query} onChange={(e) => set('query', e.target.value)} />
      </Field>
      <div className="row3">
        <Field label={t('rules.form.op')}>
          <select value={r.op} onChange={(e) => set('op', e.target.value)}>
            {ops.map((o) => (
              <option key={o}>{o}</option>
            ))}
          </select>
        </Field>
        <Field label={t('rules.form.threshold')}>
          <input type="number" step="any" value={r.threshold} onChange={(e) => set('threshold', e.target.value as unknown as number)} />
        </Field>
        <Field label={t('rules.form.severity')}>
          <select value={r.severity} onChange={(e) => set('severity', e.target.value as Severity)}>
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {sevLabel(s)}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="row3">
        <Field label={t('rules.form.for')} help={t('rules.form.forHelp')}>
          <input value={r.for} onChange={(e) => set('for', e.target.value)} />
        </Field>
        <Field label={t('rules.form.interval')}>
          <input value={r.interval} onChange={(e) => set('interval', e.target.value)} />
        </Field>
        <Field label={t('rules.form.team')}>
          <select value={r.team ?? ''} onChange={(e) => set('team', e.target.value)}>
            <option value="">—</option>
            {meta?.teams.map((tm) => (
              <option key={tm.id} value={tm.id}>
                {tm.name}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="row2">
        <Field label={t('rules.form.ciLabel')} help={t('rules.form.ciLabelHelp')}>
          <input className="mono" value={r.ci_label} onChange={(e) => set('ci_label', e.target.value)} />
        </Field>
        <Field label={t('rules.form.serviceLabel')}>
          <input className="mono" value={r.service_label ?? ''} onChange={(e) => set('service_label', e.target.value)} placeholder="service" />
        </Field>
      </div>
      <Field label={t('rules.form.title')} help={t('rules.form.titleHelp')}>
        <input value={r.title} onChange={(e) => set('title', e.target.value)} />
      </Field>
      <Field label={t('rules.form.description')}>
        <input value={r.description ?? ''} onChange={(e) => set('description', e.target.value)} />
      </Field>
      <label className="check">
        <input type="checkbox" checked={r.enabled} onChange={(e) => set('enabled', e.target.checked)} /> {t('rules.form.enabled')}
      </label>
      {r.last_error && <div className="form-error">{t('rules.form.lastError', { error: r.last_error })}</div>}
      {error && <div className="form-error">{error}</div>}
      {firing.length > 0 && (
        <div className="section">
          <h4>{t('rules.form.state')}</h4>
          <table className="table table-compact">
            <tbody>
              {firing.map((s, i) => (
                <tr key={i}>
                  <td>{s.ci}</td>
                  <td className="num">{s.value}</td>
                  <td className="muted">{t('rules.form.since', { time: fmtTime(s.fired_at ?? s.since) })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {preview && (
        <div className="section">
          {preview.error ? (
            <div className="form-error">{preview.error}</div>
          ) : preview.series.length === 0 ? (
            <p className="hint">{t('rules.form.previewEmpty')}</p>
          ) : (
            <>
              <h4>{t('rules.form.previewMatched', { n: preview.series.length, m: preview.matched })}</h4>
              <table className="table table-compact">
                <thead>
                  <tr>
                    <th>{t('rules.form.ci')}</th>
                    <th>{t('rules.form.value')}</th>
                    <th>{t('rules.form.match')}</th>
                    <th>{t('rules.form.title')}</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.series.map((s, i) => (
                    <tr key={i}>
                      <td>
                        {s.ci || '—'} <span className="muted">{s.ci_id ? t('rules.form.inCmdb') : t('rules.form.notInCmdb')}</span>
                      </td>
                      <td className="num">{Math.round(s.value * 100) / 100}</td>
                      <td>{s.match ? <span className="result-bad">✓</span> : <span className="muted">—</span>}</td>
                      <td>{s.title}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
      )}
    </Modal>
  )
}
