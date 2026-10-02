package alert

import "github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"

// DefaultRules is the catalog of RED and USE rules the engine applies. It is
// reference data shown on the rules page, present with or without demo data.
func DefaultRules() []model.Rule {
	return []model.Rule{
		{ID: "R-1", Method: model.MethodRED, Signal: "red.rate", Name: "Падение трафика", Condition: "rate < 0.5 × baseline(10m)", AppliesTo: "ИТ-сервисы", Severity: model.SevError, Enabled: true},
		{ID: "R-2", Method: model.MethodRED, Signal: "red.errors", Name: "Расход бюджета ошибок", Condition: "burn_rate(1h) > 14.4", AppliesTo: "ИТ-сервисы с SLO", Severity: model.SevCritical, Enabled: true},
		{ID: "R-3", Method: model.MethodRED, Signal: "red.duration", Name: "Задержка выше цели", Condition: "p99 > slo.latency for 5m", AppliesTo: "ИТ-сервисы с SLO", Severity: model.SevError, Enabled: true},
		{ID: "R-4", Method: model.MethodUSE, Signal: "use.cpu.utilization", Name: "Высокая загрузка", Condition: "utilization > 90% for 15m or disk_full_eta < 24h", AppliesTo: "хосты, БД, облачные группы", Severity: model.SevWarning, Enabled: true},
		{ID: "R-5", Method: model.MethodUSE, Signal: "use.saturation", Name: "Насыщение", Condition: "load > 2 × cores or queue grows 10m", AppliesTo: "хосты, БД", Severity: model.SevError, Enabled: true},
		{ID: "R-6", Method: model.MethodUSE, Signal: "use.errors", Name: "Ошибки ресурса", Condition: "increase(errors[5m]) > 0", AppliesTo: "все КЕ", Severity: model.SevWarning, Enabled: true},
	}
}
