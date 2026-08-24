package planning

import "github.com/gabrielassisxyz/kernl/internal/graph"

// CompanionPartner returns the id of the node's companion - the entity a
// companion note describes, or the note describing an entity - or "" when the
// node has none. It is the exported form of companionPartner, so the search
// handler can collapse a task/companion pair to one autocomplete entry without
// reimplementing the pairing rule.
func CompanionPartner(tx *graph.ReadTx, nodeID, typ string) (string, error) {
	return companionPartner(tx, nodeID, typ)
}
