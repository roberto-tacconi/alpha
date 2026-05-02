package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
)

const (
	threatContainedName = "threat-contained"
	threatRemovedName   = "threat-removed"
	hostRecoveredName   = "host-recovered"

	isAvailableName = "is-available"
)

type IncidentRemediationStrategy struct{}

func (s *IncidentRemediationStrategy) CalculateGoals(w *world.WorldState) []Fact {
	goals := make([]Fact, 0, len(w.Digitals())*3)

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		args := []string{d.PDDLName()}

		if d.Compromised() {
			if d.Powered() && !d.Quarantined() {
				goals = append(goals, Fact{Name: threatContainedName, Args: args})
			}

			goals = append(goals,
				Fact{Name: threatRemovedName, Args: args},
				Fact{Name: hostRecoveredName, Args: args},
			)
		} else if d.Quarantined() || !d.Powered() {
			goals = append(goals, Fact{Name: hostRecoveredName, Args: args})
		}
	}

	return goals
}

type CriticalAvailabilityStrategy struct{}

func (s *CriticalAvailabilityStrategy) CalculateGoals(w *world.WorldState) []Fact {
	goals := make([]Fact, 0, len(w.RootCapabilities()))

	for _, r := range w.RootCapabilities() {
		if r.ContextCritical() {
			goals = append(goals, Fact{
				Name: isAvailableName,
				Args: []string{r.PDDLName()},
			})
		}
	}

	return goals
}
