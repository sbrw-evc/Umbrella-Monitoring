package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// heatmapColumns is the number of time buckets; the bucket length follows
// the period, so a day is 24 hourly columns and a week is 24 columns of 7 h.
const heatmapColumns = 24

// HeatCell is one CI in one time bucket: how many incidents were active
// in that bucket and the worst severity among them.
type HeatCell struct {
	N   int            `json:"n"`
	Sev model.Severity `json:"sev,omitempty"`
}

// HeatRow is one CI (or one CI name without a CMDB record).
type HeatRow struct {
	CIID   string         `json:"ci_id,omitempty"`
	CIName string         `json:"ci_name"`
	CIType string         `json:"ci_type,omitempty"`
	Status model.Severity `json:"status"` // current, propagated from CIs below
	Open   int            `json:"open"`   // active incidents on the CI now
	Total  int            `json:"total"`  // incidents active at any time in the period
	Maint  bool           `json:"maintenance"`
	Cells  []HeatCell     `json:"cells"`
}

// HeatGroup groups rows by IT service, team or CI type.
type HeatGroup struct {
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Status model.Severity `json:"status"`
	Total  int            `json:"total"`
	Rows   []*HeatRow     `json:"rows"`
}

// Heatmap is the response of GET /api/heatmap.
type Heatmap struct {
	Hours         int          `json:"hours"`
	BucketMinutes int          `json:"bucket_minutes"`
	From          time.Time    `json:"from"`
	To            time.Time    `json:"to"`
	Group         string       `json:"group"`
	Columns       []time.Time  `json:"columns"` // bucket starts
	Max           int          `json:"max"`     // largest cell count, for the color scale
	Totals        []HeatCell   `json:"totals"`  // per bucket over all rows
	Groups        []*HeatGroup `json:"groups"`
}

// heatmap answers GET /api/heatmap?hours=24&group=service|team|type&team=&type=&state=problem.
func (s *Server) heatmap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hours, _ := strconv.Atoi(q.Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	if hours > 30*24 {
		hours = 30 * 24
	}
	group := q.Get("group")
	if group != "team" && group != "type" {
		group = "service"
	}
	var hm Heatmap
	s.st.Read(func(d *store.Data) {
		hm = buildHeatmap(d, time.Now(), hours, group, q.Get("team"), splitList(q.Get("type")), q.Get("state") == "problem")
	})
	writeJSON(w, 200, hm)
}

func buildHeatmap(d *store.Data, now time.Time, hours int, group, team string, types []string, problemsOnly bool) Heatmap {
	bucket := time.Duration(hours) * time.Hour / heatmapColumns
	to := now.Truncate(time.Minute).Add(time.Minute)
	from := to.Add(-time.Duration(hours) * time.Hour)
	hm := Heatmap{Hours: hours, BucketMinutes: int(bucket / time.Minute), From: from, To: to, Group: group}
	for i := 0; i < heatmapColumns; i++ {
		hm.Columns = append(hm.Columns, from.Add(time.Duration(i)*bucket))
	}
	hm.Totals = make([]HeatCell, heatmapColumns)

	inScope := func(t, typ string) bool {
		if team != "" && team != "all" && t != team {
			return false
		}
		return len(types) == 0 || contains(types, typ)
	}

	views := ciViews(d, now)
	rows := map[string]*HeatRow{} // by CI ID, or "name:"+CIName for unknown CIs
	rowGroup := map[string]*HeatGroup{}
	groups := map[string]*HeatGroup{}
	groupOf := func(ci *model.CI, a *model.Alert) (string, string) {
		switch group {
		case "team":
			t := ""
			if ci != nil {
				t = ci.Team
			} else if a != nil {
				t = a.Team
			}
			return t, t
		case "type":
			if ci != nil {
				return ci.Type, ci.Type
			}
			return "", ""
		}
		if ci != nil {
			if svc := alert.ServiceOf(d, ci.ID); svc != nil {
				return svc.ID, svc.Name
			}
			// Business services sit above IT services and form their own group.
			if ci.Type == model.CIBusinessService {
				return "business", ""
			}
		} else if a != nil && a.Service != "" {
			return "svc:" + a.Service, a.Service
		}
		return "", ""
	}
	ensure := func(key string, ci *model.CI, a *model.Alert) *HeatRow {
		if row := rows[key]; row != nil {
			return row
		}
		row := &HeatRow{Cells: make([]HeatCell, heatmapColumns)}
		if ci != nil {
			row.CIID, row.CIName, row.CIType = ci.ID, ci.Name, ci.Type
			if v := views[ci.ID]; v != nil {
				row.Status, row.Open, row.Maint = v.Status, v.OpenAlerts, v.Maintenance
			}
		} else {
			row.CIName, row.CIType = a.CIName, a.CIType
		}
		gk, gn := groupOf(ci, a)
		g := groups[gk]
		if g == nil {
			g = &HeatGroup{Key: gk, Name: gn}
			groups[gk] = g
		}
		rows[key] = row
		rowGroup[key] = g
		return row
	}

	for _, ci := range d.CIs {
		if inScope(ci.Team, ci.Type) {
			ensure(ci.ID, ci, nil)
		}
	}
	for _, a := range d.Alerts {
		if a.Suppressed {
			continue
		}
		end := now
		if a.ResolvedAt != nil {
			end = *a.ResolvedAt
		}
		if end.Before(from) || !a.FirstSeen.Before(to) {
			continue
		}
		ci := d.CIs[a.CIID]
		var key string
		if ci != nil {
			if !inScope(ci.Team, ci.Type) {
				continue
			}
			key = ci.ID
		} else {
			if !inScope(a.Team, a.CIType) {
				continue
			}
			key = "name:" + a.CIName
		}
		row := ensure(key, ci, a)
		if ci == nil && a.Status.Active() {
			row.Open++
			row.Status = model.MaxSeverity(row.Status, a.Severity)
		}
		row.Total++
		first := int(a.FirstSeen.Sub(from) / bucket)
		last := int(end.Sub(from) / bucket)
		if first < 0 {
			first = 0
		}
		if last >= heatmapColumns {
			last = heatmapColumns - 1
		}
		for i := first; i <= last; i++ {
			c := &row.Cells[i]
			c.N++
			c.Sev = model.MaxSeverity(c.Sev, a.Severity)
			t := &hm.Totals[i]
			t.N++
			t.Sev = model.MaxSeverity(t.Sev, a.Severity)
		}
	}

	for key, row := range rows {
		if problemsOnly && row.Total == 0 && row.Status == "" {
			continue
		}
		g := rowGroup[key]
		g.Rows = append(g.Rows, row)
		g.Total += row.Total
		g.Status = model.MaxSeverity(g.Status, row.Status)
		for _, c := range row.Cells {
			if c.N > hm.Max {
				hm.Max = c.N
			}
		}
	}
	for _, g := range groups {
		if len(g.Rows) == 0 {
			continue
		}
		sort.Slice(g.Rows, func(i, j int) bool {
			a, b := g.Rows[i], g.Rows[j]
			if a.Status.Rank() != b.Status.Rank() {
				return a.Status.Rank() > b.Status.Rank()
			}
			if a.Total != b.Total {
				return a.Total > b.Total
			}
			return a.CIName < b.CIName
		})
		hm.Groups = append(hm.Groups, g)
	}
	sort.Slice(hm.Groups, func(i, j int) bool {
		a, b := hm.Groups[i], hm.Groups[j]
		// The "no group" bucket always goes last.
		if (a.Key == "") != (b.Key == "") {
			return b.Key == ""
		}
		if a.Status.Rank() != b.Status.Rank() {
			return a.Status.Rank() > b.Status.Rank()
		}
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		return a.Name < b.Name
	})
	if hm.Groups == nil {
		hm.Groups = []*HeatGroup{}
	}
	return hm
}
