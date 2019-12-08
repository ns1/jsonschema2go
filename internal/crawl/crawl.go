package crawl

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/pkg/ctxflags"
	gen "github.com/jwilner/jsonschema2go/pkg/generate"
	"github.com/jwilner/jsonschema2go/pkg/schema"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
)

type CrawlResult struct {
	Plan gen.Plan
	Err  error
}

func Crawl(
	ctx context.Context,
	planner gen.Planner,
	loader schema.Loader,
	typer planning.Typer,
	fileNames []string,
) (map[string][]gen.Plan, error) {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	loaded, errC := initialLoad(ctx, &childRoutines, loader, fileNames)

	results := crawl(ctx, planner, loader, typer, loaded)

	return group(ctx, results, errC)
}

func initialLoad(
	ctx context.Context,
	wg *sync.WaitGroup,
	loader schema.Loader,
	fileNames []string,
) (<-chan *schema.Schema, <-chan error) {
	// load all initial schemas concurrently
	loaded := make(chan *schema.Schema)
	errC := make(chan error, 1)
	var sent int64 // used to track completion of tasks
	for _, fileName := range fileNames {
		wg.Add(1)
		go func(fileName string) {
			defer wg.Done()

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
	return loaded, errC
}

func crawl(
	ctx context.Context,
	planner gen.Planner,
	loader schema.Loader,
	typer planning.Typer,
	schemas <-chan *schema.Schema,
) <-chan CrawlResult {
	helper := planning.NewHelper(ctx, loader, typer, schemas)

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

		derivePlan := func(s *schema.Schema) {
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

		seen := make(map[gen.TypeInfo]bool)
		for {
			if allCopied && inFlight == 0 {
				return
			}

			select {
			case s := <-helper.Schemas():
				t := helper.TypeInfo(s)
				if seen[t] {
					if ctxflags.IsDebug(ctx) {
						log.Printf("crawler: skipping %v -- already seen", t)
					}
					continue
				}
				seen[t] = true

				if s.Config.Exclude {
					if ctxflags.IsDebug(ctx) {
						log.Printf("crawler: excluding %v by request", t)
					}
					continue
				}

				if ctxflags.IsDebug(ctx) {
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

func group(ctx context.Context, results <-chan CrawlResult, errC <-chan error) (map[string][]gen.Plan, error) {
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
