// Small pages; each top-level key (events, parseErrors, rules, maintenance,
// selfcheck, audit) becomes its own section.
export const ru = {
  events: {
    header: {
      title: 'События',
      sub: 'Нормализованные события всех коннекторов, новые сверху',
      resume: 'Продолжить',
      pause: 'Пауза',
    },
    filters: {
      search: 'Заголовок, КЕ, сигнал, источник',
    },
    table: {
      time: 'Время',
      severity: 'Severity',
      status: 'Статус',
      event: 'Событие',
      ci: 'КЕ',
      signal: 'Сигнал',
      source: 'Источник',
      incident: 'Инцидент',
    },
    row: {
      resolved: 'норма',
      active: 'активно',
      suppressed: 'подавлено',
      noCi: 'без КЕ',
    },
  },
  parseErrors: {
    header: {
      title: 'Ошибки разбора',
      sub: 'События, которые коннектор не смог разобрать (events.dlq), с исходным телом',
    },
    empty: {
      none: 'Ошибок нет',
    },
    table: {
      time: 'Время',
      connector: 'Коннектор',
      block: 'Блок',
      error: 'Ошибка',
      raw: 'Исходные данные',
    },
  },
  rules: {
    header: {
      title: 'Правила RED/USE',
      sub: 'RED формирует тревоги по ИТ-сервисам, USE — по ресурсам. В этом шаге правила только просматриваются; редактор правил — следующий шаг',
    },
    table: {
      method: 'Метод',
      signal: 'Сигнал',
      rule: 'Правило',
      condition: 'Условие по умолчанию',
      appliesTo: 'Применяется к',
      severity: 'Severity',
      enabled: 'Включено',
    },
  },
  maintenance: {
    header: {
      title: 'Окна обслуживания',
      sub: 'Во время окна тревоги по КЕ и её сервису не отправляются в PagerDuty; после окончания активные тревоги уходят дежурным',
      create: 'Создать окно',
    },
    empty: {
      none: 'Окон нет',
    },
    table: {
      id: 'ID',
      window: 'Окно',
      ci: 'КЕ',
      state: 'Состояние',
      start: 'Начало',
      end: 'Конец',
      author: 'Автор',
    },
    modal: {
      title: 'Новое окно обслуживания',
      name: 'Название',
      namePlaceholder: 'Обновление ПО',
      ci: 'КЕ',
      ciHelp: 'Для ИТ-сервиса окно подавляет тревоги всех его КЕ',
      ciPlaceholder: 'Выберите КЕ',
      start: 'Начало',
      end: 'Конец',
    },
    toasts: {
      created: 'Окно обслуживания создано',
    },
    confirm: {
      delete: 'Удалить окно обслуживания?',
    },
  },
  selfcheck: {
    header: {
      title: 'Самоконтроль',
      sub: 'Состояние конвейера: приём, обработка, передача в PagerDuty',
    },
    tiles: {
      uptime: 'Работает',
      uptimeValue: '{m} мин',
      uptimeSub: 'хранилище: {store}, шина: {bus}',
      connectors: 'Коннекторы',
      connectorsSub: 'запущено / всего',
      events: 'События',
      eventsSub: 'последнее: {time}',
      parseErrors: 'Ошибки разбора',
      alerts: 'Тревоги',
      alertsSub: 'активные / всего',
    },
    pd: {
      mode: 'Режим',
      modeLive: 'отправка в PagerDuty',
      modeDry: 'ключ интеграции не задан: события не уходят наружу',
      breaker: 'Circuit breaker',
      breakerOpen: 'открыт',
      breakerClosed: 'закрыт',
      breakerSub: 'ошибок подряд: {n}',
      sent: 'Отправлено',
      sentSub: 'ошибок: {failed}, в очереди: {queue}',
      lastSuccess: 'Последний успех',
      lastError: 'ошибка: {error}',
    },
    outage: {
      title: 'Проверка резервного оповещения.',
      text: 'Симуляция недоступности PagerDuty: отправки начнут падать, откроется circuit breaker, а тревоги error и critical, не принятые за 2 минуты, получат отметку «Резерв».',
      restore: 'Вернуть PagerDuty',
      simulate: 'Симулировать недоступность',
    },
    toasts: {
      outageOn: 'Симуляция: PagerDuty недоступен',
      outageOff: 'PagerDuty снова принимает события',
    },
  },
  audit: {
    header: {
      title: 'Журнал аудита',
      sub: 'Все изменения конфигурации и действия с инцидентами',
    },
    empty: {
      none: 'Записей пока нет',
    },
    table: {
      time: 'Время',
      actor: 'Кто',
      action: 'Действие',
      object: 'Объект',
    },
  },
}

export const en: typeof ru = {
  events: {
    header: {
      title: 'Events',
      sub: 'Normalized events from all connectors, newest first',
      resume: 'Resume',
      pause: 'Pause',
    },
    filters: {
      search: 'Title, CI, signal, source',
    },
    table: {
      time: 'Time',
      severity: 'Severity',
      status: 'Status',
      event: 'Event',
      ci: 'CI',
      signal: 'Signal',
      source: 'Source',
      incident: 'Incident',
    },
    row: {
      resolved: 'normal',
      active: 'active',
      suppressed: 'suppressed',
      noCi: 'no CI',
    },
  },
  parseErrors: {
    header: {
      title: 'Parse errors',
      sub: 'Events the connector failed to parse (events.dlq), with the raw body',
    },
    empty: {
      none: 'No errors',
    },
    table: {
      time: 'Time',
      connector: 'Connector',
      block: 'Block',
      error: 'Error',
      raw: 'Raw data',
    },
  },
  rules: {
    header: {
      title: 'RED/USE rules',
      sub: 'RED raises alerts for IT services, USE for resources. Rules are read-only for now; the rule editor comes next',
    },
    table: {
      method: 'Method',
      signal: 'Signal',
      rule: 'Rule',
      condition: 'Default condition',
      appliesTo: 'Applies to',
      severity: 'Severity',
      enabled: 'Enabled',
    },
  },
  maintenance: {
    header: {
      title: 'Maintenance windows',
      sub: 'During a window, alerts for the CI and its service are not sent to PagerDuty; once it ends, active alerts go to on-call',
      create: 'Create window',
    },
    empty: {
      none: 'No windows',
    },
    table: {
      id: 'ID',
      window: 'Window',
      ci: 'CI',
      state: 'State',
      start: 'Start',
      end: 'End',
      author: 'Author',
    },
    modal: {
      title: 'New maintenance window',
      name: 'Name',
      namePlaceholder: 'Software update',
      ci: 'CI',
      ciHelp: 'For an IT service, the window suppresses alerts for all its CIs',
      ciPlaceholder: 'Select a CI',
      start: 'Start',
      end: 'End',
    },
    toasts: {
      created: 'Maintenance window created',
    },
    confirm: {
      delete: 'Delete the maintenance window?',
    },
  },
  selfcheck: {
    header: {
      title: 'Self-check',
      sub: 'Pipeline health: intake, processing, delivery to PagerDuty',
    },
    tiles: {
      uptime: 'Uptime',
      uptimeValue: '{m} min',
      uptimeSub: 'store: {store}, bus: {bus}',
      connectors: 'Connectors',
      connectorsSub: 'running / total',
      events: 'Events',
      eventsSub: 'last: {time}',
      parseErrors: 'Parse errors',
      alerts: 'Alerts',
      alertsSub: 'active / total',
    },
    pd: {
      mode: 'Mode',
      modeLive: 'sending to PagerDuty',
      modeDry: 'no integration key: events are not sent out',
      breaker: 'Circuit breaker',
      breakerOpen: 'open',
      breakerClosed: 'closed',
      breakerSub: 'consecutive failures: {n}',
      sent: 'Sent',
      sentSub: 'failed: {failed}, queued: {queue}',
      lastSuccess: 'Last success',
      lastError: 'error: {error}',
    },
    outage: {
      title: 'Fallback notification test.',
      text: 'Simulates a PagerDuty outage: sends start failing, the circuit breaker opens, and error and critical alerts not accepted within 2 minutes are marked “Fallback”.',
      restore: 'Restore PagerDuty',
      simulate: 'Simulate outage',
    },
    toasts: {
      outageOn: 'Simulation: PagerDuty unavailable',
      outageOff: 'PagerDuty is accepting events again',
    },
  },
  audit: {
    header: {
      title: 'Audit log',
      sub: 'All configuration changes and incident actions',
    },
    empty: {
      none: 'No entries yet',
    },
    table: {
      time: 'Time',
      actor: 'Who',
      action: 'Action',
      object: 'Object',
    },
  },
}
