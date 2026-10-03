export const ru = {
  header: {
    title: 'Команды',
    sub: 'Команды отвечают за КЕ, коннекторы, интеграции и правила, по ним фильтруются инциденты и строятся маршруты PagerDuty.',
    add: 'Создать команду',
  },
  table: {
    team: 'Команда',
    members: 'Участники',
    contacts: 'Контакты',
    usage: 'КЕ · коннекторы · правила',
    alerts: 'Открытые инциденты',
    unmanaged: 'не заведена',
    adopt: 'Завести',
    empty: 'Команд пока нет',
  },
  form: {
    createTitle: 'Новая команда',
    editTitle: 'Команда {name}',
    id: 'Код',
    idHelp: 'Латиница в нижнем регистре, цифры, точка, дефис; используется в КЕ, правилах и маршрутах. После создания не меняется.',
    name: 'Название',
    description: 'Описание',
    email: 'Почта команды',
    chat: 'Канал в мессенджере',
    members: 'Участники',
    leads: 'Руководители',
    delete: 'Удалить команду',
  },
  toasts: {
    created: 'Команда создана',
    saved: 'Команда сохранена',
    deleted: 'Команда удалена',
  },
  confirm: {
    delete: 'Удалить команду «{name}»?',
    detach: 'Команда используется ({n}). Удалить и отвязать от неё КЕ, коннекторы, интеграции и правила?',
  },
}

export const en: typeof ru = {
  header: {
    title: 'Teams',
    sub: 'Teams own CIs, connectors, integrations and rules; incidents are filtered and PagerDuty routes are built by team.',
    add: 'New team',
  },
  table: {
    team: 'Team',
    members: 'Members',
    contacts: 'Contacts',
    usage: 'CIs · connectors · rules',
    alerts: 'Open incidents',
    unmanaged: 'not registered',
    adopt: 'Register',
    empty: 'No teams yet',
  },
  form: {
    createTitle: 'New team',
    editTitle: 'Team {name}',
    id: 'Code',
    idHelp: 'Lowercase letters, digits, dot, dash; used in CIs, rules and routes. Cannot change after creation.',
    name: 'Name',
    description: 'Description',
    email: 'Team email',
    chat: 'Chat channel',
    members: 'Members',
    leads: 'Leads',
    delete: 'Delete team',
  },
  toasts: {
    created: 'Team created',
    saved: 'Team saved',
    deleted: 'Team deleted',
  },
  confirm: {
    delete: 'Delete team “{name}”?',
    detach: 'The team is in use ({n}). Delete it and detach CIs, connectors, integrations and rules?',
  },
}
