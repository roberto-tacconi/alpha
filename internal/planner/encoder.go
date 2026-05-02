package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"alpha/internal/pkg/pddlx/ast"
)

type ProblemEncoder interface {
	Encode(w *world.WorldState, parts ProblemParts) (*EncodedProblem, error)
}

type EncodedProblem struct {
	Objects []ast.TypedElement
	Preds   []ast.Predicate
	Assigns []ast.NumericAssignment
	Goals   []ast.Predicate
	Metric  *ast.Metric
}

type alphaProblemEncoder struct{}

func NewAlphaProblemEncoder() ProblemEncoder {
	return &alphaProblemEncoder{}
}

func (e *alphaProblemEncoder) Encode(w *world.WorldState, parts ProblemParts) (*EncodedProblem, error) {
	objects, err := e.extractObjects(w)
	if err != nil {
		return nil, err
	}

	preds, assigns, err := e.extractInitialState(w, parts)
	if err != nil {
		return nil, err
	}

	goals := e.extractGoals(parts)

	return &EncodedProblem{
		Objects: objects,
		Preds:   preds,
		Assigns: assigns,
		Goals:   goals,
		Metric:  parts.Metric,
	}, nil
}

func (e *alphaProblemEncoder) extractObjects(w *world.WorldState) ([]ast.TypedElement, error) {
	roots := w.RootCapabilities()
	children := w.ChildCapabilities()
	services := w.Services()
	digitals := w.Digitals()
	analogs := w.Analogs()

	objects := make([]ast.TypedElement, 0,
		len(roots)+len(children)+len(services)+len(digitals)+len(analogs))

	for _, r := range roots {
		el, err := r.WritePDDLObject()
		if err != nil {
			return nil, err
		}
		objects = append(objects, el)
	}
	for _, c := range children {
		el, err := c.WritePDDLObject()
		if err != nil {
			return nil, err
		}
		objects = append(objects, el)
	}
	for _, s := range services {
		el, err := s.WritePDDLObject()
		if err != nil {
			return nil, err
		}
		objects = append(objects, el)
	}
	for _, d := range digitals {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}
		el, err := d.WritePDDLObject()
		if err != nil {
			return nil, err
		}
		objects = append(objects, el)
	}
	for _, a := range analogs {
		el, err := a.WritePDDLObject()
		if err != nil {
			return nil, err
		}
		objects = append(objects, el)
	}

	return objects, nil
}

func (e *alphaProblemEncoder) extractInitialState(w *world.WorldState, riskParts ProblemParts) ([]ast.Predicate, []ast.NumericAssignment, error) {
	estPreds := (len(w.Digitals()) * 6) +
		(len(w.RootCapabilities()) * 5) +
		(len(w.ChildCapabilities()) * 3) +
		(len(w.Services()) * 5) +
		len(riskParts.InitState)

	preds := make([]ast.Predicate, 0, estPreds)
	assigns := make([]ast.NumericAssignment, 0, len(riskParts.InitCosts)+1)

	assigns = append(assigns, ast.NewNumericAssignment("total-cost", 0, nil))
	for _, m := range riskParts.InitCosts {
		assigns = append(assigns, ast.NewNumericAssignment(m.Name, m.Value, m.Args))
	}
	for _, f := range riskParts.InitState {
		preds = append(preds, ast.NewPredicate(f.Name, f.Args...))
	}

	capPreds, err := e.buildCapabilityPredicates(w)
	if err != nil {
		return nil, nil, err
	}
	preds = append(preds, capPreds...)
	preds = append(preds, e.buildServicePredicates(w)...)
	preds = append(preds, e.buildDigitalPredicates(w)...)
	preds = append(preds, e.buildAnalogPredicates(w)...)

	return preds, assigns, nil
}

func (e *alphaProblemEncoder) extractGoals(riskParts ProblemParts) []ast.Predicate {
	goals := make([]ast.Predicate, 0, len(riskParts.Goals))
	for _, g := range riskParts.Goals {
		pred := ast.NewPredicate(g.Name, g.Args...)
		pred.Negated = g.Negated
		goals = append(goals, pred)
	}
	return goals
}

func (e *alphaProblemEncoder) buildCapabilityPredicates(w *world.WorldState) ([]ast.Predicate, error) {
	roots := w.RootCapabilities()
	children := w.ChildCapabilities()

	preds := make([]ast.Predicate, 0, len(roots)*4+len(children)*3)

	for _, r := range roots {
		rName := r.PDDLName()

		if r.ContextCritical() {
			preds = append(preds, ast.NewPredicate("context-critical", rName))
		}

		ok, err := w.AllowsCapabilityCriticalityInheritance(r.ID())
		if err != nil {
			return nil, err
		}
		if ok && !r.ContextCritical() {
			preds = append(preds, ast.NewPredicate("capability-can-inherit-criticality", rName))
		}

		for _, prov := range r.Providers() {
			if prov.Service() == nil {
				continue
			}
			preds = append(preds, ast.NewPredicate("can-be-provided", rName, prov.Service().PDDLName()))
		}

		if prov := r.CurrentProvider(); prov != nil {
			preds = append(preds, ast.NewPredicate("root-is-provided-by", rName, prov.PDDLName()))
		}
	}

	for _, c := range children {
		cName := c.PDDLName()
		rName := c.Root().PDDLName()

		preds = append(preds, ast.NewPredicate("is-part-of", cName, rName))

		ok, err := w.AllowsCapabilityCriticalityInheritance(c.ID())
		if err != nil {
			return nil, err
		}
		if ok {
			preds = append(preds, ast.NewPredicate("capability-can-inherit-criticality", cName))
		}

		for _, prov := range c.Providers() {
			if prov.Service() == nil {
				continue
			}
			preds = append(preds, ast.NewPredicate("can-be-provided", cName, prov.Service().PDDLName()))
		}
	}

	return preds, nil
}

func (e *alphaProblemEncoder) buildServicePredicates(w *world.WorldState) []ast.Predicate {
	services := w.Services()
	preds := make([]ast.Predicate, 0, len(services)*5)

	for _, s := range services {
		sName := s.PDDLName()

		if s.SupportsCriticality() {
			preds = append(preds, ast.NewPredicate("provider-can-be-critical", sName))
		}

		if ok, _ := w.AllowsServiceCriticalityInheritance(s.ID()); ok && !s.SupportsCriticality() {
			preds = append(preds, ast.NewPredicate("provider-can-inherit-criticality", sName))
		}

		for _, req := range s.TransitiveRequirements() {
			if value, ok := req.(world.PDDLConverter); ok {
				preds = append(preds, ast.NewPredicate("transitively-requires", sName, value.PDDLName()))
			}
		}

		if host := s.CurrentHost(); host != nil {
			if value, ok := host.(world.PDDLConverter); ok {
				preds = append(preds, ast.NewPredicate("is-hosted-on", sName, value.PDDLName()))
			}
		}
	}

	return preds
}

func (e *alphaProblemEncoder) buildDigitalPredicates(w *world.WorldState) []ast.Predicate {
	digitals := w.Digitals()
	preds := make([]ast.Predicate, 0, len(digitals)*6)

	for _, d := range digitals {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}
		name := d.PDDLName()

		if ok, _ := w.AllowsDigitalCriticality(d.ID()); ok {
			preds = append(preds, ast.NewPredicate("host-can-be-critical", name))
		}

		if d.HasCleanImage() {
			preds = append(preds, ast.NewPredicate("has-clean-image", name))
		}

		for _, hs := range d.SupportedServices() {
			if hs.Service == nil {
				continue
			}
			preds = append(preds, ast.NewPredicate("can-be-hosted", hs.Service.PDDLName(), name))
		}

		if d.Powered() {
			preds = append(preds, ast.NewPredicate("is-powered-on", name))
		}
		if d.Quarantined() {
			preds = append(preds, ast.NewPredicate("is-quarantined", name))
		}
		if d.Compromised() {
			preds = append(preds, ast.NewPredicate("is-compromised", name))
		}
	}

	return preds
}

func (e *alphaProblemEncoder) buildAnalogPredicates(w *world.WorldState) []ast.Predicate {
	analogs := w.Analogs()
	preds := make([]ast.Predicate, 0, len(analogs)*2)

	for _, a := range analogs {
		aName := a.PDDLName()
		for _, hs := range a.SupportedServices() {
			if hs.Service == nil {
				continue
			}
			preds = append(preds, ast.NewPredicate("can-be-hosted", hs.Service.PDDLName(), aName))
		}
	}

	return preds
}
