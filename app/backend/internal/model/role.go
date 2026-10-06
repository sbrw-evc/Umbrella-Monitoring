package model

import "time"

type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	System      bool      `json:"system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func SystemRoles(now time.Time) []*Role {
	return []*Role{
		{ID: RoleAdmin, Name: "Administrator", System: true, CreatedAt: now, UpdatedAt: now},
		{ID: RoleUser, Name: "User", System: true, Permissions: []string{}, CreatedAt: now, UpdatedAt: now},
	}
}

// Preset roles are created once, on the first start of a version that has them, and are
// ordinary roles after that: they can be renamed, changed and deleted, and are not created again.
const (
	RoleViewer       = "preset-viewer"
	RoleOnCall       = "preset-oncall"
	RoleServiceOwner = "preset-service-owner"
	RoleIntegrations = "preset-integrations"
	RoleNOCLead      = "preset-noc-lead"
)

type presetRole struct {
	id             string
	nameEN, nameRU string
	descEN, descRU string
	permissions    []string
}

var viewerPerms = []string{"incidents:view", "cmdb:view", "cis:view", "services:view", "maintenance:view", "wallboards:view"}

func withPerms(base []string, extra ...string) []string {
	return append(append([]string{}, base...), extra...)
}

var onCallPerms = withPerms(viewerPerms, "incidents:ack", "maintenance:edit")

var presetRoles = []presetRole{
	{RoleViewer, "Viewer", "Наблюдатель",
		"Sees incidents, the CMDB map, configuration items, services, maintenance windows and TV wallboards; changes nothing.",
		"Видит инциденты, карту CMDB, КЕ, сервисы, сервисные окна и ТВ-панели; ничего не меняет.",
		viewerPerms},
	{RoleOnCall, "On-call engineer", "Дежурный инженер",
		"Viewer who also acknowledges, resolves and comments incidents and plans maintenance windows.",
		"Наблюдатель, который также подтверждает, решает и комментирует инциденты и планирует сервисные окна.",
		onCallPerms},
	{RoleServiceOwner, "Service owner", "Владелец сервиса",
		"Sees and acknowledges incidents of their services. Give it with the scope «services of my teams».",
		"Видит и подтверждает инциденты своих сервисов. Выдавайте вместе с областью «сервисы моих команд».",
		[]string{"incidents:view", "incidents:ack", "cmdb:view", "cis:view", "services:view", "maintenance:view"}},
	{RoleIntegrations, "Integration administrator", "Администратор интеграций",
		"Viewer who connects sources: connectors, credentials, RED/USE rules, monitoring systems, NetBox, configuration items and services.",
		"Наблюдатель, который подключает источники: коннекторы, учётные данные, правила RED/USE, системы мониторинга, NetBox, КЕ и сервисы.",
		withPerms(viewerPerms,
			"connectors:edit", "connectors:publish", "connectors:payload", "credentials:edit", "rules:edit",
			"monitoring:test", "monitoring:edit", "monitoring:sync", "monitoring:link",
			"netbox:test", "netbox:edit", "netbox:sync", "cis:edit", "services:edit", "status:view")},
	{RoleNOCLead, "NOC lead", "Руководитель NOC",
		"On-call engineer who also manages teams, users (except administrators), TV wallboards and checks alerting.",
		"Дежурный инженер, который также управляет командами, пользователями (кроме администраторов), ТВ-панелями и проверяет оповещения.",
		withPerms(onCallPerms, "teams:edit", "users:view", "users:edit", "users:lock", "wallboards:edit",
			"settings.alerting:view", "settings.alerting:test", "status:view")},
}

// PresetRoles are the ready-made roles, named in the language of the installation. Permissions
// are not normalized here: the caller adds the view permission of every page it grants.
func PresetRoles(locale string, now time.Time) []*Role {
	out := make([]*Role, 0, len(presetRoles))
	for _, p := range presetRoles {
		r := &Role{ID: p.id, Name: p.nameEN, Description: p.descEN, Permissions: append([]string{}, p.permissions...), CreatedAt: now, UpdatedAt: now}
		if locale == LocaleRU {
			r.Name, r.Description = p.nameRU, p.descRU
		}
		out = append(out, r)
	}
	return out
}
