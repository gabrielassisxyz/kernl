// Package relate provides a compute-on-read relevance scorer for graph nodes.
//
// All weights are provisional constants inherited from llm_wiki and
// have not been tuned against this product's graph shape. Deferring
// actual tuning to P2.1. Normalisation to [0,1] is also deferred and
// will be introduced in that tuning pass.
package relate

// WeightDirectLink is the coefficient for the binary direct-link signal.
const WeightDirectLink = 3.0

// WeightSourceOverlap is the coefficient for shared-provenance overlap.
const WeightSourceOverlap = 4.0

// WeightAdamicAdar is the coefficient for the structural proximity signal.
const WeightAdamicAdar = 1.5

// WeightTypeAffinity is the coefficient for same-type bonus.
const WeightTypeAffinity = 1.0

// CrossTypeAffinity is the value used when two nodes have different types.
const CrossTypeAffinity = 0.25

// provenanceLabels lists the edge labels that indicate shared source nodes.
// The labels disagree on which side of the edge holds the source: derived_from
// points from the derived node to its source (src=derived, dst=source), while
// the others point from the source to the derived node (src=source,
// dst=derived). sourceOverlap and discoverCandidates normalise both directions
// to a common (source, derived) pair before counting, so the CASE in those
// queries must stay in step with this list.
var provenanceLabels = []string{"generated_from", "processed_from", "processed_into", "derived_from"}
