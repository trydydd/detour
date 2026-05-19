BIN := bin/detour
IMAGE := detour:dev

.PHONY: build test run clean snapshot release-check image image-test image-clean image-publish

build:
	go build -o $(BIN) ./cmd/detour

test:
	go test ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/ dist/

snapshot:
	goreleaser build --snapshot --clean

release-check:
	goreleaser check

image:
	docker build -t $(IMAGE) .

image-test: image
	IMAGE=$(IMAGE) SKIP_BUILD=1 bash scripts/test-docker.sh

image-clean:
	docker rmi -f $(IMAGE) 2>/dev/null || true

image-publish:
ifeq ($(PUSH_REGISTRY),)
	@echo "PUSH_REGISTRY not set; image-publish is a no-op"
else
	docker tag $(IMAGE) $(PUSH_REGISTRY):dev
	docker push $(PUSH_REGISTRY):dev
endif
