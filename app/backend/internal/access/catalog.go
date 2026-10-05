package access

import (
	"slices"
	"strings"
)

type Text struct {
	EN string `json:"en"`
	RU string `json:"ru"`
}

type Feature struct {
	ID    string `json:"id"`
	Title Text   `json:"title"`
}

type Page struct {
	ID       string    `json:"id"`
	Group    string    `json:"group"`
	Title    Text      `json:"title"`
	Features []Feature `json:"features"`
}

type Group struct {
	ID    string `json:"id"`
	Title Text   `json:"title"`
}

const (
	View = "view"

	GroupMain     = "main"
	GroupOverview = "overview"
	GroupAuto     = "automation"
	GroupOrg      = "org"
	GroupSettings = "settings"
)

var (
	view    = Feature{View, Text{"View the page", "Просмотр страницы"}}
	edit    = Feature{"edit", Text{"Create, change and delete", "Создание, изменение и удаление"}}
	test    = Feature{"test", Text{"Check the connection", "Проверка подключения"}}
	migrate = Feature{"migrate", Text{"Move data to another installation", "Перенос данных в другую установку"}}
)

var groups = []Group{
	{GroupMain, Text{"Monitoring", "Мониторинг"}},
	{GroupOverview, Text{"Overview", "Обзор"}},
	{GroupAuto, Text{"Automation", "Автоматизация"}},
	{GroupOrg, Text{"Organization", "Организация"}},
	{GroupSettings, Text{"Settings", "Настройки"}},
}

var pages = []Page{
	{"incidents", GroupMain, Text{"Incidents", "Инциденты"}, []Feature{
		view,
		{"ack", Text{"Acknowledge, resolve and comment incidents", "Подтверждение, решение и комментарии инцидентов"}},
	}},
	{"services", GroupOverview, Text{"Business services", "Бизнес-сервисы"}, []Feature{
		view,
		{"edit", Text{"Create, change and delete services, bind configuration items, link services to NetBox", "Создание, изменение и удаление сервисов, привязка КЕ, связь с NetBox"}},
	}},
	{"cis", GroupOverview, Text{"Configuration items", "Конфигурационные единицы"}, []Feature{
		view,
		{"edit", Text{"Create, change and delete configuration items, also in NetBox", "Создание, изменение и удаление КЕ, в том числе в NetBox"}},
	}},
	{"cmdb", GroupOverview, Text{"CMDB map", "Карта CMDB"}, []Feature{view}},
	{"connectors", GroupAuto, Text{"Connectors", "Коннекторы"}, []Feature{
		view,
		{"edit", Text{"Create and change drafts, samples and test runs", "Создание и изменение черновиков, образцов и тестовых прогонов"}},
		{"publish", Text{"Publish and stop connectors", "Публикация и остановка коннекторов"}},
		{"payload", Text{"See request bodies, samples and failed records", "Просмотр тел запросов, образцов и ошибочных записей"}},
	}},
	{"credentials", GroupAuto, Text{"Credentials", "Учётные данные"}, []Feature{
		view,
		{"edit", Text{"Create, replace and delete credentials", "Создание, замена и удаление учётных данных"}},
	}},
	{"netbox", GroupAuto, Text{"NetBox", "NetBox"}, []Feature{
		view,
		test,
		{"edit", Text{"Change the connection and synchronization settings", "Изменение подключения и настроек синхронизации"}},
		{"sync", Text{"Run synchronization", "Запуск синхронизации"}},
	}},
	{"users", GroupOrg, Text{"Users", "Пользователи"}, []Feature{
		view,
		{"create", Text{"Create local users", "Создание локальных пользователей"}},
		{"edit", Text{"Edit users, their role and team", "Изменение пользователей, их роли и команды"}},
		{"lock", Text{"Lock and unlock users", "Блокировка и разблокировка"}},
		{"password", Text{"Set passwords of local users", "Смена паролей локальных пользователей"}},
		{"delete", Text{"Delete users", "Удаление пользователей"}},
	}},
	{"roles", GroupOrg, Text{"Roles", "Роли"}, []Feature{view, edit}},
	{"teams", GroupOrg, Text{"Teams", "Команды"}, []Feature{view, edit}},
	{"status", GroupSettings, Text{"System status", "Состояние системы"}, []Feature{view, {"defaults", Text{"Change default theme, language and time zone", "Изменение темы, языка и часового пояса по умолчанию"}}}},
	{"settings.postgres", GroupSettings, Text{"PostgreSQL", "PostgreSQL"}, []Feature{view, test, migrate}},
	{"settings.openbao", GroupSettings, Text{"OpenBao", "OpenBao"}, []Feature{view, test, migrate}},
	{"settings.ldap", GroupSettings, Text{"LDAP / AD", "LDAP / AD"}, []Feature{view, test, {"edit", Text{"Change the connection", "Изменение подключения"}}}},
	{"settings.policy", GroupSettings, Text{"Password policy", "Парольная политика"}, []Feature{view, {"edit", Text{"Change the policy", "Изменение политики"}}}},
}

func Groups() []Group { return slices.Clone(groups) }

func Pages() []Page { return slices.Clone(pages) }

func Perm(page, feature string) string { return page + ":" + feature }

func All() []string {
	var out []string
	for _, p := range pages {
		for _, f := range p.Features {
			out = append(out, Perm(p.ID, f.ID))
		}
	}
	return out
}

func Valid(perm string) bool { return slices.Contains(All(), perm) }

func Normalize(perms []string) []string {
	set := map[string]bool{}
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if !Valid(p) {
			continue
		}
		set[p] = true
		page, _, _ := strings.Cut(p, ":")
		set[Perm(page, View)] = true
	}
	out := make([]string, 0, len(set))
	for _, p := range All() {
		if set[p] {
			out = append(out, p)
		}
	}
	return out
}

type Set map[string]bool

func NewSet(perms []string) Set {
	s := Set{}
	for _, p := range perms {
		s[p] = true
	}
	return s
}

func (s Set) Has(perm string) bool { return s[perm] }

func (s Set) List() []string {
	out := []string{}
	for _, p := range All() {
		if s[p] {
			out = append(out, p)
		}
	}
	return out
}
