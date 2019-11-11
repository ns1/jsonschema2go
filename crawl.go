package jsonschema2go

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
)

type CrawlResult struct {
	Plan Plan
	Err  error
}

func doCrawl(
	ctx context.Context,
	planner Planner,
	loader Loader,
	typer Typer,
	fileNames []string,
) (map[string][]Plan, error) {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	// load all initial schemas concurrently
	loaded := make(chan *Schema)
	errC := make(chan error, 1)
	var sent int64 // used to track completion of tasks
	for _, fileName := range fileNames {
		childRoutines.Add(1)
		go func(fileName string) {
			defer childRoutines.Done()

			u, err := url.Parse(fileName)
			if err != nil {
				errC <- err
				return
			}

			schema, err := loader.Load(ctx, u)
			if err != nil {
				errC <- fmt.Errorf("unable to resolve schema from %q: %w", fileName, err)
				return
			}
			select {
			case <-ctx.Done():
			case loaded <- schema:
				if atomic.AddInt64(&sent, 1) == int64(len(fileNames)) {
					close(loaded)
				}
			}
		}(fileName)
	}

	results := crawl(ctx, planner, loader, typer, loaded)

	return group(ctx, results, errC)
}

func newPlanningHelper(ctx context.Context, loader Loader, typer Typer, schemas <-chan *Schema) *PlanningHelper {
	// allSchemas represents the merged stream of explicitly requested schemas and their children; it is
	// in essence the queue which powers a breadth-first search of the object graph
	allSchemas := make(chan *Schema)
	// puts all schemas on merged and puts a signal on noMoreComing when no more coming
	noMoreComing := copyAndSignal(ctx, schemas, allSchemas)

	return &PlanningHelper{loader, typer, allSchemas, noMoreComing}
}

func crawl(
	ctx context.Context,
	planner Planner,
	loader Loader,
	typer Typer,
	schemas <-chan *Schema,
) <-chan CrawlResult {
	helper := newPlanningHelper(ctx, loader, typer, schemas)

	results := make(chan CrawlResult)

	go func() {
		defer close(results)

		var (
			wg           sync.WaitGroup
			allCopied    bool
			inFlight     int
			innerResults = make(chan CrawlResult)
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
			case s := <-helper.Schemas():
				t := helper.TypeInfo(s.Meta())
				if seen[t] {
					continue
				}
				seen[t] = true

				if s.Config.Exclude {
					continue
				}

				inFlight++
				wg.Add(1)
				go func() {
					defer wg.Done()
					derivePlan(s)
				}()
			case <-helper.Submitted():
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

func group(ctx context.Context, results <-chan CrawlResult, errC <-chan error) (map[string][]Plan, error) {
	// group together results
	grouped := make(map[string][]Plan)
	for {
		select {
		case err := <-errC:
			return nil, err
		case result, ok := <-results:
			if !ok {
				return grouped, nil
			}
			if result.Err != nil {
				return nil, result.Err
			}
			plan := result.Plan
			goPath := plan.Type().GoPath
			grouped[goPath] = append(grouped[goPath], plan)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
