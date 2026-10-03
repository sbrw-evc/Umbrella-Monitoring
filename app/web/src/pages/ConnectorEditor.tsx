import {
  addEdge,
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { ArrowLeft, CheckCircle2, FlaskConical, Play, Save, Square, Trash2, Upload } from 'lucide-react'
import { type DragEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type BlockSpec, type Connector, type DryRunResult, type Graph, type StepTrace } from '../api'
import { useApp } from '../context'
import { Empty, Field, SevBadge, Tabs } from '../components/ui'
import { t } from '../i18n'

// Block palette texts come from the server in Russian; translate them on the
// client by block kind and fall back to the server text when a key is missing.
function tr(key: string, fallback: string): string {
  const v = t(key)
  return v === key ? fallback : v
}
const blockKey = (kind: string) => `blocks.${kind.replace(/\./g, '_')}`
function blockTitle(spec: BlockSpec): string {
  return tr(`${blockKey(spec.kind)}.title`, spec.title)
}
function blockDescription(spec: BlockSpec): string {
  return tr(`${blockKey(spec.kind)}.description`, spec.description)
}
function fieldLabel(spec: BlockSpec, f: BlockSpec['fields'][number]): string {
  return tr(`${blockKey(spec.kind)}.fields.${f.key}.label`, f.label)
}
function fieldHelp(spec: BlockSpec, f: BlockSpec['fields'][number]): string | undefined {
  return f.help ? tr(`${blockKey(spec.kind)}.fields.${f.key}.help`, f.help) : undefined
}
function fieldPlaceholder(spec: BlockSpec, f: BlockSpec['fields'][number]): string | undefined {
  return f.placeholder ? tr(`${blockKey(spec.kind)}.fields.${f.key}.placeholder`, f.placeholder) : undefined
}
function categoryTitle(c: { id: string; title: string }): string {
  return tr(`blocks.categories.${c.id}`, c.title)
}

// Block category colors come from the theme (--blocks-<category>).
const CAT_COLOR: Record<string, string> = {
  trigger: 'var(--blocks-trigger)',
  fetch: 'var(--blocks-fetch)',
  parse: 'var(--blocks-parse)',
  transform: 'var(--blocks-transform)',
  ack: 'var(--blocks-ack)',
  output: 'var(--blocks-output)',
}

type BlockData = { spec?: BlockSpec; config: Record<string, string>; trace?: StepTrace }

function summary(spec: BlockSpec | undefined, config: Record<string, string>): string {
  if (!spec) return ''
  const main = spec.fields.find((f) => config[f.key])
  if (!main) return blockDescription(spec)
  const v = config[main.key]
  return `${fieldLabel(spec, main)}: ${v.length > 40 ? v.slice(0, 40) + '…' : v}`
}

function BlockNode({ data, selected }: NodeProps<Node<BlockData>>) {
  const spec = data.spec
  const color = CAT_COLOR[spec?.category ?? ''] ?? 'var(--app-border)'
  const isTrigger = spec?.category === 'trigger'
  return (
    <div className={`block ${selected ? 'block-selected' : ''} ${data.trace?.error ? 'block-error' : ''}`} style={{ borderTopColor: color }}>
      {!isTrigger && <Handle type="target" position={Position.Left} />}
      <div className="block-cat" style={{ color }}>
        {spec?.category ?? '?'}
      </div>
      <div className="block-title">{spec ? blockTitle(spec) : t('editor.block.unknown')}</div>
      <div className="block-sum">{summary(spec, data.config)}</div>
      {data.trace && (
        <div className={`block-trace ${data.trace.error ? 'block-trace-err' : ''}`} title={data.trace.error}>
          {data.trace.in} → {data.trace.out}
          {data.trace.error && ` · ${t('editor.block.error')}`}
        </div>
      )}
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

const nodeTypes = { block: BlockNode }

function toFlow(g: Graph, specs: Map<string, BlockSpec>): { nodes: Node<BlockData>[]; edges: Edge[] } {
  return {
    nodes: g.nodes.map((n) => ({ id: n.id, type: 'block', position: { x: n.x, y: n.y }, data: { spec: specs.get(n.kind), config: { ...n.config } } })),
    edges: g.edges.map((e) => ({ id: e.id, source: e.source, target: e.target, markerEnd: { type: MarkerType.ArrowClosed } })),
  }
}

function fromFlow(nodes: Node<BlockData>[], edges: Edge[]): Graph {
  return {
    nodes: nodes.map((n) => ({ id: n.id, kind: n.data.spec?.kind ?? 'unknown', x: Math.round(n.position.x), y: Math.round(n.position.y), config: n.data.config })),
    edges: edges.map((e) => ({ id: e.id, source: e.source, target: e.target })),
  }
}

export function ConnectorEditorPage() {
  return (
    <ReactFlowProvider>
      <Editor />
    </ReactFlowProvider>
  )
}

type BottomTab = 'dry' | 'help'

function Editor() {
  const { id = '' } = useParams()
  const nav = useNavigate()
  const { toast, can } = useApp()
  const canEdit = can('connectors.edit')
  const rf = useReactFlow()
  const [connector, setConnector] = useState<Connector | null>(null)
  const [blocks, setBlocks] = useState<BlockSpec[]>([])
  const [cats, setCats] = useState<{ id: string; title: string }[]>([])
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<BlockData>>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)
  const [sample, setSample] = useState('')
  const [result, setResult] = useState<DryRunResult | null>(null)
  const [bottom, setBottom] = useState<BottomTab>('dry')
  const [name, setName] = useState('')

  const specs = useMemo(() => new Map(blocks.map((b) => [b.kind, b])), [blocks])

  useEffect(() => {
    Promise.all([api.get<{ items: BlockSpec[]; categories: { id: string; title: string }[] }>('/api/blocks'), api.get<{ connector: Connector }>(`/api/connectors/${id}`)])
      .then(([b, c]) => {
        setBlocks(b.items)
        setCats(b.categories)
        const sp = new Map(b.items.map((x) => [x.kind, x]))
        const f = toFlow(c.connector.draft, sp)
        setNodes(f.nodes)
        setEdges(f.edges)
        setConnector(c.connector)
        setName(c.connector.name)
        setSample(c.connector.sample_input ?? '')
      })
      .catch((e: Error) => toast(e.message, 'error'))
  }, [id, setNodes, setEdges, toast])

  const markDirty = () => setDirty(true)

  const onConnect = useCallback(
    (c: Connection) => {
      setEdges((es) => addEdge({ ...c, id: `e${Date.now()}`, markerEnd: { type: MarkerType.ArrowClosed } }, es))
      markDirty()
    },
    [setEdges],
  )

  const addBlock = (kind: string, pos?: { x: number; y: number }) => {
    const spec = specs.get(kind)
    if (!spec) return
    const config: Record<string, string> = {}
    for (const f of spec.fields) if (f.default) config[f.key] = f.default
    const position = pos ?? { x: 80 + Math.random() * 200, y: 280 + Math.random() * 120 }
    const nid = `n${Date.now().toString(36)}`
    setNodes((ns) => [...ns, { id: nid, type: 'block', position, data: { spec, config } }])
    setSelected(nid)
    markDirty()
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    const kind = e.dataTransfer.getData('application/umbrella-block')
    if (kind) addBlock(kind, rf.screenToFlowPosition({ x: e.clientX, y: e.clientY }))
  }

  const updateConfig = (key: string, value: string) => {
    setNodes((ns) => ns.map((n) => (n.id === selected ? { ...n, data: { ...n.data, config: { ...n.data.config, [key]: value } } } : n)))
    markDirty()
  }

  const removeSelected = () => {
    if (!selected) return
    setNodes((ns) => ns.filter((n) => n.id !== selected))
    setEdges((es) => es.filter((e) => e.source !== selected && e.target !== selected))
    setSelected(null)
    markDirty()
  }

  const save = async (): Promise<boolean> => {
    try {
      const c = await api.put<Connector>(`/api/connectors/${id}`, { name, draft: fromFlow(nodes, edges), sample_input: sample })
      setConnector(c)
      setDirty(false)
      return true
    } catch (e) {
      toast((e as Error).message, 'error')
      return false
    }
  }

  const dryRun = async () => {
    try {
      const r = await api.post<DryRunResult>(`/api/connectors/${id}/dry-run`, { graph: fromFlow(nodes, edges), sample })
      setResult(r)
      setBottom('dry')
      const byNode = new Map(r.trace.map((t) => [t.node_id, t]))
      setNodes((ns) => ns.map((n) => ({ ...n, data: { ...n.data, trace: byNode.get(n.id) } })))
      if (r.error) toast(r.error, 'error')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const publish = async () => {
    if (!(await save())) return
    try {
      const c = await api.post<Connector>(`/api/connectors/${id}/publish`)
      setConnector(c)
      toast(t('editor.toasts.published', { version: c.version }))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const setStatus = async (action: 'start' | 'stop') => {
    try {
      await api.post(`/api/connectors/${id}/${action}`)
      setConnector((c) => (c ? { ...c, status: action === 'start' ? 'running' : 'stopped' } : c))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const sel = nodes.find((n) => n.id === selected)
  const unpublished = dirty || connector?.draft_dirty

  return (
    <div className="editor">
      <div className="editor-bar">
        <button className="icon-btn" onClick={() => nav('/connectors')} title={t('editor.toolbar.back')}>
          <ArrowLeft size={18} />
        </button>
        <input
          className="editor-name"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            markDirty()
          }}
        />
        <span className="tag">{connector?.version ? t('editor.toolbar.published', { version: connector.version }) : t('editor.toolbar.notPublished')}</span>
        {unpublished && (
          <span className="unsaved">
            <span className="dirty-dot" /> {dirty ? t('editor.toolbar.unsaved') : t('editor.toolbar.draftDiffers')}
          </span>
        )}
        <div className="filterbar-spacer" />
        {canEdit && (
          <>
            <button className="btn" onClick={() => save().then((ok) => ok && toast(t('editor.toasts.saved')))}>
              <Save size={14} /> {t('editor.toolbar.save')}
            </button>
            <button className="btn" onClick={dryRun}>
              <FlaskConical size={14} /> {t('editor.toolbar.dryRun')}
            </button>
            <button className="btn btn-primary" onClick={publish}>
              <Upload size={14} /> {t('editor.toolbar.publish')}
            </button>
            {connector?.status === 'running' ? (
              <button className="btn" onClick={() => setStatus('stop')}>
                <Square size={14} /> {t('editor.toolbar.stop')}
              </button>
            ) : (
              <button className="btn" onClick={() => setStatus('start')}>
                <Play size={14} /> {t('editor.toolbar.start')}
              </button>
            )}
          </>
        )}
      </div>

      <div className="editor-body">
        <aside className="palette">
          <div className="palette-title">{t('editor.palette.title')}</div>
          {cats.map((c) => (
            <div key={c.id} className="palette-group">
              <div className="palette-cat" style={{ color: CAT_COLOR[c.id] }}>
                {categoryTitle(c)}
              </div>
              {blocks
                .filter((b) => b.category === c.id)
                .map((b) => (
                  <div
                    key={b.kind}
                    className="palette-item"
                    draggable
                    onDragStart={(e) => e.dataTransfer.setData('application/umbrella-block', b.kind)}
                    onDoubleClick={() => addBlock(b.kind)}
                    title={`${blockDescription(b)}\n${t('editor.palette.dragHint')}`}
                    style={{ borderLeftColor: CAT_COLOR[c.id] }}
                  >
                    {blockTitle(b)}
                  </div>
                ))}
            </div>
          ))}
        </aside>

        <div className="canvas" onDrop={onDrop} onDragOver={(e) => e.preventDefault()}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={(ch) => {
              onNodesChange(ch)
              if (ch.some((c) => c.type === 'position' || c.type === 'remove')) markDirty()
            }}
            onEdgesChange={(ch) => {
              onEdgesChange(ch)
              if (ch.some((c) => c.type === 'remove')) markDirty()
            }}
            onConnect={onConnect}
            onNodeClick={(_, n) => setSelected(n.id)}
            onPaneClick={() => setSelected(null)}
            fitView
            fitViewOptions={{ padding: 0.15, maxZoom: 1 }}
            proOptions={{ hideAttribution: true }}
            deleteKeyCode={['Delete']}
          >
            <Background gap={16} color="var(--graph-grid)" />
            <Controls />
          </ReactFlow>
        </div>

        <aside className="inspector">
          {sel ? (
            <>
              <div className="inspector-head">
                <div>
                  <div className="block-cat" style={{ color: CAT_COLOR[sel.data.spec?.category ?? ''] }}>
                    {sel.data.spec?.category}
                  </div>
                  <h3>{sel.data.spec && blockTitle(sel.data.spec)}</h3>
                </div>
                <button className="icon-btn" onClick={removeSelected} title={t('editor.inspector.deleteBlock')}>
                  <Trash2 size={16} />
                </button>
              </div>
              <p className="hint">{sel.data.spec && blockDescription(sel.data.spec)}</p>
              {sel.data.spec?.fields.map((f) => (
                <Field key={f.key} label={fieldLabel(sel.data.spec!, f)} help={fieldHelp(sel.data.spec!, f)}>
                  {f.type === 'select' ? (
                    <select value={sel.data.config[f.key] ?? f.default ?? ''} onChange={(e) => updateConfig(f.key, e.target.value)}>
                      {f.options?.map((o) => (
                        <option key={o}>{o}</option>
                      ))}
                    </select>
                  ) : f.type === 'textarea' ? (
                    <textarea rows={4} className="mono" value={sel.data.config[f.key] ?? ''} placeholder={fieldPlaceholder(sel.data.spec!, f)} onChange={(e) => updateConfig(f.key, e.target.value)} />
                  ) : f.type === 'secret' ? (
                    <SecretField
                      connector={id}
                      name={`${sel.id}_${f.key}`}
                      value={sel.data.config[f.key] ?? ''}
                      placeholder={fieldPlaceholder(sel.data.spec!, f)}
                      onChange={(v) => updateConfig(f.key, v)}
                    />
                  ) : (
                    <input value={sel.data.config[f.key] ?? ''} placeholder={fieldPlaceholder(sel.data.spec!, f)} onChange={(e) => updateConfig(f.key, e.target.value)} />
                  )}
                </Field>
              ))}
              {sel.data.trace && (
                <div className="trace-box">
                  <div className="section-title">{t('editor.inspector.lastCheck')}</div>
                  <div>
                    {t('editor.inspector.trace', { in: sel.data.trace.in, out: sel.data.trace.out, ms: sel.data.trace.ms })}
                  </div>
                  {sel.data.trace.error && <div className="text-danger">{sel.data.trace.error}</div>}
                  {sel.data.trace.sample !== undefined && <pre className="json">{JSON.stringify(sel.data.trace.sample, null, 2)}</pre>}
                </div>
              )}
            </>
          ) : (
            <ConnectorInfo connector={connector} />
          )}
        </aside>
      </div>

      <div className="editor-bottom">
        <Tabs<BottomTab>
          value={bottom}
          onChange={setBottom}
          tabs={[
            { id: 'dry', title: t('editor.tabs.dry') },
            { id: 'help', title: t('editor.tabs.help') },
          ]}
        />
        {bottom === 'dry' ? (
          <div className="dry">
            <div className="dry-input">
              <div className="section-title">{t('editor.dry.sampleTitle')}</div>
              <textarea
                className="mono"
                value={sample}
                onChange={(e) => {
                  setSample(e.target.value)
                  markDirty()
                }}
                placeholder={t('editor.dry.samplePlaceholder')}
              />
            </div>
            <div className="dry-result">
              {!result ? (
                <Empty>{t('editor.dry.empty')}</Empty>
              ) : result.error ? (
                <div className="text-danger">{result.error}</div>
              ) : (
                <>
                  <div className="section-title">
                    <CheckCircle2 size={14} /> {t('editor.dry.summary', { events: result.events.length, errors: result.errors.length })}
                  </div>
                  {result.errors.map((e, i) => (
                    <div key={i} className="text-danger">
                      {(() => { const sp = specs.get(e.kind); return sp ? blockTitle(sp) : e.kind })()}: {e.error}
                    </div>
                  ))}
                  <table className="table table-compact">
                    <thead>
                      <tr>
                        <th>{t('editor.columns.severity')}</th>
                        <th>{t('editor.columns.title')}</th>
                        <th>{t('editor.columns.ciFromEvent')}</th>
                        <th>{t('editor.columns.ciInMap')}</th>
                        <th>{t('editor.columns.signal')}</th>
                        <th>{t('editor.columns.status')}</th>
                        <th>{t('editor.columns.id')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {result.events.map((ev, i) => (
                        <tr key={i}>
                          <td>
                            <SevBadge sev={ev.severity} />
                          </td>
                          <td>{ev.title}</td>
                          <td className="mono">{ev.ci || '—'}</td>
                          <td>{result.cis[i]?.found ? result.cis[i].ci_name : <span className="tag tag-warn">{t('editor.dry.notFound')}</span>}</td>
                          <td className="mono">
                            {ev.signal} <span className="tag">{ev.method}</span>
                          </td>
                          <td>{ev.status}</td>
                          <td className="mono">{ev.external_id || '—'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </>
              )}
            </div>
          </div>
        ) : (
          <div className="help">
            <p>{t('editor.help.chain')}</p>
            <p>
              {t('editor.help.tplIntro')} <code>${'{'}{t('editor.help.pathWord')}{'}'}</code> {t('editor.help.tplPath')} <code>${'{'}labels.instance{'}'}</code>), <code>${'{'}{t('editor.help.fieldWord')}|{t('editor.help.valueWord')}{'}'}</code> {t('editor.help.tplDefaultText')} <code>${'{'}{t('editor.help.fieldWord')}|${t('editor.help.otherWord')}{'}'}</code> {t('editor.help.tplOther')}
            </p>
            <p>{t('editor.help.secrets')}</p>
          </div>
        )}
      </div>
    </div>
  )
}

function ConnectorInfo({ connector }: { connector: Connector | null }) {
  if (!connector) return <Empty>{t('common.words.loading')}</Empty>
  const url = `${location.origin}/api/ingest/${connector.id}`
  return (
    <div>
      <h3>{connector.name}</h3>
      <p className="hint">{t('editor.info.selectHint')}</p>
      <div className="props">
        <div className="prop">
          <div className="prop-k">{t('editor.info.status')}</div>
          <div className="prop-v">{connector.status === 'running' ? t('editor.info.running') : t('editor.info.stopped')}</div>
        </div>
        <div className="prop">
          <div className="prop-k">{t('editor.info.events')}</div>
          <div className="prop-v">{connector.events_total}</div>
        </div>
        <div className="prop">
          <div className="prop-k">{t('editor.info.errors')}</div>
          <div className="prop-v">{connector.errors_total}</div>
        </div>
      </div>
      <div className="section-title">{t('editor.info.ingestUrl')}</div>
      <pre className="json">{url}</pre>
      <div className="section-title">{t('editor.info.example')}</div>
      <pre className="json">{`curl -X POST ${url} \\
  -H "Authorization: Bearer <${t('editor.info.token')}>" \\
  -d @event.json`}</pre>
      <p className="hint">{t('editor.info.retryHint')}</p>
    </div>
  )
}

function SecretField({ connector, name, value, placeholder, onChange }: { connector: string; name: string; value: string; placeholder?: string; onChange: (v: string) => void }) {
  const { toast, meta } = useApp()
  const [secret, setSecret] = useState('')
  const store = async () => {
    try {
      const r = await api.post<{ ref: string }>(`/api/connectors/${connector}/secret`, { key: name.toLowerCase().replace(/[^a-z0-9_]/g, '_'), value: secret })
      onChange(r.ref)
      setSecret('')
      toast(t('editor.inspector.secretStored'))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <>
      <input className="mono" value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
      {meta?.openbao && (
        <div className="copy-box">
          <input type="password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={t('editor.inspector.secretValue')} autoComplete="new-password" />
          <button type="button" className="btn btn-sm" disabled={!secret} onClick={store}>
            {t('editor.inspector.secretSave')}
          </button>
        </div>
      )}
    </>
  )
}
