import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { ApiError, type Locale } from './api'

export type Dict = Record<Locale, Record<string, string>>

const KEY = 'umbrella.locale'

export const common: Dict = {
  en: {
    'app.name': 'Umbrella Monitoring',
    'theme.light': 'Light',
    'theme.dark': 'Dark',
    'theme.toggle': 'Switch theme',
    'lang.en': 'English',
    'lang.ru': 'Русский',
    'lang.toggle': 'Language',
    'yes': 'Yes',
    'no': 'No',
    'loading': 'Loading…',
    'retry': 'Retry',
    'err.network': 'The server does not answer. Check the connection and try again.',
    'err.bad_request': 'The request was rejected as invalid.',
    'err.internal': 'Internal server error.',
    'err.invalid_setup_token': 'The setup code is wrong.',
    'err.too_many_attempts': 'Too many failed attempts. Wait a few minutes and try again.',
    'err.setup_completed': 'Setup is already completed. Reload the page.',
    'err.setup_required': 'Initial setup is not finished yet.',
    'err.weak_password': 'The password must be at least 10 characters long, contain letters and digits and differ from the username.',
    'err.invalid_username': 'Username: 2–64 characters, Latin letters, digits and . _ @ -, starting with a letter or digit.',
    'err.invalid_email': 'The e-mail address is invalid.',
    'err.invalid_name': 'The display name is too long.',
    'err.invalid_locale': 'Choose English or Russian.',
    'err.invalid_theme': 'Choose the light or dark theme.',
    'err.openbao_invalid': 'OpenBao settings are incomplete.',
    'err.openbao_unavailable': 'Umbrella cannot work with OpenBao using these settings.',
    'err.openbao_write_failed': 'Umbrella could not save secrets to OpenBao.',
    'err.ldap_invalid': 'LDAP / AD settings are incomplete.',
    'err.ldap_unavailable': 'Umbrella cannot connect to the directory using these settings.',
    'err.postgres_invalid': 'PostgreSQL settings are incomplete.',
    'err.postgres_unavailable': 'Umbrella cannot connect to PostgreSQL.',
    'err.postgres_has_state': 'The database already holds Umbrella data. Confirm that it should be used.',
    'err.postgres_no_create': 'The database user cannot create tables in the database.',
    'err.postgres_state_unreadable': 'The database holds Umbrella data that cannot be read.',
    'err.postgres_write_failed': 'Umbrella could not write to PostgreSQL.',
    'err.config_write_failed': 'Umbrella could not write its configuration file.',
    'err.invalid_credentials': 'Wrong username or password.',
    'err.directory_unavailable': 'The directory (LDAP / AD) is unavailable. Try again later or sign in with a local account.',
    'err.unauthenticated': 'Your session has ended. Sign in again.',
    'err.forbidden': 'Not enough permissions.',
    'err.csrf': 'The session token is outdated. Reload the page.',
    'err.unknown': 'Something went wrong.',
  },
  ru: {
    'app.name': 'Umbrella Monitoring',
    'theme.light': 'Светлая',
    'theme.dark': 'Тёмная',
    'theme.toggle': 'Сменить тему',
    'lang.en': 'English',
    'lang.ru': 'Русский',
    'lang.toggle': 'Язык',
    'yes': 'Да',
    'no': 'Нет',
    'loading': 'Загрузка…',
    'retry': 'Повторить',
    'err.network': 'Сервер не отвечает. Проверьте подключение и повторите.',
    'err.bad_request': 'Запрос отклонён как некорректный.',
    'err.internal': 'Внутренняя ошибка сервера.',
    'err.invalid_setup_token': 'Неверный код установки.',
    'err.too_many_attempts': 'Слишком много неудачных попыток. Подождите несколько минут.',
    'err.setup_completed': 'Настройка уже завершена. Обновите страницу.',
    'err.setup_required': 'Первичная настройка ещё не завершена.',
    'err.weak_password': 'Пароль: не короче 10 символов, буквы и цифры, не совпадает с логином.',
    'err.invalid_username': 'Логин: 2–64 символа — латиница, цифры и . _ @ -, начинается с буквы или цифры.',
    'err.invalid_email': 'Некорректный адрес почты.',
    'err.invalid_name': 'Слишком длинное имя.',
    'err.invalid_locale': 'Выберите английский или русский язык.',
    'err.invalid_theme': 'Выберите светлую или тёмную тему.',
    'err.openbao_invalid': 'Настройки OpenBao заполнены не полностью.',
    'err.openbao_unavailable': 'С этими настройками Umbrella не может работать с OpenBao.',
    'err.openbao_write_failed': 'Umbrella не смогла сохранить секреты в OpenBao.',
    'err.ldap_invalid': 'Настройки LDAP / AD заполнены не полностью.',
    'err.ldap_unavailable': 'С этими настройками Umbrella не может подключиться к каталогу.',
    'err.postgres_invalid': 'Настройки PostgreSQL заполнены не полностью.',
    'err.postgres_unavailable': 'Umbrella не может подключиться к PostgreSQL.',
    'err.postgres_has_state': 'В базе уже есть данные Umbrella. Подтвердите, что их нужно использовать.',
    'err.postgres_no_create': 'У пользователя БД нет права создавать таблицы.',
    'err.postgres_state_unreadable': 'В базе есть данные Umbrella, но прочитать их нельзя.',
    'err.postgres_write_failed': 'Umbrella не смогла записать данные в PostgreSQL.',
    'err.config_write_failed': 'Umbrella не смогла записать файл конфигурации.',
    'err.invalid_credentials': 'Неверный логин или пароль.',
    'err.directory_unavailable': 'Каталог (LDAP / AD) недоступен. Повторите позже или войдите локальной учётной записью.',
    'err.unauthenticated': 'Сессия завершилась. Войдите снова.',
    'err.forbidden': 'Недостаточно прав.',
    'err.csrf': 'Токен сессии устарел. Обновите страницу.',
    'err.unknown': 'Что-то пошло не так.',
  },
}

type Ctx = { locale: Locale; setLocale: (l: Locale, persist?: boolean) => void }

const LocaleContext = createContext<Ctx>({ locale: 'en', setLocale: () => {} })

export function savedLocale(): Locale | null {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'en' || v === 'ru' ? v : null
  } catch {
    return null
  }
}

export function browserLocale(): Locale {
  return navigator.language?.toLowerCase().startsWith('ru') ? 'ru' : 'en'
}

export function LocaleProvider({ initial, children }: { initial: Locale; children: ReactNode }) {
  const [locale, set] = useState<Locale>(initial)
  const setLocale = useCallback((l: Locale, persist = true) => {
    set(l)
    document.documentElement.lang = l
    if (!persist) return
    try {
      localStorage.setItem(KEY, l)
    } catch {
      return
    }
  }, [])
  document.documentElement.lang = locale
  return <LocaleContext.Provider value={{ locale, setLocale }}>{children}</LocaleContext.Provider>
}

export function useLocale() {
  return useContext(LocaleContext)
}

export function useT(extra?: Dict) {
  const { locale } = useLocale()
  return useMemo(() => {
    const table = { ...common[locale], ...(extra?.[locale] ?? {}) }
    return (key: string, vars?: Record<string, string | number>) => {
      let s = table[key] ?? key
      if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
      return s
    }
  }, [locale, extra])
}

export function errorText(t: (k: string) => string, e: unknown): { message: string; detail?: string } {
  if (e instanceof ApiError) {
    const key = `err.${e.code}`
    const message = t(key)
    return { message: message === key ? t('err.unknown') : message, detail: e.detail }
  }
  return { message: t('err.unknown'), detail: e instanceof Error ? e.message : String(e) }
}
