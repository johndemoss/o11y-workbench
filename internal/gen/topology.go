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
func GenerateTopology(t TopologyType, rng *SeededRNG, spanCount int, maxDepth int) *SpanNode {
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
		root.Children = genBranch(rng, spanCount-1, maxDepth)

	case TopologySplit:
		root.Children = genSplit(rng, spanCount-1, maxDepth)
	}

	markRandomErrors(root, rng)
	return root
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

func genBranch(rng *SeededRNG, budget, maxDepth int) []*SpanNode {
	if budget <= 0 {
		return nil
	}

	type item struct {
		node  *SpanNode
		alloc int
		depth int
	}

	nRoots := rng.IntRange(2, 4)
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
		nCh := rng.IntRange(2, 4)
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

	return roots
}

func genSplit(rng *SeededRNG, budget, maxDepth int) []*SpanNode {
	if budget <= 0 {
		return nil
	}

	nBranches := rng.IntRange(2, 3)
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
		roots[i] = buildSubtree(rng, a, 1, maxDepth)
	}
	return roots
}

func buildSubtree(rng *SeededRNG, alloc, depth, maxDepth int) *SpanNode {
	if alloc <= 0 {
		return nil
	}

	nameIdx := depth % len(namingPrefixes)
	root := &SpanNode{
		Name:  fmt.Sprintf("%s.%d", namingPrefixes[nameIdx], alloc),
		Depth: depth,
	}

	if alloc <= 1 || (maxDepth > 0 && depth >= maxDepth) {
		return root
	}

	available := alloc - 1
	nCh := rng.IntRange(1, 4)
	if nCh > available {
		nCh = available
	}
	if nCh == 0 {
		return root
	}

	per := available / nCh
	root.Children = make([]*SpanNode, nCh)
	for i := 0; i < nCh; i++ {
		a := per + boolToInt(i < available%nCh)
		if a < 1 {
			a = 1
		}
		root.Children[i] = buildSubtree(rng, a, depth+1, maxDepth)
	}

	return root
}

// markRandomErrors marks roughly 10% of spans as errors via partial Fisher-Yates.
func markRandomErrors(root *SpanNode, rng *SeededRNG) {
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
	errors := n / 10
	if errors == 0 {
		errors = 1
	}

	for i := 0; i < errors; i++ {
		j := rng.IntRange(i, n-1)
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
