// Heatmap page: CIs against time and the tile map of the current state.
export const ru = {
  header: {
    title: 'Тепловая карта',
    sub: 'Где и когда были активны инциденты: строки — КЕ, столбцы — время, цвет — худшая важность, насыщенность — число инцидентов',
  },
  controls: {
    period: 'Период',
    groupBy: 'Группировать',
    problemsOnly: 'Только КЕ с инцидентами',
  },
  periods: {
    h6: '6 ч',
    h24: '24 ч',
    h72: '3 дня',
    h168: '7 дней',
  },
  groupBy: {
    service: 'по ИТ-сервису',
    team: 'по команде',
    type: 'по типу КЕ',
  },
  group: {
    business: 'Бизнес-услуги',
    none: {
      service: 'Вне ИТ-сервисов',
      team: 'Без команды',
      type: 'Тип не указан',
    },
  },
  tabs: {
    time: 'Во времени',
    state: 'Состояние сейчас',
  },
  matrix: {
    ci: 'КЕ',
    total: 'Всего',
    allCis: 'Все КЕ',
    openCi: 'Открыть карточку КЕ',
    noCi: 'нет в CMDB',
    groupStat: 'КЕ: {rows}, инцидентов: {n}',
  },
  cell: {
    active: { one: '{n} активный инцидент', few: '{n} активных инцидента', many: '{n} активных инцидентов', other: '{n} активного инцидента' },
    worst: 'худшая важность',
    none: 'инцидентов не было',
  },
  tiles: {
    count: { one: '{n} КЕ', few: '{n} КЕ', many: '{n} КЕ', other: '{n} КЕ' },
    open: { one: '{n} открытый', few: '{n} открытых', many: '{n} открытых', other: '{n} открытого' },
    ok: 'норма',
    below: 'проблема в зависимостях',
    period: { one: '{n} за период', few: '{n} за период', many: '{n} за период', other: '{n} за период' },
  },
  legend: {
    none: 'нет инцидентов',
    intensity: 'чем насыщеннее цвет, тем больше инцидентов в ячейке (до {max})',
  },
  empty: {
    problems: 'За выбранный период инцидентов нет. Снимите флажок «Только КЕ с инцидентами», чтобы увидеть все КЕ.',
    all: 'В области видимости нет КЕ.',
  },
}

export const en: typeof ru = {
  header: {
    title: 'Heatmap',
    sub: 'Where and when incidents were active: rows are CIs, columns are time, color is the worst severity, intensity is the number of incidents',
  },
  controls: {
    period: 'Period',
    groupBy: 'Group',
    problemsOnly: 'Only CIs with incidents',
  },
  periods: {
    h6: '6 h',
    h24: '24 h',
    h72: '3 days',
    h168: '7 days',
  },
  groupBy: {
    service: 'by IT service',
    team: 'by team',
    type: 'by CI type',
  },
  group: {
    business: 'Business services',
    none: {
      service: 'Outside IT services',
      team: 'No team',
      type: 'No type',
    },
  },
  tabs: {
    time: 'Over time',
    state: 'Current state',
  },
  matrix: {
    ci: 'CI',
    total: 'Total',
    allCis: 'All CIs',
    openCi: 'Open CI card',
    noCi: 'not in CMDB',
    groupStat: 'CIs: {rows}, incidents: {n}',
  },
  cell: {
    active: { one: '{n} active incident', few: '{n} active incidents', many: '{n} active incidents', other: '{n} active incidents' },
    worst: 'worst severity',
    none: 'no incidents',
  },
  tiles: {
    count: { one: '{n} CI', few: '{n} CIs', many: '{n} CIs', other: '{n} CIs' },
    open: { one: '{n} open', few: '{n} open', many: '{n} open', other: '{n} open' },
    ok: 'healthy',
    below: 'issue in dependencies',
    period: { one: '{n} in period', few: '{n} in period', many: '{n} in period', other: '{n} in period' },
  },
  legend: {
    none: 'no incidents',
    intensity: 'the stronger the color, the more incidents in the cell (up to {max})',
  },
  empty: {
    problems: 'No incidents in the selected period. Clear "Only CIs with incidents" to see all CIs.',
    all: 'No CIs in scope.',
  },
}
