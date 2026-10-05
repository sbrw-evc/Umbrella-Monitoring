package integration

const (
	TypeZabbix        = "zabbix"
	TypeAlertmanager  = "alertmanager"
	TypeGrafana       = "grafana"
	TypeOpenSearch    = "opensearch"
	TypeElasticsearch = "elasticsearch"
	TypeWebhook       = "webhook"
	TypeHTTP          = "http"
	TypePrometheus    = "prometheus"
	TypeNetBox        = "netbox"

	ModePush      = "push"
	ModePull      = "pull"
	ModeMetrics   = "metrics"
	ModeInventory = "inventory"

	AuthNone   = "none"
	AuthToken  = "token"
	AuthBasic  = "basic"
	AuthAPIKey = "apikey"
	AuthBearer = "bearer"
)

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

type TypeSpec struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Mode           string   `json:"mode"`
	URLLabel       string   `json:"url_label"`
	URLPlaceholder string   `json:"url_placeholder"`
	URLRequired    bool     `json:"url_required"`
	Auth           []string `json:"auth"`
	SecretLabel    string   `json:"secret_label"`
	Params         []Field  `json:"params"`
	Setup          bool     `json:"setup"`
	SetupHelp      string   `json:"setup_help,omitempty"`
}

var umbrellaURLField = Field{Key: "umbrella_url", Label: "Адрес Umbrella для источника", Type: "text",
	Placeholder: "http://umbrella:8080", Help: "Как источник достаёт до Umbrella; пусто — внешний адрес Umbrella"}

var mappingFields = []Field{
	{Key: "items", Label: "Путь к массиву событий", Type: "text", Placeholder: "data.alerts", Help: "Пусто: тело целиком — одно событие или массив"},
	{Key: "title", Label: "Заголовок", Type: "text", Default: "${title}"},
	{Key: "ci", Label: "КЕ", Type: "text", Default: "${host}"},
	{Key: "signal", Label: "Сигнал", Type: "text", Default: "${signal|generic}"},
	{Key: "severity_field", Label: "Поле важности", Type: "text", Default: "severity"},
	{Key: "severity_map", Label: "Соответствие важности", Type: "textarea", Default: "critical=critical\nerror=error\nwarning=warning\n*=info"},
	{Key: "status", Label: "Статус", Type: "text", Default: "${status}", Help: "resolved, ok, closed, recovery → решено"},
	{Key: "external_id", Label: "ID в источнике", Type: "text", Default: "${id}"},
	{Key: "method", Label: "Метод", Type: "select", Options: []string{"other", "red", "use"}, Default: "other"},
}

var logFields = []Field{
	{Key: "index", Label: "Индекс или шаблон", Type: "text", Required: true, Placeholder: "logs-*"},
	{Key: "interval", Label: "Интервал опроса", Type: "text", Default: "30s"},
	{Key: "window", Label: "Окно поиска", Type: "text", Default: "5m", Help: "Документы новее now-окно; повторы отбрасываются по _id"},
	{Key: "time_field", Label: "Поле времени", Type: "text", Default: "@timestamp"},
	{Key: "level_field", Label: "Поле уровня", Type: "text", Default: "level"},
	{Key: "levels", Label: "Уровни, которые создают события", Type: "text", Default: "error,critical,fatal,panic"},
	{Key: "host_field", Label: "Поле КЕ (хост)", Type: "text", Default: "host"},
	{Key: "service_field", Label: "Поле сервиса", Type: "text", Default: "service"},
	{Key: "message_field", Label: "Поле текста", Type: "text", Default: "message"},
	{Key: "code_field", Label: "Поле кода ошибки", Type: "text", Default: "error_code"},
}

var Types = []TypeSpec{
	{ID: TypeZabbix, Title: "Zabbix", Mode: ModePush,
		Description: "Проблемы и восстановления Zabbix. Umbrella сама создаёт в Zabbix тип оповещения «webhook», медиа пользователя и действие.",
		URLLabel:    "Адрес веб-интерфейса Zabbix", URLPlaceholder: "https://zabbix.example.com", URLRequired: true,
		Auth: []string{AuthToken, AuthBasic}, SecretLabel: "API-токен или пароль Zabbix",
		Params: []Field{
			{Key: "zabbix_user", Label: "Пользователь Zabbix, от имени которого уходят оповещения", Type: "text", Default: "Admin"},
			{Key: "media_type", Label: "Название типа оповещения", Type: "text", Default: "Umbrella"},
			{Key: "action", Label: "Название действия", Type: "text", Default: "Send problems to Umbrella"},
			{Key: "discovery", Label: "Загружать хосты как КЕ", Type: "select", Options: []string{"true", "false"}, Default: "true"},
			{Key: "discovery_interval", Label: "Интервал загрузки хостов", Type: "text", Default: "10m"},
			umbrellaURLField,
		},
		Setup: true, SetupHelp: "Создаёт или обновляет в Zabbix тип оповещения, медиа пользователя и действие для проблем и восстановлений"},
	{ID: TypeAlertmanager, Title: "Prometheus Alertmanager", Mode: ModePush,
		Description: "Webhook Alertmanager: каждый элемент alerts[] — событие, resolved закрывает инцидент.",
		URLLabel:    "Адрес Alertmanager (для проверки)", URLPlaceholder: "http://alertmanager:9093",
		Auth: []string{AuthNone, AuthBasic, AuthToken}, SecretLabel: "Пароль или токен Alertmanager",
		Params:    []Field{umbrellaURLField},
		SetupHelp: "Конфигурацию Alertmanager задаёт его файл: вставьте показанный блок receivers"},
	{ID: TypeGrafana, Title: "Grafana Alerting", Mode: ModePush,
		Description: "Алерты Grafana. Umbrella создаёт в Grafana точку контакта webhook и маршрут уведомлений.",
		URLLabel:    "Адрес Grafana", URLPlaceholder: "https://grafana.example.com", URLRequired: true,
		Auth: []string{AuthToken, AuthBasic}, SecretLabel: "Токен сервисного аккаунта или пароль",
		Params: []Field{
			{Key: "contact_point", Label: "Название точки контакта", Type: "text", Default: "Umbrella"},
			{Key: "route_all", Label: "Отправлять все алерты (маршрут с continue)", Type: "select", Options: []string{"true", "false"}, Default: "true"},
			umbrellaURLField,
		},
		Setup: true, SetupHelp: "Создаёт или обновляет точку контакта webhook и маршрут уведомлений в Grafana"},
	{ID: TypeOpenSearch, Title: "OpenSearch", Mode: ModePull,
		Description: "Опрос индекса логов: записи выбранных уровней становятся событиями.",
		URLLabel:    "Адрес OpenSearch", URLPlaceholder: "https://opensearch:9200", URLRequired: true,
		Auth: []string{AuthNone, AuthBasic, AuthToken}, SecretLabel: "Пароль или токен",
		Params: logFields},
	{ID: TypeElasticsearch, Title: "Elasticsearch", Mode: ModePull,
		Description: "Опрос индекса логов Elasticsearch: записи выбранных уровней становятся событиями.",
		URLLabel:    "Адрес Elasticsearch", URLPlaceholder: "https://elasticsearch:9200", URLRequired: true,
		Auth: []string{AuthNone, AuthBasic, AuthAPIKey, AuthToken}, SecretLabel: "Пароль, API key или токен",
		Params: logFields},
	{ID: TypeWebhook, Title: "Webhook (JSON)", Mode: ModePush,
		Description: "Любая система, которая отправляет JSON на адрес Umbrella.",
		Auth:        []string{AuthNone}, Params: mappingFields},
	{ID: TypeHTTP, Title: "HTTP API (опрос)", Mode: ModePull,
		Description: "Umbrella периодически запрашивает REST API и разбирает JSON.",
		URLLabel:    "URL запроса", URLPlaceholder: "https://monitoring.example/api/problems", URLRequired: true,
		Auth: []string{AuthNone, AuthToken, AuthBasic, AuthAPIKey}, SecretLabel: "Пароль или токен",
		Params: append([]Field{
			{Key: "interval", Label: "Интервал опроса", Type: "text", Default: "60s"},
			{Key: "method", Label: "Метод", Type: "select", Options: []string{"GET", "POST"}, Default: "GET"},
			{Key: "body", Label: "Тело запроса", Type: "textarea"},
		}, mappingFields...)},
}

func init() {
	Types = append(Types,
		TypeSpec{ID: TypePrometheus, Title: "Prometheus (метрики для правил RED/USE)", Mode: ModeMetrics,
			Description: "Источник метрик для правил RED и USE: Prometheus, VictoriaMetrics, Thanos или Mimir с API /api/v1/query.",
			URLLabel:    "Адрес Prometheus API", URLPlaceholder: "http://prometheus:9090", URLRequired: true,
			Auth: []string{AuthNone, AuthBasic, AuthToken}, SecretLabel: "Пароль или токен",
			Params: []Field{
				{Key: "discovery", Label: "Загружать хосты как КЕ", Type: "select", Options: []string{"true", "false"}, Default: "true"},
				{Key: "discovery_interval", Label: "Интервал загрузки хостов", Type: "text", Default: "10m"},
				{Key: "host_label", Label: "Метка с именем хоста", Type: "text", Default: "host", Help: "Без неё имя берётся из instance"},
				{Key: "jobs", Label: "Только эти job", Type: "text", Placeholder: "node,cadvisor"},
			}},
		TypeSpec{ID: TypeNetBox, Title: "NetBox (КЕ и ответственные)", Mode: ModeInventory,
			Description: "Устройства и виртуальные машины NetBox становятся КЕ, контакты — ответственными (контакты объекта, иначе площадки, иначе арендатора).",
			URLLabel:    "Адрес NetBox", URLPlaceholder: "https://netbox.example.com", URLRequired: true,
			Auth: []string{AuthToken, AuthBearer}, SecretLabel: "API-токен NetBox",
			Params: []Field{
				{Key: "interval", Label: "Интервал синхронизации", Type: "text", Default: "15m"},
				{Key: "objects", Label: "Что загружать", Type: "select", Options: []string{"devices,vms", "devices", "vms"}, Default: "devices,vms"},
				{Key: "filter", Label: "Фильтр NetBox", Type: "text", Placeholder: "status=active&tag=prod", Help: "Параметры запроса API NetBox, общие для устройств и ВМ"},
				{Key: "team_from", Label: "Команда КЕ", Type: "select", Options: []string{"tenant", "site", "none"}, Default: "tenant", Help: "Из арендатора или площадки NetBox; none — команда интеграции"},
				{Key: "contact_roles", Label: "Роли контактов", Type: "text", Placeholder: "Owner,On-call", Help: "Пусто — все роли"},
				{Key: "missing", Label: "КЕ, пропавшие из NetBox", Type: "select", Options: []string{"mark", "delete"}, Default: "mark"},
			}},
	)
}

func Spec(id string) (TypeSpec, bool) {
	for _, t := range Types {
		if t.ID == id {
			return t, true
		}
	}
	return TypeSpec{}, false
}

func (t TypeSpec) param(params map[string]string, key string) string {
	if v := params[key]; v != "" {
		return v
	}
	for _, f := range t.Params {
		if f.Key == key {
			return f.Default
		}
	}
	return ""
}
