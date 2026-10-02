import { Background, Controls, Handle, MarkerType, Position, ReactFlow, type Edge, type Node, type NodeProps } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Box, Building2, Cloud, Database, Layers, Router, Server } from 'lucide-react'
import { useMemo } from 'react'
import { CI_TYPE_LABEL, type CI, type Relation } from '../api'

export function CiIcon({ type, size = 16 }: { type: string; size?: number }) {
  switch (type) {
    case 'business_service':
      return <Building2 size={size} />
    case 'it_service':
      return <Layers size={size} />
    case 'database':
      return <Database size={size} />
    case 'cloud_group':
      return <Cloud size={size} />
    case 'deployment':
      return <Box size={size} />
    case 'network':
      return <Router size={size} />
    default:
      return <Server size={size} />
  }
}

const LEVEL: Record<string, number> = { business_service: 0, it_service: 1, network: 3 }

type CiNodeData = { ci: CI; selected: boolean }

function CiNode({ data }: NodeProps<Node<CiNodeData>>) {
  const { ci, selected } = data
  return (
    <div className={`ci-node ci-node-${ci.status || 'ok'} ${selected ? 'ci-node-selected' : ''}`}>
      <Handle type="target" position={Position.Top} />
      <div className="ci-node-icon">
        <CiIcon type={ci.type} />
      </div>
      <div className="ci-node-text">
        <div className="ci-node-name">{ci.name}</div>
        <div className="ci-node-type">
          {CI_TYPE_LABEL[ci.type] ?? ci.type}
          {ci.open_alerts > 0 && <span className="ci-node-count">{ci.open_alerts}</span>}
        </div>
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

const nodeTypes = { ci: CiNode }

// CmdbGraph draws the resource-service model top-down: business services,
// IT services, resources, network. Node color is the worst open alert on
// the CI or below it.
export function CmdbGraph({
  nodes,
  edges,
  selected,
  onSelect,
}: {
  nodes: CI[]
  edges: Relation[]
  selected?: string
  onSelect?: (id: string) => void
}) {
  const flow = useMemo(() => {
    // Level by type, then push a CI below every CI it depends on is above.
    const level = new Map<string, number>()
    for (const n of nodes) level.set(n.id, LEVEL[n.type] ?? 2)
    for (let i = 0; i < 4; i++) {
      for (const e of edges) {
        const a = level.get(e.from)
        const b = level.get(e.to)
        if (a !== undefined && b !== undefined && b <= a) level.set(e.to, a + 1)
      }
    }
    const rows = new Map<number, CI[]>()
    for (const n of nodes) {
      const l = level.get(n.id) ?? 2
      rows.set(l, [...(rows.get(l) ?? []), n])
    }
    const W = 185
    const maxRow = Math.max(1, ...[...rows.values()].map((r) => r.length))
    const fn: Node<CiNodeData>[] = []
    for (const [l, row] of rows) {
      row.sort((a, b) => a.name.localeCompare(b.name))
      const offset = ((maxRow - row.length) * W) / 2
      row.forEach((ci, i) => {
        fn.push({ id: ci.id, type: 'ci', position: { x: offset + i * W, y: l * 120 }, data: { ci, selected: ci.id === selected } })
      })
    }
    const fe: Edge[] = edges.map((e, i) => ({
      id: `r${i}`,
      source: e.from,
      target: e.to,
      data: { kind: e.type },
      markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16, color: '#9aa5b1' },
      style: { stroke: '#9aa5b1' },
    }))
    return { fn, fe }
  }, [nodes, edges, selected])

  return (
    <div className="graph">
      <ReactFlow
        nodes={flow.fn}
        edges={flow.fe}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.08, maxZoom: 1.1 }}
        nodesDraggable={false}
        nodesConnectable={false}
        onNodeClick={(_, n) => onSelect?.(n.id)}
        proOptions={{ hideAttribution: true }}
        minZoom={0.2}
      >
        <Background gap={20} color="#e3e7eb" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}
