package memgraph

import (
	"alpha/internal/pkg/memgraphx"
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
)

type problemSpace struct {
	digitalIDs    []int64
	analogIDs     []int64
	serviceIDs    []int64
	capabilityIDs []int64
}

type perimeterState struct {
	visited mapset.Set[int64]

	blastDevices mapset.Set[int64]
	blastSrvs    mapset.Set[int64]
	blastCaps    mapset.Set[int64]

	altSrvs mapset.Set[int64]
	altCaps mapset.Set[int64]

	hostDigitals mapset.Set[int64]
	hostAnalogs  mapset.Set[int64]

	frontierSrvs mapset.Set[int64]
}

func newPerimeterState() *perimeterState {
	return &perimeterState{
		visited:      mapset.NewThreadUnsafeSet[int64](),
		blastDevices: mapset.NewThreadUnsafeSet[int64](),
		blastSrvs:    mapset.NewThreadUnsafeSet[int64](),
		blastCaps:    mapset.NewThreadUnsafeSet[int64](),
		altSrvs:      mapset.NewThreadUnsafeSet[int64](),
		altCaps:      mapset.NewThreadUnsafeSet[int64](),
		hostDigitals: mapset.NewThreadUnsafeSet[int64](),
		hostAnalogs:  mapset.NewThreadUnsafeSet[int64](),
		frontierSrvs: mapset.NewThreadUnsafeSet[int64](),
	}
}

func (ps *perimeterState) toProblemSpace() *problemSpace {
	allDigitals := ps.blastDevices.Union(ps.hostDigitals)
	allSrvs := ps.blastSrvs.Union(ps.altSrvs)
	allCaps := ps.blastCaps.Union(ps.altCaps)

	return &problemSpace{
		digitalIDs:    allDigitals.ToSlice(),
		analogIDs:     ps.hostAnalogs.ToSlice(),
		serviceIDs:    allSrvs.ToSlice(),
		capabilityIDs: allCaps.ToSlice(),
	}
}

func (r *graphRepository) calculatePerimeter(ctx context.Context, tx memgraphx.QueryExecutor, wm int64) (*problemSpace, error) {
	ps := newPerimeterState()

	if err := r.seeds(ctx, tx, wm, ps); err != nil {
		return nil, fmt.Errorf("extracting affected devices: %w", err)
	}

	if ps.blastDevices.IsEmpty() {
		return ps.toProblemSpace(), nil
	}

	if err := r.blastRadius(ctx, tx, ps); err != nil {
		return nil, fmt.Errorf("computing blast radius: %w", err)
	}

	if err := r.alternativeProviders(ctx, tx, ps); err != nil {
		return nil, fmt.Errorf("finding alternative providers: %w", err)
	}

	if err := r.dependencyClosure(ctx, tx, ps); err != nil {
		return nil, fmt.Errorf("computing dependency closure: %w", err)
	}

	if err := r.alternativeHosts(ctx, tx, ps); err != nil {
		return nil, fmt.Errorf("finding alternative hosts: %w", err)
	}

	return ps.toProblemSpace(), nil
}

func (r *graphRepository) seeds(ctx context.Context, tx memgraphx.QueryExecutor, wm int64, ps *perimeterState) error {
	const query = `
        MATCH (rd:Resource:Digital:Device)
 
        OPTIONAL MATCH (rd)-[:HAS_STATE]->(ds:DigitalState)
        WHERE ds.v_from <= $wm AND (ds.v_to IS NULL OR $wm < ds.v_to)
 
        OPTIONAL MATCH (rd)-[:HAS_INTERFACE]->(ni:NetworkInterface)
 
        OPTIONAL MATCH (ni)-[:HAS_STATE]->(nis_src:NetworkInterfaceState)-[:ANOMALOUS_COMMUNICATION]->(:NetworkInterface)
        WHERE nis_src.v_from <= $wm AND (nis_src.v_to IS NULL OR $wm < nis_src.v_to)
 
        OPTIONAL MATCH (:NetworkInterface)-[:HAS_STATE]->(nis_dst:NetworkInterfaceState)-[:ANOMALOUS_COMMUNICATION]->(ni)
        WHERE nis_dst.v_from <= $wm AND (nis_dst.v_to IS NULL OR $wm < nis_dst.v_to)
 
        WITH rd, ds, nis_src, nis_dst
        WHERE coalesce(ds.is_compromised, false) = true
           OR coalesce(rd.is_quarantined, false) = true
           OR coalesce(ds.is_powered, true)      = false
           OR nis_src IS NOT NULL
           OR nis_dst IS NOT NULL
 
        RETURN collect(DISTINCT id(rd)) AS device_ids
    `

	res, err := tx.RunCypher(ctx, query, map[string]any{"wm": wm})
	if err != nil {
		return fmt.Errorf("querying affected devices: %w", err)
	}

	if len(res.Records) == 0 {
		return nil
	}

	ids := toInt64Slice(res.Records[0].Values[0])
	ps.blastDevices.Append(ids...)
	ps.visited.Append(ids...)

	return nil
}

func (r *graphRepository) blastRadius(ctx context.Context, tx memgraphx.QueryExecutor, ps *perimeterState) error {
	const seedQuery = `
        MATCH (s:Service)-[:HOSTED_ON]->(h)
        WHERE id(h) IN $device_ids
          AND NOT (id(s) IN $visited_ids)
        RETURN collect(DISTINCT id(s)) AS srv_ids
    `

	res, err := tx.RunCypher(ctx, seedQuery, map[string]any{
		"device_ids":  ps.blastDevices.ToSlice(),
		"visited_ids": ps.visited.ToSlice(),
	})
	if err != nil {
		return fmt.Errorf("querying hosted services: %w", err)
	}

	if len(res.Records) == 0 {
		return nil
	}

	frontier := mapset.NewThreadUnsafeSet[int64]()
	frontier.Append(toInt64Slice(res.Records[0].Values[0])...)

	ps.blastSrvs.Append(frontier.ToSlice()...)
	ps.visited.Append(frontier.ToSlice()...)

	for !frontier.IsEmpty() {
		frontier, err = r.propagateRadius(ctx, tx, frontier, ps)
		if err != nil {
			return fmt.Errorf("expanding blast radius: %w", err)
		}
	}

	return nil
}

func (r *graphRepository) propagateRadius(
	ctx context.Context,
	tx memgraphx.QueryExecutor,
	frontier mapset.Set[int64],
	ps *perimeterState,
) (mapset.Set[int64], error) {
	const query = `
        MATCH (s:Service)
        WHERE id(s) IN $frontier_srv_ids
 
        OPTIONAL MATCH (direct_cap:Capability)-[:IS_PROVIDED]->(s)
 
        OPTIONAL MATCH (child_cap:Capability)-[:PART_OF]->(root_cap:Capability)-[:IS_PROVIDED]->(s)
        WHERE (child_cap)-[:CAN_BE_PROVIDED]->(s)
 
        WITH
            collect(DISTINCT CASE WHEN direct_cap IS NOT NULL THEN id(direct_cap) END) +
            collect(DISTINCT CASE WHEN child_cap  IS NOT NULL THEN id(child_cap)  END) +
            collect(DISTINCT CASE WHEN root_cap   IS NOT NULL THEN id(root_cap)   END) AS cap_ids
 
        OPTIONAL MATCH (s_next:Service)-[:REQUIRES]->(c:Capability)
        WHERE id(c) IN cap_ids
          AND NOT (id(s_next) IN $visited_ids)
 
        RETURN
            cap_ids                              AS cap_ids,
            collect(DISTINCT id(s_next))         AS next_srv_ids
    `

	res, err := tx.RunCypher(ctx, query, map[string]any{
		"frontier_srv_ids": frontier.ToSlice(),
		"visited_ids":      ps.visited.ToSlice(),
	})
	if err != nil {
		return nil, fmt.Errorf("querying blast radius hop: %w", err)
	}

	next := mapset.NewThreadUnsafeSet[int64]()

	if len(res.Records) == 0 {
		return next, nil
	}

	vals := res.Records[0].Values
	capIDs := toInt64Slice(vals[0])
	nextSrvIDs := toInt64Slice(vals[1])

	for _, id := range capIDs {
		if !ps.visited.Contains(id) {
			ps.blastCaps.Add(id)
		}
	}
	ps.visited.Append(capIDs...)

	next.Append(nextSrvIDs...)
	ps.blastSrvs.Append(nextSrvIDs...)
	ps.visited.Append(nextSrvIDs...)

	return next, nil
}

func (r *graphRepository) alternativeProviders(ctx context.Context, tx memgraphx.QueryExecutor, ps *perimeterState) error {
	if ps.blastCaps.IsEmpty() {
		return nil
	}

	const query = `
        MATCH (r:Capability:Root)
        WHERE id(r) IN $blast_cap_ids
 
        OPTIONAL MATCH (ch:Capability)-[:PART_OF]->(r)
        WHERE NOT (id(ch) IN $visited_ids)
 
        OPTIONAL MATCH (ch)-[:CAN_BE_PROVIDED]->(s_via_child:Service)
        WHERE ch IS NOT NULL
          AND NOT (id(s_via_child) IN $visited_ids)
 
        OPTIONAL MATCH (r)-[:CAN_BE_PROVIDED]->(s_direct:Service)
        WHERE NOT (id(s_direct) IN $visited_ids)
 
        RETURN
            collect(DISTINCT CASE WHEN ch       IS NOT NULL THEN id(ch)       END) AS child_cap_ids,
            collect(DISTINCT CASE WHEN s_via_child IS NOT NULL THEN id(s_via_child) END) +
            collect(DISTINCT CASE WHEN s_direct    IS NOT NULL THEN id(s_direct)    END) AS alt_srv_ids
    `

	res, err := tx.RunCypher(ctx, query, map[string]any{
		"blast_cap_ids": ps.blastCaps.ToSlice(),
		"visited_ids":   ps.visited.ToSlice(),
	})
	if err != nil {
		return fmt.Errorf("querying alternative providers: %w", err)
	}

	if len(res.Records) == 0 {
		return nil
	}

	vals := res.Records[0].Values
	childCapIDs := toInt64Slice(vals[0])
	altSrvIDs := toInt64Slice(vals[1])

	ps.altCaps.Append(childCapIDs...)
	ps.visited.Append(childCapIDs...)

	for _, id := range altSrvIDs {
		if !ps.visited.Contains(id) {
			ps.altSrvs.Add(id)
			ps.frontierSrvs.Add(id)
		}
	}
	ps.visited.Append(altSrvIDs...)

	return nil
}

func (r *graphRepository) dependencyClosure(ctx context.Context, tx memgraphx.QueryExecutor, ps *perimeterState) error {
	frontier := ps.frontierSrvs.Union(ps.blastSrvs)
	var err error

	for !frontier.IsEmpty() {
		frontier, err = r.propagateClosure(ctx, tx, frontier, ps)
		if err != nil {
			return fmt.Errorf("expanding dependency closure: %w", err)
		}
	}

	return nil
}

func (r *graphRepository) propagateClosure(
	ctx context.Context,
	tx memgraphx.QueryExecutor,
	frontier mapset.Set[int64],
	ps *perimeterState,
) (mapset.Set[int64], error) {
	const query = `
        MATCH (:Ship)-[:CURRENT_CONTEXT]->(ctx:Context)
 
        MATCH (s:Service)
        WHERE id(s) IN $frontier_srv_ids
 
        OPTIONAL MATCH (s)-[:REQUIRES]->(cap:Capability)
        WHERE NOT (id(cap) IN $visited_ids)
 
        OPTIONAL MATCH (cap)-[:PART_OF]->(root:Capability)
 
        OPTIONAL MATCH (cap)-[cbp:CAN_BE_PROVIDED]->(alt_srv:Service)
        WHERE cap IS NOT NULL
          AND cbp.context_status IS NOT NULL
          AND coalesce(cbp.context_status[ctx.name], -1) >= 0
          AND NOT (id(alt_srv) IN $visited_ids)
 
        OPTIONAL MATCH (cap)-[:IS_PROVIDED]->(cap_current_srv:Service)
        WHERE cap IS NOT NULL
          AND NOT (id(cap_current_srv) IN $visited_ids)
 
        OPTIONAL MATCH (root)-[:IS_PROVIDED]->(root_current_srv:Service)
        WHERE root IS NOT NULL
          AND NOT (id(root_current_srv) IN $visited_ids)
 
        RETURN
            collect(DISTINCT CASE WHEN cap              IS NOT NULL THEN id(cap)              END) +
            collect(DISTINCT CASE WHEN root             IS NOT NULL THEN id(root)             END) AS cap_ids,
            collect(DISTINCT CASE WHEN alt_srv          IS NOT NULL THEN id(alt_srv)          END) +
            collect(DISTINCT CASE WHEN cap_current_srv  IS NOT NULL THEN id(cap_current_srv)  END) +
            collect(DISTINCT CASE WHEN root_current_srv IS NOT NULL THEN id(root_current_srv) END) AS next_srv_ids
    `

	res, err := tx.RunCypher(ctx, query, map[string]any{
		"frontier_srv_ids": frontier.ToSlice(),
		"visited_ids":      ps.visited.ToSlice(),
	})
	if err != nil {
		return nil, fmt.Errorf("querying closure hop: %w", err)
	}

	next := mapset.NewThreadUnsafeSet[int64]()

	if len(res.Records) == 0 {
		return next, nil
	}

	vals := res.Records[0].Values
	capIDs := toInt64Slice(vals[0])
	nextSrvIDs := toInt64Slice(vals[1])

	ps.altCaps.Append(capIDs...)
	ps.visited.Append(capIDs...)

	for _, id := range nextSrvIDs {
		if !ps.visited.Contains(id) {
			next.Add(id)
			ps.altSrvs.Add(id)
		}
	}
	ps.visited.Append(nextSrvIDs...)

	return next, nil
}

func (r *graphRepository) alternativeHosts(ctx context.Context, tx memgraphx.QueryExecutor, ps *perimeterState) error {
	allSrvs := ps.blastSrvs.Union(ps.altSrvs)
	if allSrvs.IsEmpty() {
		return nil
	}

	const query = `
        MATCH (h:Resource)-[:CAN_HOST]->(s:Service)
        WHERE id(s) IN $srv_ids
          AND NOT (id(h) IN $visited_ids)
 
        RETURN
            collect(DISTINCT CASE WHEN 'Digital' IN labels(h) THEN id(h) END) AS digital_ids,
            collect(DISTINCT CASE WHEN 'Analog'  IN labels(h) THEN id(h) END) AS analog_ids
    `

	res, err := tx.RunCypher(ctx, query, map[string]any{
		"srv_ids":     allSrvs.ToSlice(),
		"visited_ids": ps.visited.ToSlice(),
	})
	if err != nil {
		return fmt.Errorf("querying alternative hosts: %w", err)
	}

	if len(res.Records) == 0 {
		return nil
	}

	vals := res.Records[0].Values
	digitalIDs := toInt64Slice(vals[0])
	analogIDs := toInt64Slice(vals[1])

	ps.hostDigitals.Append(digitalIDs...)
	ps.hostAnalogs.Append(analogIDs...)
	ps.visited.Append(digitalIDs...)
	ps.visited.Append(analogIDs...)

	return nil
}

func toInt64Slice(raw any) []int64 {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(items))
	for _, v := range items {
		if v == nil {
			continue
		}
		if id, ok := v.(int64); ok {
			out = append(out, id)
		}
	}
	return out
}
