// Built-in dictionaries, one file per page or UI area. Each file exports the
// same tree in Russian (ru) and English (en).
import type { Dict } from '../i18n'
import * as blocks from './blocks'
import * as cmdb from './cmdb'
import * as common from './common'
import * as connectors from './connectors'
import * as editor from './editor'
import * as incidents from './incidents'
import * as menu from './menu'
import * as misc from './misc'
import * as ops from './ops'
import * as settings from './settings'

type Pair = { ru: Dict; en: Dict }

// misc.ts holds several small pages (events, parseErrors, rules, ...), each
// becomes its own top-level section.
const miscSections: Record<string, Pair> = {}
for (const k of Object.keys(misc.ru)) {
  miscSections[k] = { ru: (misc.ru as Dict)[k] as Dict, en: (misc.en as Dict)[k] as Dict }
}

export const sections: Record<string, Pair> = {
  common,
  menu,
  incidents,
  ops,
  cmdb,
  connectors,
  editor,
  blocks,
  ...miscSections,
  settings,
}
