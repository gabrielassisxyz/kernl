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
// These are currently unused because the graph contains no provenance edges,
// but the signal is built in full and will activate when P2.5 writes them.
var provenanceLabels = []string{"generated_from", "processed_from", "processed_into"}
