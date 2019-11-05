package jsonschema2go

import (
	"context"
	"sync"
)

type CrawlResult struct {
	Plan Plan
	Err  error
}

func crawl(ctx context.Context, loader Loader, schemas <-chan *Schema, planner Planner) <-chan CrawlResult {
	results := make(chan CrawlResult)

	go func() {
		defer close(results)

		var (
			wg        sync.WaitGroup
			allCopied bool
			inFlight  int

			// allSchemas represents the merged stream of explicitly requested schemas and their children; it is
			// in essence the queue which powers a breadth-first search of the object graph
			allSchemas = make(chan *Schema)
			// puts all schemas on merged and puts a signal on noMoreComing when no more coming
			noMoreComing = copyAndSignal(ctx, schemas, allSchemas)
			innerResults = make(chan CrawlResult)

			helper = &PlanningHelper{loader, allSchemas}
		)

		defer wg.Wait()

		derivePlan := func(s *Schema) {
			plan, err := planner.Plan(ctx, helper, s)
			select {
			case innerResults <- CrawlResult{plan, err}:
			case <-ctx.Done():
				return
			}
		}

		forward := func(result CrawlResult) {
			select {
			case <-ctx.Done():
			case results <- result:
			}
		}

		seen := make(map[TypeInfo]bool)
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
				wg.Add(1)
				go func() {
					defer wg.Done()
					derivePlan(s)
				}()
			case <-noMoreComing:
				allCopied = true
			case res := <-innerResults:
				if res.Err != nil {
					forward(res)
					return
				}
				inFlight--
				wg.Add(1)
				go func() {
					defer wg.Done()
					forward(res)
				}()
			case <-ctx.Done():
				return
			}
		}
	}()

	return results
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
