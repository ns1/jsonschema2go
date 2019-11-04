package jsonschema2go

import (
	"context"
	"errors"
)

func newCrawler() *Crawler {
	return &Crawler{
		planners: []Planner{
			plannerFunc(planAllOfObject),
			plannerFunc(planSimpleObject),
			plannerFunc(planSimpleArray),
			plannerFunc(planEnum),
		},
	}
}

type Crawler struct {
	planners []Planner
}

type CrawlResult struct {
	Plan Plan
	Err  error
}

func copyAndSignal(ctx context.Context, schemas <-chan *Schema, merged chan<- *Schema) <-chan struct{} {
	schemasDone := make(chan struct{})
	go func() {
		for {
			select {
			case s, ok := <-schemas:
				if !ok {
					select {
					case schemasDone <- struct{}{}:
					case <-ctx.Done():
					}
					return
				}
				select {
				case merged <- s:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return schemasDone
}

func (c *Crawler) Plan(ctx context.Context, loader Loader, schemas <-chan *Schema) <-chan CrawlResult {
	results := make(chan CrawlResult)

	go func() {
		defer close(results)

		var (
			allCopied bool
			inFlight  int
			seen      = make(map[TypeInfo]bool)

			// allSchemas represents the merged stream of explicitly requested schemas and their children; it is
			// in essence the queue which powers a breadth-first search of the object graph
			allSchemas = make(chan *Schema)
			// puts all schemas on merged and puts a signal on noMoreComing when no more coming
			noMoreComing = copyAndSignal(ctx, schemas, allSchemas)
			schemasDone  = make(chan struct{}) // one per deps completed
			errs         = make(chan error, 1)

			helper = &PlanningHelper{loader, allSchemas}
		)

		derivePlan := func(s *Schema) {
			plan, err := c.derivePlan(ctx, helper, s)
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
			// publish result
			select {
			case results <- CrawlResult{plan, nil}:
			case <-ctx.Done():
				return
			}

			select {
			case schemasDone <- struct{}{}:
			case <-ctx.Done():
			}
		}

		for {
			if allCopied && inFlight == 0 {
				return
			}

			select {
			case s := <-allSchemas:
				t := helper.TypeInfo(s.Meta())
				if seen[t] {
					continue
				}
				seen[t] = true
				inFlight++
				go derivePlan(s)
			case <-noMoreComing:
				allCopied = true
			case err := <-errs:
				select {
				case results <- CrawlResult{nil, err}:
				case <-ctx.Done():
				}
				return
			case <-ctx.Done():
				return
			case <-schemasDone:
				inFlight--
			}
		}
	}()

	return results
}

func (c *Crawler) derivePlan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	for _, p := range c.planners {
		pl, err := p.Plan(ctx, helper, schema)
		if err != nil {
			return nil, err
		}
		if pl != nil {
			return pl, nil
		}
	}

	return nil, errors.New("unable to plan")
}
