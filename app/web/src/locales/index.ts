// Built-in dictionaries, one file per page or UI area. Each file exports the
// same tree in Russian (ru) and English (en).
import type { Dict } from '../i18n'
import * as access from './access'
import * as blocks from './blocks'
import * as cmdb from './cmdb'
import * as common from './common'
import * as connectors from './connectors'
import * as editor from './editor'
import * as heatmap from './heatmap'
import * as incidents from './incidents'
import * as menu from './menu'
import * as misc from './misc'
import * as ops from './ops'
import * as rules from './rules'
import * as integrations from './integrations'
import * as cis from './cis'
import * as setup from './setup'
import * as teams from './teams'
import * as settings from './settings'

type Pair = { ru: Dict; en: Dict }

// misc.ts and access.ts hold several small pages (events, parseErrors, rules, ...), each
// becomes its own top-level section.
const miscSections: Record<string, Pair> = {}
for (const m of [misc, access]) {
  for (const k of Object.keys(m.ru)) {
    miscSections[k] = { ru: (m.ru as Dict)[k] as Dict, en: (m.en as Dict)[k] as Dict }
  }
}

export const sections: Record<string, Pair> = {
  common,
  menu,
  incidents,
  heatmap,
  ops,
  cmdb,
  connectors,
  editor,
  blocks,
  ...miscSections,
  rules,
  integrations,
  cis,
  setup,
  teams,
  settings,
}
