package auth

import (
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

// Built-in role ids.
const (
	RoleAdmin      = "admin"
	RoleMonitoring = "monitoring"
	RoleOnCall     = "oncall"
	RoleOwner      = "owner"
	RoleViewer     = "viewer"
	RoleAuditor    = "auditor"
	RoleReader     = "reader"
)

// BuiltInRoles returns the default role set. Their permissions can be edited
// except for admin; they cannot be deleted.
func BuiltInRoles(now time.Time) []model.Role {
	p := model.AllPermissions
	return []model.Role{
		{ID: RoleAdmin, Name: "Администратор", Description: "Все функции, пользователи и роли", Permissions: append([]string(nil), p...), AllServices: true},
		{ID: RoleMonitoring, Name: "Инженер мониторинга", Description: "Коннекторы, CMDB, правила, каналы уведомлений", AllServices: true,
			Permissions: []string{model.PermIncidentsView, model.PermIncidentsAct, model.PermCMDBView, model.PermCMDBEdit, model.PermConnectorsView,
				model.PermConnectorsEdit, model.PermEventsView, model.PermMaintenanceEdit, model.PermRulesView, model.PermNotifyEdit,
				model.PermSelfcheckView, model.PermSelfcheckAdmin}},
		{ID: RoleOnCall, Name: "Дежурный инженер", Description: "Работа с инцидентами всех услуг", AllServices: true,
			Permissions: []string{model.PermIncidentsView, model.PermIncidentsAct, model.PermCMDBView, model.PermEventsView,
				model.PermMaintenanceEdit, model.PermRulesView, model.PermSelfcheckView}},
		{ID: RoleOwner, Name: "Владелец услуги", Description: "Инциденты и обслуживание своих бизнес-услуг",
			Permissions: []string{model.PermIncidentsView, model.PermIncidentsAct, model.PermCMDBView, model.PermMaintenanceEdit}},
		{ID: RoleViewer, Name: "Наблюдатель", Description: "Просмотр инцидентов своих бизнес-услуг",
			Permissions: []string{model.PermIncidentsView, model.PermCMDBView}},
		{ID: RoleAuditor, Name: "Аудитор", Description: "Журнал аудита и просмотр без изменений", AllServices: true,
			Permissions: []string{model.PermIncidentsView, model.PermCMDBView, model.PermEventsView, model.PermConnectorsView,
				model.PermRulesView, model.PermSelfcheckView, model.PermAuditView}},
		{ID: RoleReader, Name: "Чтение (сервисная)", Description: "Только чтение всех инцидентов: Grafana, отчёты", AllServices: true,
			Permissions: []string{model.PermIncidentsView, model.PermCMDBView, model.PermEventsView}},
	}
}

// ValidPermission reports whether p is known.
func ValidPermission(p string) bool {
	for _, x := range model.AllPermissions {
		if x == p {
			return true
		}
	}
	return false
}
