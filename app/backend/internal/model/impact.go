package model

// ImpactPolicy finds the priority of an incident from the severity of its events and the
// business impact (the criticality of the affected services), then the classification rules.
type ImpactPolicy struct{}
