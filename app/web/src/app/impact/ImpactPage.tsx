import { useCallback, useEffect, useState } from 'react'
import { ArrowDown, ArrowUp, Pencil, Plus, RotateCcw, Trash2 } from 'lucide-react'
import { api } from '../../api'
import { useT } from '../../i18n'
import { Banner, Button, Field, Input, Modal, Select, Stepper, Switch, Textarea } from '../../ui'
import { ProfileCard } from '../profile/ProfileCard'
import { useAction } from '../profile/useAction'
import { useSession } from '../session'
import { ImpactLine, ImpactSteps } from '../incidents/IncidentImpact'
import { severityTone } from '../incidents/format'
import { METHODS, SEVERITIES, severityText, type Impact } from '../incidents/types'
import { LEVELS, levelText, ruleSummary, type Policy, type Rule, type RuleAction } from './explain'
import { strings } from './strings'
import '../incidents/incidents.css'
import './impact.css'

// View is app.ImpactView.
type View = { policy: Policy; defaults: Policy; levels: string[]; variables: string[]; actions: RuleAction[] }

// Preview is alert.ImpactPreview.
type Preview = { event_severity: string; severity: string; impact: Impact; ci?: { id: string; name: string }; team?: { id: string; name: string } }

const ACTIONS: RuleAction[] = ['set', 'raise', 'lower', 'impact']
const RULE_ID = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/
const MAX_DEPTH = 10

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)

export function ImpactPage() {
  const t = useT(strings)
  const { can } = useSession()
  const editor = can('impact:edit')
  const [view, setView] = useState<View | null>(null)
  const [draft, setDraft] = useState<Policy | null>(null)
  const loader = useAction(strings)
  const saver = useAction(strings)

  const apply = useCallback((v: View) => {
    setView(v)
    setDraft(v.policy)
  }, [])
  const { run: load } = loader
  useEffect(() => {
    void load(async () => apply(await api<View>('GET', '/api/impact')))
  }, [load, apply])

  if (!view || !draft) {
    return loader.error ? (
      <Banner kind="error" title={loader.error.message}>
        {loader.error.detail}
      </Banner>
    ) : (
      <p className="muted">{t('loading')}</p>
    )
  }

  const dirty = !same(draft, view.policy)
  const save = () =>
    saver.run(async () => {
      apply(await api<View>('PUT', '/api/impact', draft))
      return t('im.saved')
    })
  const reset = () =>
    window.confirm(t('im.reset.confirm')) &&
    saver.run(async () => {
      apply(await api<View>('POST', '/api/impact/reset-defaults'))
      return t('im.reset.done')
    })

  return (
    <div className="stack im-page">
      <ProfileCard
        title={t('im.policy')}
        action={saver}
        onSubmit={save}
        footer={
          editor && (
            <div className="row">
              <Button type="button" variant="ghost" onClick={reset} disabled={saver.busy}>
                <RotateCcw size={16} aria-hidden />
                {t('im.reset')}
              </Button>
              <Button type="button" variant="ghost" onClick={() => setDraft(view.policy)} disabled={!dirty || saver.busy}>
                {t('im.revert')}
              </Button>
              <Button type="submit" variant="primary" busy={saver.busy} disabled={!dirty}>
                {t('im.save')}
              </Button>
            </div>
          )
        }
      >
        <p className="muted">{t('im.policy.text')}</p>
        {!view.policy.configured && <Banner kind="info" title={t('im.template')} />}
        <fieldset className="plain-fieldset stack" disabled={!editor}>
          <Switch checked={draft.enabled} onChange={(enabled) => setDraft({ ...draft, enabled })} label={t('im.enabled')} hint={t('im.enabled.hint')} />
          <Field label={t('im.depth')} hint={t('im.depth.hint')}>
            {(id) => (
              <Stepper id={id} value={draft.upstream_depth} min={0} max={MAX_DEPTH} onChange={(upstream_depth) => setDraft({ ...draft, upstream_depth })} />
            )}
          </Field>
        </fieldset>
      </ProfileCard>

      <Matrix policy={draft} defaults={view.defaults} editor={editor} onChange={(matrix) => setDraft({ ...draft, matrix })} />
      <Rules rules={draft.rules} variables={view.variables} editor={editor} onChange={(rules) => setDraft({ ...draft, rules })} />
      <Test policy={draft} />
    </div>
  )
}

function Matrix({
  policy,
  defaults,
  editor,
  onChange,
}: {
  policy: Policy
  defaults: Policy
  editor: boolean
  onChange: (m: Policy['matrix']) => void
}) {
  const t = useT(strings)
  const cell = (ev: string, lvl: string) => policy.matrix[ev]?.[lvl] ?? defaults.matrix[ev]?.[lvl] ?? ev
  const set = (ev: string, lvl: string, sev: string) => onChange({ ...policy.matrix, [ev]: { ...policy.matrix[ev], [lvl]: sev } })
  return (
    <ProfileCard title={t('im.matrix')}>
      <p className="muted">{t('im.matrix.text')}</p>
      <div className="im-matrix-wrap">
        <table className="im-matrix">
          <thead>
            <tr>
              <th scope="col">{t('im.matrix.event')}</th>
              {LEVELS.map((l) => (
                <th key={l} scope="col">
                  {levelText(t, l)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {SEVERITIES.map((ev) => (
              <tr key={ev}>
                <th scope="row">
                  <span className={`pill inc-sev inc-sev-${severityTone(ev)}`}>
                    <span className="inc-dot" aria-hidden />
                    {severityText(t, ev)}
                  </span>
                </th>
                {LEVELS.map((lvl) => {
                  const v = cell(ev, lvl)
                  const def = defaults.matrix[ev]?.[lvl]
                  const label = `${severityText(t, ev)} × ${levelText(t, lvl)}`
                  const title = [v !== ev ? t('im.matrix.changed') : '', def && def !== v ? t('im.matrix.template', { sev: severityText(t, def) }) : '']
                    .filter(Boolean)
                    .join(' · ')
                  return (
                    <td key={lvl} className={v !== ev ? 'im-moved' : undefined} title={title || undefined}>
                      <Select
                        aria-label={label}
                        value={v}
                        disabled={!editor}
                        className={`im-cell inc-sev inc-sev-${severityTone(v)}`}
                        onChange={(e) => set(ev, lvl, e.target.value)}
                      >
                        {SEVERITIES.map((s) => (
                          <option key={s} value={s}>
                            {severityText(t, s)}
                          </option>
                        ))}
                      </Select>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </ProfileCard>
  )
}

function Rules({ rules, variables, editor, onChange }: { rules: Rule[]; variables: string[]; editor: boolean; onChange: (r: Rule[]) => void }) {
  const t = useT(strings)
  const [editing, setEditing] = useState<{ index: number; rule: Rule } | null>(null)
  const move = (i: number, d: number) => {
    const next = [...rules]
    const [r] = next.splice(i, 1)
    next.splice(i + d, 0, r)
    onChange(next)
  }
  const add = () => {
    let n = rules.length + 1
    while (rules.some((r) => r.id === `rule-${n}`)) n++
    setEditing({ index: -1, rule: { id: `rule-${n}`, name: '', enabled: true, when: '', action: 'raise', n: 1, stop: false } })
  }
  const commit = (r: Rule) => {
    const next = [...rules]
    if (editing && editing.index >= 0) next[editing.index] = r
    else next.push(r)
    onChange(next)
    setEditing(null)
  }
  return (
    <ProfileCard
      title={t('im.rules')}
      footer={
        editor && (
          <Button type="button" onClick={add}>
            <Plus size={16} aria-hidden />
            {t('im.rule.add')}
          </Button>
        )
      }
    >
      <p className="muted">{t('im.rules.text')}</p>
      {rules.length === 0 ? (
        <p className="muted">{t('im.rules.empty')}</p>
      ) : (
        <ol className="im-rules">
          {rules.map((r, i) => (
            <li key={r.id} className={r.enabled ? undefined : 'im-rule-off'}>
              <div className="im-rule-main">
                <div className="im-rule-head">
                  <b>{r.name}</b>
                  <span className="muted">{ruleSummary(t, r)}</span>
                  {r.stop && <span className="pill">{t('im.rule.stopped')}</span>}
                  {!r.enabled && <span className="pill pill-off">{t('im.rule.off')}</span>}
                </div>
                <code className="im-when">{r.when}</code>
              </div>
              {editor && (
                <div className="row im-rule-tools">
                  <Switch checked={r.enabled} onChange={(enabled) => onChange(rules.map((x, j) => (j === i ? { ...x, enabled } : x)))} label={t('im.rule.enabled')} />
                  <button type="button" className="icon-btn" disabled={i === 0} onClick={() => move(i, -1)} aria-label={t('im.rule.up')} title={t('im.rule.up')}>
                    <ArrowUp size={16} />
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    disabled={i === rules.length - 1}
                    onClick={() => move(i, 1)}
                    aria-label={t('im.rule.down')}
                    title={t('im.rule.down')}
                  >
                    <ArrowDown size={16} />
                  </button>
                  <button type="button" className="icon-btn" onClick={() => setEditing({ index: i, rule: r })} aria-label={t('im.rule.edit')} title={t('im.rule.edit')}>
                    <Pencil size={16} />
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => onChange(rules.filter((_, j) => j !== i))}
                    aria-label={t('im.rule.delete')}
                    title={t('im.rule.delete')}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              )}
            </li>
          ))}
        </ol>
      )}
      {editing && (
        <RuleEditor
          rule={editing.rule}
          taken={rules.filter((_, j) => j !== editing.index).map((r) => r.id)}
          variables={variables}
          onCancel={() => setEditing(null)}
          onDone={commit}
        />
      )}
    </ProfileCard>
  )
}

function RuleEditor({
  rule,
  taken,
  variables,
  onCancel,
  onDone,
}: {
  rule: Rule
  taken: string[]
  variables: string[]
  onCancel: () => void
  onDone: (r: Rule) => void
}) {
  const t = useT(strings)
  const [r, setR] = useState<Rule>(rule)
  const [tried, setTried] = useState(false)
  const errors = [
    !r.name.trim() && t('im.rule.error.name'),
    (!RULE_ID.test(r.id) || r.id.length > 64 || taken.includes(r.id)) && t('im.rule.error.id'),
    !r.when.trim() && t('im.rule.error.when'),
  ].filter(Boolean) as string[]
  const done = () => {
    setTried(true)
    if (errors.length) return
    const out: Rule = { id: r.id, name: r.name.trim(), enabled: r.enabled, when: r.when.trim(), action: r.action, stop: r.stop }
    if (r.action === 'set') out.severity = r.severity || 'error'
    if (r.action === 'raise' || r.action === 'lower') out.n = r.n || 1
    if (r.action === 'impact') out.level = r.level || 'critical'
    onDone(out)
  }
  return (
    <Modal
      open
      title={rule.name ? t('im.rule.edit') : t('im.rule.new')}
      onClose={onCancel}
      footer={
        <div className="row">
          <Button type="button" variant="ghost" onClick={onCancel}>
            {t('im.rule.cancel')}
          </Button>
          <Button type="button" variant="primary" onClick={done}>
            {t('im.rule.done')}
          </Button>
        </div>
      }
    >
      <div className="stack">
        <div className="im-form-row">
          <Field label={t('im.rule.name')}>{(id) => <Input id={id} value={r.name} maxLength={200} onChange={(e) => setR({ ...r, name: e.target.value })} />}</Field>
          <Field label={t('im.rule.id')} hint={t('im.rule.id.hint')}>
            {(id) => <Input id={id} value={r.id} maxLength={64} onChange={(e) => setR({ ...r, id: e.target.value.trim() })} />}
          </Field>
        </div>
        <Field label={t('im.rule.when')} hint={t('im.rule.when.hint')}>
          {(id) => (
            <div className="im-mono">
              <Textarea id={id} rows={3} maxLength={2000} spellCheck={false} value={r.when} onChange={(e) => setR({ ...r, when: e.target.value })} />
            </div>
          )}
        </Field>
        <details className="im-details">
          <summary>{t('im.vars')}</summary>
          <dl className="im-vars">
            {variables.map((v) => (
              <div key={v}>
                <dt>
                  <code>{v}</code>
                </dt>
                <dd>{t(`im.var.${v}`)}</dd>
              </div>
            ))}
          </dl>
          <p className="muted">
            <code>{t('im.vars.example')}</code>
          </p>
        </details>
        <div className="im-form-row">
          <Field label={t('im.rule.action')}>
            {(id) => (
              <Select id={id} value={r.action} onChange={(e) => setR({ ...r, action: e.target.value as RuleAction })}>
                {ACTIONS.map((a) => (
                  <option key={a} value={a}>
                    {t(`im.action.${a}`)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {r.action === 'set' && (
            <Field label={t('im.rule.severity')}>
              {(id) => (
                <Select id={id} value={r.severity || 'error'} onChange={(e) => setR({ ...r, severity: e.target.value })}>
                  {SEVERITIES.map((s) => (
                    <option key={s} value={s}>
                      {severityText(t, s)}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          )}
          {(r.action === 'raise' || r.action === 'lower') && (
            <Field label={t('im.rule.n')}>{(id) => <Stepper id={id} value={r.n || 1} min={1} max={4} onChange={(n) => setR({ ...r, n })} />}</Field>
          )}
          {r.action === 'impact' && (
            <Field label={t('im.rule.level')}>
              {(id) => (
                <Select id={id} value={r.level || 'critical'} onChange={(e) => setR({ ...r, level: e.target.value })}>
                  {LEVELS.map((l) => (
                    <option key={l} value={l}>
                      {levelText(t, l)}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          )}
        </div>
        <Switch checked={!!r.stop} onChange={(stop) => setR({ ...r, stop })} label={t('im.rule.stop')} hint={t('im.rule.stop.hint')} />
        <Switch checked={r.enabled} onChange={(enabled) => setR({ ...r, enabled })} label={t('im.rule.enabled')} />
        {tried && errors.length > 0 && <Banner kind="error" title={errors.join(' ')} />}
      </div>
    </Modal>
  )
}

function parseLabels(text: string) {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const i = line.indexOf('=')
    if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return out
}

function Test({ policy }: { policy: Policy }) {
  const t = useT(strings)
  const action = useAction(strings)
  const [ev, setEv] = useState({ title: '', ci: '', signal: '', severity: 'warning', method: 'other', labels: '' })
  const [result, setResult] = useState<Preview | null>(null)
  const run = () =>
    action.run(async () => {
      const event = { title: ev.title, ci: ev.ci, signal: ev.signal, severity: ev.severity, method: ev.method, labels: parseLabels(ev.labels) }
      setResult(await api<Preview>('POST', '/api/impact/preview', { event, policy }))
    })
  return (
    <ProfileCard
      title={t('im.test')}
      action={action}
      onSubmit={run}
      footer={
        <Button type="submit" variant="primary" busy={action.busy}>
          {t('im.test.run')}
        </Button>
      }
    >
      <p className="muted">{t('im.test.text')}</p>
      <div className="im-form-row">
        <Field label={t('im.test.title')}>{(id) => <Input id={id} value={ev.title} onChange={(e) => setEv({ ...ev, title: e.target.value })} />}</Field>
        <Field label={t('im.test.ci')} hint={t('im.test.ci.hint')}>
          {(id) => <Input id={id} value={ev.ci} onChange={(e) => setEv({ ...ev, ci: e.target.value })} />}
        </Field>
      </div>
      <div className="im-form-row">
        <Field label={t('im.test.signal')}>{(id) => <Input id={id} value={ev.signal} onChange={(e) => setEv({ ...ev, signal: e.target.value })} />}</Field>
        <Field label={t('im.test.severity')}>
          {(id) => (
            <Select id={id} value={ev.severity} onChange={(e) => setEv({ ...ev, severity: e.target.value })}>
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>
                  {severityText(t, s)}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t('im.test.method')}>
          {(id) => (
            <Select id={id} value={ev.method} onChange={(e) => setEv({ ...ev, method: e.target.value })}>
              {METHODS.map((m) => (
                <option key={m} value={m}>
                  {m.toUpperCase()}
                </option>
              ))}
            </Select>
          )}
        </Field>
      </div>
      <Field label={t('im.test.labels')} hint={t('im.test.labels.hint')}>
        {(id) => (
          <div className="im-mono">
            <Textarea id={id} rows={2} spellCheck={false} value={ev.labels} onChange={(e) => setEv({ ...ev, labels: e.target.value })} />
          </div>
        )}
      </Field>
      {result && (
        <div className="im-result" aria-live="polite">
          <ImpactLine severity={result.severity} event={result.event_severity} impact={result.impact} />
          <p className="muted">
            {result.ci ? t('im.test.ci.found', { name: result.ci.name }) : t('im.test.ci.missing')}
            {result.team && <> · {t('im.test.team', { name: result.team.name })}</>}
          </p>
          <ImpactSteps impact={result.impact} />
        </div>
      )}
    </ProfileCard>
  )
}
