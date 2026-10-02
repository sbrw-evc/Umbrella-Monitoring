// Block palette texts, keyed by block kind with dots replaced by "_".
// Russian must match the backend (internal/pipeline/pipeline.go) exactly.
export const ru = {
  categories: {
    trigger: 'Триггеры',
    fetch: 'Получение',
    parse: 'Парсинг',
    transform: 'Преобразование',
    ack: 'Подтверждение',
    output: 'Выход',
  },
  trigger_webhook: {
    title: 'Входящий webhook',
    description: 'Источник отправляет события на /api/ingest/{id}. Ответ 2xx уходит только после записи события.',
    fields: {
      auth: { label: 'Проверка' },
      secret_ref: { label: 'Ссылка на секрет', help: 'Сам токен хранится только в хранилище секретов' },
    },
  },
  trigger_schedule: {
    title: 'Расписание',
    description: 'Запускает получение данных с заданным интервалом (pull).',
    fields: {
      interval: { label: 'Интервал' },
    },
  },
  fetch_http: {
    title: 'HTTP-запрос',
    description: 'GET или POST к REST API источника.',
    fields: {
      url: { label: 'URL' },
      method: { label: 'Метод' },
      body: { label: 'Тело запроса' },
      auth_header: { label: 'Заголовок авторизации' },
      secret_ref: { label: 'Ссылка на секрет' },
    },
  },
  parse_json: {
    title: 'Парсинг JSON',
    description: 'Разбирает JSON. Путь к массиву разбивает пачку на отдельные события.',
    fields: {
      items: { label: 'Путь к массиву событий', help: 'Пусто: тело целиком — одно событие или массив' },
    },
  },
  parse_kv: {
    title: 'Парсинг key=value',
    description: 'Строки вида host=db1 sev=5 msg="disk full". Каждая строка — событие.',
    fields: {
      pair_sep: { label: 'Разделитель пар' },
      kv_sep: { label: 'Разделитель ключа' },
    },
  },
  parse_regex: {
    title: 'Парсинг regex',
    description: 'Именованные группы (?P<name>...) становятся полями. Каждая строка — событие.',
    fields: {
      pattern: { label: 'Выражение' },
    },
  },
  parse_csv: {
    title: 'Парсинг CSV',
    description: 'Первая строка — заголовок, остальные — события.',
    fields: {
      delimiter: { label: 'Разделитель' },
    },
  },
  filter: {
    title: 'Фильтр',
    description: 'Пропускает дальше только события, подходящие под условие.',
    fields: {
      field: { label: 'Поле' },
      op: { label: 'Условие' },
      value: { label: 'Значение' },
    },
  },
  map_severity: {
    title: 'Справочник severity',
    description: 'Переводит значение поля источника в critical, error, warning или info.',
    fields: {
      field: { label: 'Поле источника' },
      mapping: { label: 'Соответствие', help: 'по строке на значение; * — всё остальное' },
    },
  },
  enrich_labels: {
    title: 'Метки',
    description: 'Добавляет постоянные или вычисленные метки.',
    fields: {
      labels: { label: 'Метки' },
    },
  },
  map_event: {
    title: 'Шаблон источника',
    description: 'Сопоставляет поля источника с единой моделью события. ${путь} подставляет значение поля.',
    fields: {
      title: { label: 'Заголовок' },
      ci: { label: 'КЕ', help: 'имя, хост, тег или облачный ID — КЕ найдёт CI Resolver' },
      signal: { label: 'Сигнал' },
      method: { label: 'Метод' },
      severity: { label: 'Severity' },
      status: { label: 'Статус', help: 'resolved, ok, closed, recovery → resolved; иначе firing' },
      external_id: { label: 'ID в источнике' },
      value: { label: 'Значение' },
    },
  },
  ack_response: {
    title: 'Подтверждение получения',
    description: 'Ответ источнику после записи событий. Для pull — сдвиг курсора.',
    fields: {
      mode: { label: 'Способ' },
    },
  },
  out_event: {
    title: 'Событие',
    description: 'Передаёт нормализованные события в обработку тревог.',
    fields: {},
  },
}

export const en: typeof ru = {
  categories: {
    trigger: 'Triggers',
    fetch: 'Fetch',
    parse: 'Parsing',
    transform: 'Transform',
    ack: 'Acknowledgement',
    output: 'Output',
  },
  trigger_webhook: {
    title: 'Incoming webhook',
    description: 'The source sends events to /api/ingest/{id}. A 2xx response is returned only after the event is stored.',
    fields: {
      auth: { label: 'Verification' },
      secret_ref: { label: 'Secret reference', help: 'The token itself is kept only in the secret store' },
    },
  },
  trigger_schedule: {
    title: 'Schedule',
    description: 'Fetches data at the given interval (pull).',
    fields: {
      interval: { label: 'Interval' },
    },
  },
  fetch_http: {
    title: 'HTTP request',
    description: 'GET or POST to the source REST API.',
    fields: {
      url: { label: 'URL' },
      method: { label: 'Method' },
      body: { label: 'Request body' },
      auth_header: { label: 'Authorization header' },
      secret_ref: { label: 'Secret reference' },
    },
  },
  parse_json: {
    title: 'Parse JSON',
    description: 'Parses JSON. An array path splits a batch into separate events.',
    fields: {
      items: { label: 'Path to event array', help: 'Empty: the whole body is one event or an array' },
    },
  },
  parse_kv: {
    title: 'Parse key=value',
    description: 'Lines like host=db1 sev=5 msg="disk full". Each line is an event.',
    fields: {
      pair_sep: { label: 'Pair separator' },
      kv_sep: { label: 'Key separator' },
    },
  },
  parse_regex: {
    title: 'Parse regex',
    description: 'Named groups (?P<name>...) become fields. Each line is an event.',
    fields: {
      pattern: { label: 'Expression' },
    },
  },
  parse_csv: {
    title: 'Parse CSV',
    description: 'The first line is the header, the rest are events.',
    fields: {
      delimiter: { label: 'Delimiter' },
    },
  },
  filter: {
    title: 'Filter',
    description: 'Passes on only the events that match the condition.',
    fields: {
      field: { label: 'Field' },
      op: { label: 'Condition' },
      value: { label: 'Value' },
    },
  },
  map_severity: {
    title: 'Severity mapping',
    description: 'Maps a source field value to critical, error, warning or info.',
    fields: {
      field: { label: 'Source field' },
      mapping: { label: 'Mapping', help: 'one line per value; * matches everything else' },
    },
  },
  enrich_labels: {
    title: 'Labels',
    description: 'Adds static or computed labels.',
    fields: {
      labels: { label: 'Labels' },
    },
  },
  map_event: {
    title: 'Source template',
    description: 'Maps source fields to the unified event model. ${path} inserts a field value.',
    fields: {
      title: { label: 'Title' },
      ci: { label: 'CI', help: 'name, host, tag or cloud ID — CI Resolver will find the CI' },
      signal: { label: 'Signal' },
      method: { label: 'Method' },
      severity: { label: 'Severity' },
      status: { label: 'Status', help: 'resolved, ok, closed, recovery → resolved; otherwise firing' },
      external_id: { label: 'Source ID' },
      value: { label: 'Value' },
    },
  },
  ack_response: {
    title: 'Delivery acknowledgement',
    description: 'Response to the source after events are stored. For pull, advances the cursor.',
    fields: {
      mode: { label: 'Mode' },
    },
  },
  out_event: {
    title: 'Event',
    description: 'Hands normalized events over to alert processing.',
    fields: {},
  },
}
