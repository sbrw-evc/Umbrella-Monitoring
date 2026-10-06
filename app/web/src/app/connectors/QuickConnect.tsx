import { useState } from 'react'
import { Check, Copy, Download, Send } from 'lucide-react'
import { api } from '../../api'
import { ErrorBanner } from '../../connections/ConnectionCard'
import { mergeDicts } from '../../connections/connectionStrings'
import { useAction } from '../../connections/useRequest'
import { useT } from '../../i18n'
import { Link, useRouter } from '../../router'
import { Banner, Button, Field, Input, Modal, Segmented } from '../../ui'
import { useSession } from '../session'
import { strings as connectorStrings } from './strings'
import { sourcesStrings } from './sourcesStrings'
import type { Connector } from './types'
import './sources.css'

const s = mergeDicts(connectorStrings, sourcesStrings)

export type QuickPreset = 'zabbix' | 'alertmanager' | 'grafana' | 'webhook'
const PRESETS: QuickPreset[] = ['zabbix', 'alertmanager', 'grafana', 'webhook']

export type QuickResult = {
  connector: Connector
  credential_id: string
  ingest_url: string
  token: string
  instructions: { preset: string; ingest_url: string; auth_header: string; snippet?: string; snippet_kind?: string }
  mediatype_yaml?: string
  monitoring_id?: string
}

// canQuickConnect: quick connect makes a credential and publishes a connector.
export function useCanQuickConnect() {
  const { can } = useSession()
  return can('connectors:edit') && can('connectors:publish') && can('credentials:edit')
}

// SourcesExplainer is the one line that says which page does what for a monitoring system.
export function SourcesExplainer() {
  const t = useT(s)
  return (
    <p className="muted src-explain">
      {t('src.explain')} <Link to="/connectors">{t('src.explain.connectors')}</Link> · <Link to="/monitoring">{t('src.explain.monitoring')}</Link> ·{' '}
      <Link to="/rules">{t('src.explain.rules')}</Link>
    </p>
  )
}

export function CopyField({ value, label, mono = true }: { value: string; label: string; mono?: boolean }) {
  const t = useT(s)
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard?.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <Field label={label}>
      {(id) => (
        <div className="row src-copy">
          <Input id={id} readOnly value={value} className={mono ? 'cn-mono' : ''} onFocus={(e) => e.currentTarget.select()} />
          <Button onClick={copy} title={t('src.copy')}>
            {copied ? <Check size={15} aria-hidden /> : <Copy size={15} aria-hidden />}
            {copied ? t('src.copied') : t('src.copy')}
          </Button>
        </div>
      )}
    </Field>
  )
}

function Snippet({ text, file }: { text: string; file?: string }) {
  const t = useT(s)
  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: 'text/yaml' }))
    const a = document.createElement('a')
    a.href = url
    a.download = file ?? 'umbrella.txt'
    a.click()
    URL.revokeObjectURL(url)
  }
  return (
    <div className="src-snippet">
      <pre className="cn-mono">{text}</pre>
      <div className="row">
        <Button onClick={() => void navigator.clipboard?.writeText(text)}>
          <Copy size={15} aria-hidden />
          {t('src.copy')}
        </Button>
        {file && (
          <Button onClick={download}>
            <Download size={15} aria-hidden />
            {t('src.download')}
          </Button>
        )}
      </div>
    </div>
  )
}

const STEPS: Record<string, number> = { zabbix: 3, alertmanager: 2, grafana: 2, webhook: 1 }

// QuickResultView shows what the source needs, once: address, token, steps and the ready
// configuration, then the test event.
export function QuickResultView({ r }: { r: QuickResult }) {
  const t = useT(s)
  const p = r.instructions.preset
  return (
    <div className="stack">
      <Banner kind="ok" title={t('src.connect.done', { name: r.connector.name })} />
      <CopyField label={t('src.url')} value={r.ingest_url} />
      <CopyField label={t('src.token')} value={r.token} />
      <p className="hint">{t('src.token.once')}</p>
      <h3 className="src-h">{t('src.steps')}</h3>
      <ol className="src-steps">
        {Array.from({ length: STEPS[p] ?? 0 }, (_, i) => (
          <li key={i}>{t(`src.steps.${p}.${i + 1}`)}</li>
        ))}
      </ol>
      {r.mediatype_yaml && <Snippet text={r.mediatype_yaml} file="umbrella-mediatype.yaml" />}
      {r.instructions.snippet && <Snippet text={r.instructions.snippet} file={r.instructions.snippet_kind === 'yaml' ? 'alertmanager-umbrella.yml' : undefined} />}
      <TestEvent connectorID={r.connector.id} />
    </div>
  )
}

// QuickConnectDialog: «Подключить источник». With a monitoring system the connector becomes the
// system's alert intake and the template is chosen by the system kind.
export function QuickConnectDialog({
  open,
  onClose,
  onDone,
  monitoring,
}: {
  open: boolean
  onClose: () => void
  onDone?: (r: QuickResult) => void
  monitoring?: { id: string; name: string; kind: 'zabbix' | 'prometheus' }
}) {
  const t = useT(s)
  const { navigate } = useRouter()
  const [preset, setPreset] = useState<QuickPreset>(monitoring?.kind === 'prometheus' ? 'alertmanager' : 'zabbix')
  const [name, setName] = useState('')
  const [result, setResult] = useState<QuickResult | null>(null)
  const action = useAction()
  const choices = monitoring ? (monitoring.kind === 'prometheus' ? (['alertmanager', 'grafana'] as QuickPreset[]) : (['zabbix'] as QuickPreset[])) : PRESETS
  const close = () => {
    setResult(null)
    setName('')
    action.clear()
    onClose()
  }
  const submit = async () => {
    const chosen = choices.includes(preset) ? preset : choices[0]
    const r = await action.run(() => api<QuickResult>('POST', '/api/connectors/quick', { preset: chosen, name: name.trim(), monitoring_id: monitoring?.id ?? '' }))
    if (r) {
      setResult(r)
      onDone?.(r)
    }
  }
  return (
    <Modal
      open={open}
      title={monitoring ? `${t('src.connect.title')}: ${monitoring.name}` : t('src.connect.title')}
      onClose={close}
      footer={
        result ? (
          <>
            <Button onClick={() => (close(), navigate(`/connectors/${encodeURIComponent(result.connector.id)}`))}>{t('src.open')}</Button>
            <Button variant="primary" onClick={close}>
              {t('src.close')}
            </Button>
          </>
        ) : (
          <>
            <Button variant="ghost" onClick={close}>
              {t('src.connect.cancel')}
            </Button>
            <Button variant="primary" busy={action.busy} onClick={() => void submit()}>
              {t('src.connect.go')}
            </Button>
          </>
        )
      }
    >
      {result ? (
        <QuickResultView r={result} />
      ) : (
        <>
          <p className="muted">{t('src.connect.hint')}</p>
          {choices.length > 1 && (
            <Segmented label={t('src.connect.preset')} value={preset} onChange={setPreset} options={choices.map((p) => ({ value: p, label: t(`src.preset.${p}`) }))} />
          )}
          <p className="hint">{t(`src.preset.${choices.includes(preset) ? preset : choices[0]}.hint`)}</p>
          <Field label={t('src.connect.name')} hint={t('src.connect.name.hint')}>
            {(id) => <Input id={id} value={name} placeholder={monitoring?.name ?? t(`src.preset.${preset}`)} onChange={(e) => setName(e.target.value)} />}
          </Field>
          <ErrorBanner error={action.error} strings={s} />
        </>
      )}
    </Modal>
  )
}

type TestResult = { incident_id: string; request_id: string; status: string; error?: string }
type Ref = { id: string; name: string }
type Incident = {
  id: string
  ci_id?: string
  ci_name: string
  route: { services: Ref[]; team?: Ref; people: { name: string }[]; owners: { name: string }[]; via: string }
}

// TestEvent sends a test event through a published connector and shows the incident and its
// route.
export function TestEvent({ connectorID }: { connectorID: string }) {
  const t = useT(s)
  const [ci, setCI] = useState('')
  const [res, setRes] = useState<TestResult | null>(null)
  const [inc, setInc] = useState<Incident | null>(null)
  const action = useAction()
  const send = async () => {
    setRes(null)
    setInc(null)
    const r = await action.run(async () => {
      const r = await api<TestResult>('POST', `/api/connectors/${encodeURIComponent(connectorID)}/test-event`, { ci: ci.trim() })
      if (r.incident_id) {
        try {
          const v = await api<{ alert: Incident }>('GET', `/api/incidents/${encodeURIComponent(r.incident_id)}`)
          setInc(v.alert)
        } catch {
          // The incident page may be closed to this user; the ID is still shown.
        }
      }
      return r
    })
    if (r) setRes(r)
  }
  const people = inc ? (inc.route.people.length > 0 ? inc.route.people : inc.route.owners) : []
  return (
    <div className="card src-test stack">
      <p className="muted">{t('src.test.hint')}</p>
      <Field label={t('src.test.ci')} hint={t('src.test.ci.hint')}>
        {(id) => <Input id={id} value={ci} placeholder="umbrella-test" onChange={(e) => setCI(e.target.value)} />}
      </Field>
      <div className="row">
        <Button busy={action.busy} onClick={() => void send()}>
          <Send size={15} aria-hidden />
          {t('src.test')}
        </Button>
      </div>
      <ErrorBanner error={action.error} strings={s} />
      {res && res.status === 'failed' && <Banner kind="error" title={t('src.test.failed')}>{res.error}</Banner>}
      {res && res.status !== 'failed' && !res.incident_id && <Banner kind="info" title={t('src.test.queued')} />}
      {res && res.incident_id && (
        <Banner kind="ok" title={t('src.test.sent', { id: res.incident_id })}>
          {inc && (
            <ul className="src-route">
              <li>{inc.ci_id ? t('src.test.route.ci', { ci: inc.ci_name }) : t('src.test.route.unknown', { ci: inc.ci_name })}</li>
              {inc.route.services.length > 0 && <li>{t('src.test.route.services', { services: inc.route.services.map((x) => x.name).join(', ') })}</li>}
              {inc.route.team && <li>{t('src.test.route.team', { team: inc.route.team.name })}</li>}
              <li>{people.length > 0 ? t('src.test.route.people', { people: people.map((p) => p.name).join(', ') }) : t('src.test.route.nobody')}</li>
            </ul>
          )}
          <Link to={`/incidents?id=${encodeURIComponent(res.incident_id)}`}>{t('src.test.open')}</Link>
        </Banner>
      )}
    </div>
  )
}
