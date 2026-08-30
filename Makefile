.PHONY: docs build test testacc rauthy-up rauthy-down print-rauthy-version fmt vet openapi-refresh

# Rauthy version the vendored OpenAPI spec was generated from. Bump together
# with the spec file (see openapi-refresh).
RAUTHY_VERSION ?= 0.36.2
SPEC := internal/client/mock/testdata/rauthy-openapi-$(RAUTHY_VERSION).json

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./... -count=1

# Acceptance tests against a live Rauthy. `make rauthy-up` prints the
# environment they need; CI runs the same script.
#
#   eval "$$(make -s rauthy-up)"
#   make testacc
#   make rauthy-down
testacc:
	TF_ACC=1 go test ./... -count=1 -p 1 -timeout 15m -run TestAcc

rauthy-up:
	@./scripts/rauthy-up.sh $(RAUTHY_VERSION)

rauthy-down:
	@./scripts/rauthy-down.sh

# The single place the pinned Rauthy version lives; CI reads it from here so a
# spec bump cannot leave the acceptance matrix on the old release.
print-rauthy-version:
	@echo $(RAUTHY_VERSION)

docs:
	tfplugindocs generate --provider-name rauthy

fmt:
	go fmt ./...

# Regenerate the vendored OpenAPI spec by booting the Rauthy container locally
# with the Swagger UI enabled and fetching /auth/v1/docs/openapi.json.
#
# LOCAL ONLY, run by hand when bumping RAUTHY_VERSION. CI never runs this and
# has no Docker; it tests against the spec committed to the repo. If the spec
# filename no longer matches the Rauthy version running in an environment, the
# contract tests are stale.
openapi-refresh:
	./scripts/openapi-refresh.sh $(RAUTHY_VERSION) $(SPEC)
