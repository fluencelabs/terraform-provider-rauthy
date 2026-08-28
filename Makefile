.PHONY: docs build test fmt vet openapi-refresh

# Rauthy version the vendored OpenAPI spec was generated from. Bump together
# with the spec file (see openapi-refresh).
RAUTHY_VERSION ?= 0.35.2
SPEC := internal/client/mock/testdata/rauthy-openapi-$(RAUTHY_VERSION).json

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./... -count=1

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
