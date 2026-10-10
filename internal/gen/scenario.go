package gen

import (
	"fmt"
	"strings"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

type ScenarioSpanNode struct {
	ServiceName string
	Name        string
	Kind        tracev1.Span_SpanKind
	Children    []*ScenarioSpanNode
	MinDuration time.Duration
	MaxDuration time.Duration
	IsError     bool
	Attributes  []*commonv1.KeyValue
}

type budget struct {
	remaining int
}

func (b *budget) take(n int) bool {
	if b.remaining < n {
		return false
	}
	b.remaining -= n
	return true
}

// BuiltInScenarioNames lists dashboard/CLI scenarios. Tree shapes plus the
// special "workload" selector for flat evidence grammar (see IsWorkloadScenario).
func BuiltInScenarioNames() []string {
	return []string{"microservice", "monolith", "3-tier-web-app", "event-driven", "workload"}
}

// IsWorkloadScenario reports whether name selects flat evidence grammar
// (workload.span) instead of a service-graph span tree.
func IsWorkloadScenario(scenario string) bool {
	return strings.EqualFold(strings.TrimSpace(scenario), "workload")
}

func GenerateScenario(scenario string, rng *SeededRNG, spanCount, maxDepth int) ([]*ScenarioSpanNode, error) {
	if spanCount < 1 {
		spanCount = 1
	}
	if maxDepth < 0 {
		maxDepth = 0
	}

	switch strings.ToLower(strings.TrimSpace(scenario)) {
	case "microservice":
		return buildMicroservice(rng, spanCount, maxDepth), nil
	case "monolith":
		return buildMonolith(rng, spanCount, maxDepth), nil
	case "3-tier-web-app":
		return buildThreeTier(rng, spanCount, maxDepth), nil
	case "event-driven":
		return buildEventDriven(rng, spanCount, maxDepth), nil
	case "workload":
		return nil, fmt.Errorf("scenario %q selects flat evidence grammar (workload.span), not a span tree; set Config.Scenario=workload or Workload.Shape=flat", scenario)
	default:
		return nil, fmt.Errorf("unknown scenario %q, expected one of: %s", scenario, strings.Join(BuiltInScenarioNames(), ", "))
	}
}

func CountScenarioSpans(roots []*ScenarioSpanNode) int {
	total := 0
	for _, r := range roots {
		total += countScenarioNode(r)
	}
	return total
}

func countScenarioNode(n *ScenarioSpanNode) int {
	if n == nil {
		return 0
	}
	total := 1
	for _, c := range n.Children {
		total += countScenarioNode(c)
	}
	return total
}

func markScenarioErrors(roots []*ScenarioSpanNode, rng *SeededRNG) {
	var all []*ScenarioSpanNode
	var walk func(*ScenarioSpanNode)
	walk = func(n *ScenarioSpanNode) {
		all = append(all, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}

	n := len(all)
	if n == 0 {
		return
	}
	errors := n / 10
	if errors == 0 {
		errors = 1
	}
	for i := 0; i < errors; i++ {
		j, _ := rng.IntRange(i, n-1)
		all[i], all[j] = all[j], all[i]
		all[i].IsError = true
	}
}

// ---------------------------------------------------------------------------
// Microservice scenario
// ---------------------------------------------------------------------------

func buildMicroservice(rng *SeededRNG, spanCount, maxDepth int) []*ScenarioSpanNode {
	b := &budget{remaining: spanCount}

	gateway := &ScenarioSpanNode{
		ServiceName: "api-gateway",
		Name:        "GET /api/orders",
		Kind:        tracev1.Span_SPAN_KIND_SERVER,
		MinDuration: 10 * time.Millisecond,
		MaxDuration: 50 * time.Millisecond,
	}
	b.remaining--

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		gateway.Children = append(gateway.Children, &ScenarioSpanNode{
			ServiceName: "api-gateway",
			Name:        "rate.limit.check",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 5 * time.Millisecond,
		})
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		userSvc := &ScenarioSpanNode{
			ServiceName: "api-gateway",
			Name:        "user.v1.ValidateToken",
			Kind:        tracev1.Span_SPAN_KIND_CLIENT,
			MinDuration: 20 * time.Millisecond,
			MaxDuration: 100 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			userSvc.Children = append(userSvc.Children, &ScenarioSpanNode{
				ServiceName: "user-service",
				Name:        "token.decode",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		gateway.Children = append(gateway.Children, userSvc)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		orderSvc := &ScenarioSpanNode{
			ServiceName: "api-gateway",
			Name:        "order.v1.ListOrders",
			Kind:        tracev1.Span_SPAN_KIND_CLIENT,
			MinDuration: 30 * time.Millisecond,
			MaxDuration: 200 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			orderSvc.Children = append(orderSvc.Children, &ScenarioSpanNode{
				ServiceName: "order-service",
				Name:        "parse.pagination",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			dbSpan := &ScenarioSpanNode{
				ServiceName: "order-service",
				Name:        "pg.query",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 5 * time.Millisecond,
				MaxDuration: 100 * time.Millisecond,
			}
			if b.take(1) && (maxDepth == 0 || 3 < maxDepth) {
				dbSpan.Children = append(dbSpan.Children, &ScenarioSpanNode{
					ServiceName: "database",
					Name:        "index.scan",
					Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
					MinDuration: 1 * time.Millisecond,
					MaxDuration: 5 * time.Millisecond,
				})
			}
			orderSvc.Children = append(orderSvc.Children, dbSpan)
		}
		gateway.Children = append(gateway.Children, orderSvc)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		pub := &ScenarioSpanNode{
			ServiceName: "api-gateway",
			Name:        "publish order.created",
			Kind:        tracev1.Span_SPAN_KIND_PRODUCER,
			MinDuration: 5 * time.Millisecond,
			MaxDuration: 20 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			pub.Children = append(pub.Children, &ScenarioSpanNode{
				ServiceName: "notification-service",
				Name:        "serialize.payload",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		gateway.Children = append(gateway.Children, pub)
	}

	roots := []*ScenarioSpanNode{gateway}

	if b.remaining > 0 {
		_ = b.take(1)
		consumer := &ScenarioSpanNode{
			ServiceName: "notification-service",
			Name:        "consume order.created",
			Kind:        tracev1.Span_SPAN_KIND_CONSUMER,
			MinDuration: 50 * time.Millisecond,
			MaxDuration: 500 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "notification-service",
				Name:        "deserialize.event",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "notification-service",
				Name:        "smtp.send",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 100 * time.Millisecond,
				MaxDuration: 1000 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "notification-service",
				Name:        "api.update.inventory",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 20 * time.Millisecond,
				MaxDuration: 100 * time.Millisecond,
			})
		}
		roots = append(roots, consumer)
	}

	markScenarioErrors(roots, rng)
	return roots
}

// ---------------------------------------------------------------------------
// Monolith scenario
// ---------------------------------------------------------------------------

func buildMonolith(rng *SeededRNG, spanCount, maxDepth int) []*ScenarioSpanNode {
	b := &budget{remaining: spanCount}

	root := &ScenarioSpanNode{
		ServiceName: "monolith",
		Name:        "POST /checkout",
		Kind:        tracev1.Span_SPAN_KIND_SERVER,
		MinDuration: 10 * time.Millisecond,
		MaxDuration: 50 * time.Millisecond,
	}
	b.remaining--

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		auth := &ScenarioSpanNode{
			ServiceName: "monolith",
			Name:        "auth.middleware",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 5 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			auth.Children = append(auth.Children, &ScenarioSpanNode{
				ServiceName: "monolith",
				Name:        "redis.session.get",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		root.Children = append(root.Children, auth)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		cart := &ScenarioSpanNode{
			ServiceName: "monolith",
			Name:        "cart.load",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 10 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			cart.Children = append(cart.Children, &ScenarioSpanNode{
				ServiceName: "monolith",
				Name:        "redis.get.cart",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		root.Children = append(root.Children, cart)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		payment := &ScenarioSpanNode{
			ServiceName: "monolith",
			Name:        "payment.process",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 5 * time.Millisecond,
			MaxDuration: 20 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			payment.Children = append(payment.Children, &ScenarioSpanNode{
				ServiceName: "monolith",
				Name:        "stripe.charge",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 200 * time.Millisecond,
				MaxDuration: 1000 * time.Millisecond,
			})
		}
		root.Children = append(root.Children, payment)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		persist := &ScenarioSpanNode{
			ServiceName: "monolith",
			Name:        "order.persist",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 5 * time.Millisecond,
			MaxDuration: 20 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			persist.Children = append(persist.Children, &ScenarioSpanNode{
				ServiceName: "monolith",
				Name:        "db.batch_insert",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 10 * time.Millisecond,
				MaxDuration: 100 * time.Millisecond,
			})
		}
		root.Children = append(root.Children, persist)
	}

	for b.remaining > 0 {
		if !b.take(1) {
			break
		}
		root.Children = append(root.Children, &ScenarioSpanNode{
			ServiceName: "monolith",
			Name:        fmt.Sprintf("parallel.worker.%d", len(root.Children)),
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 5 * time.Millisecond,
			MaxDuration: 50 * time.Millisecond,
		})
	}

	roots := []*ScenarioSpanNode{root}
	markScenarioErrors(roots, rng)
	return roots
}

// ---------------------------------------------------------------------------
// 3-Tier Web App scenario
// ---------------------------------------------------------------------------

func buildThreeTier(rng *SeededRNG, spanCount, maxDepth int) []*ScenarioSpanNode {
	b := &budget{remaining: spanCount}

	web := &ScenarioSpanNode{
		ServiceName: "web-server",
		Name:        "GET /users",
		Kind:        tracev1.Span_SPAN_KIND_SERVER,
		MinDuration: 10 * time.Millisecond,
		MaxDuration: 30 * time.Millisecond,
	}
	b.remaining--

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		web.Children = append(web.Children, &ScenarioSpanNode{
			ServiceName: "web-server",
			Name:        "rate.limit.check",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 5 * time.Millisecond,
		})
	}
	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		web.Children = append(web.Children, &ScenarioSpanNode{
			ServiceName: "web-server",
			Name:        "parse.request",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 3 * time.Millisecond,
		})
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		app := &ScenarioSpanNode{
			ServiceName: "web-server",
			Name:        "user.list",
			Kind:        tracev1.Span_SPAN_KIND_CLIENT,
			MinDuration: 20 * time.Millisecond,
			MaxDuration: 100 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			app.Children = append(app.Children, &ScenarioSpanNode{
				ServiceName: "app-server",
				Name:        "validate.params",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			db := &ScenarioSpanNode{
				ServiceName: "app-server",
				Name:        "SELECT * FROM users",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 5 * time.Millisecond,
				MaxDuration: 50 * time.Millisecond,
			}
			if b.take(1) && (maxDepth == 0 || 3 < maxDepth) {
				db.Children = append(db.Children, &ScenarioSpanNode{
					ServiceName: "database",
					Name:        "index.scan",
					Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
					MinDuration: 1 * time.Millisecond,
					MaxDuration: 5 * time.Millisecond,
				})
			}
			app.Children = append(app.Children, db)
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			app.Children = append(app.Children, &ScenarioSpanNode{
				ServiceName: "app-server",
				Name:        "render.template",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 5 * time.Millisecond,
				MaxDuration: 20 * time.Millisecond,
			})
		}
		web.Children = append(web.Children, app)
	}

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		web2 := &ScenarioSpanNode{
			ServiceName: "web-server",
			Name:        "POST /users",
			Kind:        tracev1.Span_SPAN_KIND_SERVER,
			MinDuration: 10 * time.Millisecond,
			MaxDuration: 30 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 2 < maxDepth) {
			web2.Children = append(web2.Children, &ScenarioSpanNode{
				ServiceName: "web-server",
				Name:        "user.create",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 20 * time.Millisecond,
				MaxDuration: 100 * time.Millisecond,
			})
		}
		if len(web2.Children) > 0 && b.take(1) && (maxDepth == 0 || 3 < maxDepth) {
			web2.Children[0].Children = append(
				web2.Children[0].Children,
				&ScenarioSpanNode{
					ServiceName: "app-server",
					Name:        "INSERT INTO users",
					Kind:        tracev1.Span_SPAN_KIND_CLIENT,
					MinDuration: 5 * time.Millisecond,
					MaxDuration: 50 * time.Millisecond,
				},
			)
		}
		web.Children = append(web.Children, web2)
	}

	roots := []*ScenarioSpanNode{web}
	markScenarioErrors(roots, rng)
	return roots
}

// ---------------------------------------------------------------------------
// Event-Driven scenario
// ---------------------------------------------------------------------------

func buildEventDriven(rng *SeededRNG, spanCount, maxDepth int) []*ScenarioSpanNode {
	b := &budget{remaining: spanCount}

	pub := &ScenarioSpanNode{
		ServiceName: "publisher",
		Name:        "publish order.created",
		Kind:        tracev1.Span_SPAN_KIND_PRODUCER,
		MinDuration: 5 * time.Millisecond,
		MaxDuration: 20 * time.Millisecond,
	}
	b.remaining--

	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		pub.Children = append(pub.Children, &ScenarioSpanNode{
			ServiceName: "publisher",
			Name:        "validate.order",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 5 * time.Millisecond,
		})
	}
	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		pub.Children = append(pub.Children, &ScenarioSpanNode{
			ServiceName: "publisher",
			Name:        "serialize.payload",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 5 * time.Millisecond,
		})
	}
	if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
		pub.Children = append(pub.Children, &ScenarioSpanNode{
			ServiceName: "publisher",
			Name:        "produce.message",
			Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
			MinDuration: 2 * time.Millisecond,
			MaxDuration: 10 * time.Millisecond,
		})
	}

	var broker *ScenarioSpanNode
	if b.take(1) && (maxDepth == 0 || 0 < maxDepth) {
		broker = &ScenarioSpanNode{
			ServiceName: "message-broker",
			Name:        "broker.route.message",
			Kind:        tracev1.Span_SPAN_KIND_CONSUMER,
			MinDuration: 1 * time.Millisecond,
			MaxDuration: 10 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			broker.Children = append(broker.Children, &ScenarioSpanNode{
				ServiceName: "message-broker",
				Name:        "topic.match",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 3 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			broker.Children = append(broker.Children, &ScenarioSpanNode{
				ServiceName: "message-broker",
				Name:        "fanout.partitions",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 3 * time.Millisecond,
			})
		}
	}

	var consumer *ScenarioSpanNode
	if b.take(1) && (maxDepth == 0 || 0 < maxDepth) {
		consumer = &ScenarioSpanNode{
			ServiceName: "consumer",
			Name:        "consume order.created",
			Kind:        tracev1.Span_SPAN_KIND_CONSUMER,
			MinDuration: 50 * time.Millisecond,
			MaxDuration: 500 * time.Millisecond,
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "consumer",
				Name:        "deserialize.event",
				Kind:        tracev1.Span_SPAN_KIND_INTERNAL,
				MinDuration: 1 * time.Millisecond,
				MaxDuration: 5 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "consumer",
				Name:        "smtp.send",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 100 * time.Millisecond,
				MaxDuration: 1000 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "consumer",
				Name:        "api.update.inventory",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 20 * time.Millisecond,
				MaxDuration: 100 * time.Millisecond,
			})
		}
		if b.take(1) && (maxDepth == 0 || 1 < maxDepth) {
			consumer.Children = append(consumer.Children, &ScenarioSpanNode{
				ServiceName: "consumer",
				Name:        "kafka.commit",
				Kind:        tracev1.Span_SPAN_KIND_CLIENT,
				MinDuration: 5 * time.Millisecond,
				MaxDuration: 20 * time.Millisecond,
			})
		}
	}

	roots := []*ScenarioSpanNode{pub}
	if broker != nil {
		roots = append(roots, broker)
	}
	if consumer != nil {
		roots = append(roots, consumer)
	}
	markScenarioErrors(roots, rng)
	return roots
}
