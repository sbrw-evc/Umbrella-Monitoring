// Connectors list page.
export const ru = {
  header: {
    title: 'Коннекторы',
    sub: 'Подключения к источникам собираются из блоков в конструкторе: получение, парсинг, шаблон, подтверждение',
    create: 'Создать коннектор',
  },
  side: {
    title: 'Состояние',
    all: 'Все',
    running: 'Запущены',
    stopped: 'Остановлены',
    draft: 'Есть неопубликованные изменения',
  },
  columns: {
    id: 'ID',
    name: 'Название',
    status: 'Состояние',
    version: 'Версия',
    events: 'Событий',
    errors: 'Ошибок разбора',
    lastEvent: 'Последнее событие',
    team: 'Команда',
    updated: 'Изменён',
  },
  status: {
    running: 'Запущен',
    stopped: 'Остановлен',
  },
  table: {
    empty: 'Коннекторов нет',
    unpublished: 'Есть неопубликованные изменения',
  },
  actions: {
    stop: 'Остановить',
    start: 'Запустить',
    copyIngest: 'Копировать адрес приёма',
    delete: 'Удалить',
  },
  toasts: {
    ingestCopied: 'Адрес приёма скопирован',
  },
  confirm: {
    delete: 'Удалить коннектор «{name}»?',
  },
  create: {
    title: 'Новый коннектор',
    submit: 'Создать и открыть конструктор',
    name: 'Название',
    namePlaceholder: 'Например: Webhook сетевого мониторинга',
    team: 'Команда-владелец',
    template: 'Заготовка',
    pushTitle: 'Push: входящий webhook',
    pushSub: 'Источник сам присылает события в Umbrella',
    pullTitle: 'Pull: опрос API по расписанию',
    pullSub: 'Umbrella сама забирает события по HTTP',
  },
}

export const en: typeof ru = {
  header: {
    title: 'Connectors',
    sub: 'Source connections are built from blocks in the builder: fetch, parse, template, acknowledge',
    create: 'New connector',
  },
  side: {
    title: 'State',
    all: 'All',
    running: 'Running',
    stopped: 'Stopped',
    draft: 'Unpublished changes',
  },
  columns: {
    id: 'ID',
    name: 'Name',
    status: 'State',
    version: 'Version',
    events: 'Events',
    errors: 'Parse errors',
    lastEvent: 'Last event',
    team: 'Team',
    updated: 'Updated',
  },
  status: {
    running: 'Running',
    stopped: 'Stopped',
  },
  table: {
    empty: 'No connectors',
    unpublished: 'Unpublished changes',
  },
  actions: {
    stop: 'Stop',
    start: 'Start',
    copyIngest: 'Copy ingest URL',
    delete: 'Delete',
  },
  toasts: {
    ingestCopied: 'Ingest URL copied',
  },
  confirm: {
    delete: 'Delete connector “{name}”?',
  },
  create: {
    title: 'New connector',
    submit: 'Create and open builder',
    name: 'Name',
    namePlaceholder: 'For example: Network monitoring webhook',
    team: 'Owner team',
    template: 'Template',
    pushTitle: 'Push: incoming webhook',
    pushSub: 'The source sends events to Umbrella',
    pullTitle: 'Pull: poll an API on a schedule',
    pullSub: 'Umbrella fetches events over HTTP',
  },
}
