// Package presets holds the built-in connector templates. They use the same document format
// as connector export, so a preset is just an import that ships with Umbrella.
package presets

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
)

//go:embed *.json
var files embed.FS

type Preset struct {
	ID          string        `json:"id"`
	Title       flow.Text     `json:"title"`
	Description flow.Text     `json:"description"`
	Document    flow.Document `json:"document"`
}

var all []Preset

func init() {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		b, err := files.ReadFile(e.Name())
		if err != nil {
			panic(err)
		}
		var p Preset
		if err := json.Unmarshal(b, &p); err != nil {
			panic(fmt.Sprintf("preset %s: %v", e.Name(), err))
		}
		doc, _ := json.Marshal(p.Document)
		if p.Document, err = flow.ParseDocument(doc); err != nil {
			panic(fmt.Sprintf("preset %s: %v", e.Name(), err))
		}
		all = append(all, p)
	}
	slices.SortFunc(all, func(a, b Preset) int {
		if a.ID == "webhook" {
			return 1
		}
		if b.ID == "webhook" {
			return -1
		}
		if a.Title.EN < b.Title.EN {
			return -1
		}
		return 1
	})
}

func All() []Preset { return slices.Clone(all) }

func Get(id string) (Preset, bool) {
	for _, p := range all {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
