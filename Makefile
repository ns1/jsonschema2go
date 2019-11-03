.PHONY: test test-cover

test:
	go test ./...

test-cover:
	go test -v -covermode=count -coverprofile=cover.out
