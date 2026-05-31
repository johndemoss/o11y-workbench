package gen

import (
	"fmt"
	"strings"
)

// TopologyType defines the shape of the trace tree that GenerateTopology produces.
type TopologyType int

const (
	TopologyFlat TopologyType = iota
	TopologyBranching
	TopologySplit
)

// String returns the lowercase name of the topology type.
func (t TopologyType) String() string {
	switch t {
	case TopologyFlat:
		return "flat"
	case TopologyBranching:
		return "branching"
	case TopologySplit:
		return "split"
	default:
		return "unknown"
	}
}

// ParseTopology converts a lowercase string to a TopologyType, returning an error
// for unrecognized values.
func ParseTopology(s string) (TopologyType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "flat":
		return TopologyFlat, nil
	case "branching":
		return TopologyBranching, nil
	case "split":
		return TopologySplit, nil
	default:
		return 0, fmt.Errorf("unknown topology type %q, expected one of: flat, branching, split", s)
	}
}

// SpanNode describes one span in the trace tree. The Generator converts these to
// tracev1.Span.
type SpanNode struct {
	Name     string
	Depth    int
	Children []*SpanNode
	IsError  bool
}

var namingPrefixes = []string{
	"root",
	"handler",
	"middleware",
	"db.query",
	"external.call",
	"cache.lookup",
	"rpc.call",
}

// GenerateTopology creates a tree of SpanNodes rooted at the given topology type,
// using rng for all structural decisions. The returned root's subtree contains
// approximately spanCount spans total.
func GenerateTopology(t TopologyType, rng *SeededRNG, spanCount int, maxDepth int) (*SpanNode, error) {
	return GenerateTopologyWithErrors(t, rng, spanCount, maxDepth, 10)
}

// GenerateTopologyWithErrors creates a tree and marks errors based on errorPercent.
// errorPercent: 0 = no errors, 10 = ~10% errors (legacy default). Minimum 1 error if > 0.
func GenerateTopologyWithErrors(t TopologyType, rng *SeededRNG, spanCount int, maxDepth int, errorPercent int) (*SpanNode, error) {
	if spanCount < 1 {
		spanCount = 1
	}
	if maxDepth < 0 {
		maxDepth = 0
	}

	root := &SpanNode{
		Name:  "root",
		Depth: 0,
	}

	switch t {
	case TopologyFlat:
		root.Children = genFlat(rng, spanCount-1)

	case TopologyBranching:
		children, err := genBranch(rng, spanCount-1, maxDepth)
		if err != nil {
			return nil, err
		}
		root.Children = children

	case TopologySplit:
		children, err := genSplit(rng, spanCount-1, maxDepth)
		if err != nil {
			return nil, err
		}
		root.Children = children
	}

	if errorPercent > 0 {
		markRandomErrors(root, rng, errorPercent)
	}
	return root, nil
}

func genFlat(_ *SeededRNG, count int) []*SpanNode {
	if count <= 0 {
		return nil
	}
	children := make([]*SpanNode, count)
	for i := 0; i < count; i++ {
		children[i] = &SpanNode{
			Name:  fmt.Sprintf("handler.%d", i),
			Depth: 1,
		}
	}
	return children
}

func genBranch(rng *SeededRNG, budget, maxDepth int) ([]*SpanNode, error) {
	if budget <= 0 {
		return nil, nil
	}

	type item struct {
		node  *SpanNode
		alloc int
		depth int
	}

	nRoots, err := rng.IntRange(2, 4)
	if err != nil {
		return nil, err
	}
	if nRoots > budget {
		nRoots = budget
	}
	if nRoots == 0 {
		nRoots = 1
	}

	per := budget / nRoots
	roots := make([]*SpanNode, 0, nRoots)
	q := make([]item, 0, budget)
	var nodeIdx int

	for i := 0; i < nRoots; i++ {
		a := per + boolToInt(i < budget%per)
		n := &SpanNode{
			Name:  fmt.Sprintf("handler.%d", nodeIdx),
			Depth: 1,
		}
		nodeIdx++
		roots = append(roots, n)
		q = append(q, item{node: n, alloc: a, depth: 1})
	}

	for qi := 0; qi < len(q); qi++ {
		cur := &q[qi]
		if cur.alloc <= 1 || (maxDepth > 0 && cur.depth >= maxDepth) {
			continue
		}

		available := cur.alloc - 1
		nCh, err := rng.IntRange(2, 4)
		if err != nil {
			return nil, err
		}
		if nCh > available {
			nCh = available
		}
		if nCh == 0 {
			continue
		}

		perCh := available / nCh
		cur.node.Children = make([]*SpanNode, nCh)
		for i := 0; i < nCh; i++ {
			a := perCh + boolToInt(i < available%nCh)
			if a < 1 {
				a = 1
			}
			nameIdx := cur.depth % len(namingPrefixes)
			child := &SpanNode{
				Name:  fmt.Sprintf("%s.%d", namingPrefixes[nameIdx], nodeIdx),
				Depth: cur.depth + 1,
			}
			nodeIdx++
			cur.node.Children[i] = child
			q = append(q, item{node: child, alloc: a, depth: cur.depth + 1})
		}
	}

	return roots, nil
}

func genSplit(rng *SeededRNG, budget, maxDepth int) ([]*SpanNode, error) {
	if budget <= 0 {
		return nil, nil
	}

	nBranches, err := rng.IntRange(2, 3)
	if err != nil {
		return nil, err
	}
	if nBranches > budget {
		nBranches = budget
	}
	if nBranches < 2 {
		nBranches = 2
	}

	roots := make([]*SpanNode, nBranches)
	per := budget / nBranches
	for i := 0; i < nBranches; i++ {
		a := per + boolToInt(i < budget%per)
		child, err := buildSubtree(rng, a, 1, maxDepth)
		if err != nil {
			return nil, err
		}
		roots[i] = child
	}
	return roots, nil
}

func buildSubtree(rng *SeededRNG, alloc, depth, maxDepth int) (*SpanNode, error) {
	if alloc <= 0 {
		return nil, nil
	}

	nameIdx := depth % len(namingPrefixes)
	root := &SpanNode{
		Name:  fmt.Sprintf("%s.%d", namingPrefixes[nameIdx], alloc),
		Depth: depth,
	}

	if alloc <= 1 || (maxDepth > 0 && depth >= maxDepth) {
		return root, nil
	}

	available := alloc - 1
	nCh, err := rng.IntRange(2, 4)
	if err != nil {
		return nil, err
	}
	if nCh > available {
		nCh = available
	}
	if nCh == 0 {
		return root, nil
	}

	per := available / nCh
	root.Children = make([]*SpanNode, nCh)
	for i := 0; i < nCh; i++ {
		a := per + boolToInt(i < available%nCh)
		if a < 1 {
			a = 1
		}
		child, err := buildSubtree(rng, a, depth+1, maxDepth)
		if err != nil {
			return nil, err
		}
		root.Children[i] = child
	}

	return root, nil
}

// markRandomErrors marks pct% of spans as errors via partial Fisher-Yates.
func markRandomErrors(root *SpanNode, rng *SeededRNG, pct int) {
	var all []*SpanNode
	var walk func(*SpanNode)
	walk = func(n *SpanNode) {
		all = append(all, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	n := len(all)
	if n == 0 {
		return
	}
	errors := n * pct / 100
	if errors == 0 && pct > 0 {
		errors = 1
	}

	for i := 0; i < errors; i++ {
		j, _ := rng.IntRange(i, n-1)
		all[i], all[j] = all[j], all[i]
		all[i].IsError = true
	}
}

// CountSpans returns the total number of SpanNode entries in the tree rooted at root.
func CountSpans(root *SpanNode) int {
	if root == nil {
		return 0
	}
	total := 1
	for _, ch := range root.Children {
		total += CountSpans(ch)
	}
	return total
}

// CollectSpans returns all SpanNodes in depth-first pre-order.
func CollectSpans(root *SpanNode) []*SpanNode {
	if root == nil {
		return nil
	}
	out := []*SpanNode{root}
	for _, ch := range root.Children {
		out = append(out, CollectSpans(ch)...)
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
