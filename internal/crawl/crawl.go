package crawl

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/planning"
	gen "github.com/jwilner/jsonschema2go/pkg/gen"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
)

// Crawl traverses a set of JSON Schemas, lazily loading their children in concurrent goroutines as need be.
func Crawl(
	ctx context.Context,
	planner gen.Planner,
	loader gen.Loader,
	typer planning.Typer,
	uris []string,
) (map[string][]gen.Plan, error) {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	loaded, errC := initialLoad(ctx, &childRoutines, loader, uris)

	results := crawl(ctx, planner, loader, typer, loaded)

	return group(ctx, results, errC)
}

func initialLoad(
	ctx context.Context,
	wg *sync.WaitGroup,
	loader gen.Loader,
	uris []string,
) (<-chan *gen.Schema, <-chan error) {
	// load all initial schemas concurrently
	loaded := make(chan *gen.Schema)
	errC := make(chan error, 1)
	var sent int64 // used to track completion of tasks
	for _, uri := range uris {
		wg.Add(1)
		go func(uri string) {
			defer wg.Done()

			u, err := url.Parse(uri)
			if err != nil {
				errC <- err
				return
			}

			schema, err := loader.Load(ctx, u)
			if err != nil {
				errC <- fmt.Errorf("unable to resolve schema from %q: %w", uri, err)
				return
			}
			select {
			case <-ctx.Done():
			case loaded <- schema:
				if atomic.AddInt64(&sent, 1) == int64(len(uris)) {
					close(loaded)
				}
			}
		}(uri)
	}
	return loaded, errC
}

func crawl(
	ctx context.Context,
	planner gen.Planner,
	loader gen.Loader,
	typer planning.Typer,
	schemas <-chan *gen.Schema,
) <-chan crawlResult {
	helper := planning.NewHelper(ctx, loader, typer, schemas)

	results := make(chan crawlResult)

	go func() {
		defer close(results)

		var (
			wg           sync.WaitGroup
			allCopied    bool
			inFlight     int
			innerResults = make(chan crawlResult)
		)

		defer wg.Wait()

		derivePlan := func(s *gen.Schema) {
			plan, err := planner.Plan(ctx, helper, s)
			select {
			case innerResults <- crawlResult{plan, err}:
			case <-ctx.Done():
				return
			}
		}

		forward := func(result crawlResult) {
			select {
			case <-ctx.Done():
			case results <- result:
			}
		}

		seen := make(map[gen.TypeInfo]bool)
		for {
			if allCopied && inFlight == 0 {
				return
			}

			select {
			case s := <-helper.Schemas():
				t := helper.TypeInfo(s)
				if seen[t] {
					if gen.IsDebug(ctx) {
						log.Printf("crawler: skipping %v -- already seen", t)
					}
					continue
				}
				seen[t] = true

				if s.Config.Exclude {
					if gen.IsDebug(ctx) {
						log.Printf("crawler: excluding %v by request", t)
					}
					continue
				}

				if gen.IsDebug(ctx) {
					log.Printf("crawler: planning %v", t)
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

func group(ctx context.Context, results <-chan crawlResult, errC <-chan error) (map[string][]gen.Plan, error) {
	// group together results
	grouped := make(map[string][]gen.Plan)
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
			if result.Plan == nil {
				continue
			}
			plan := result.Plan
			goPath := plan.Type().GoPath
			grouped[goPath] = append(grouped[goPath], plan)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

type crawlResult struct {
	Plan gen.Plan
	Err  error
}
