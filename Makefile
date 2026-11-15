.PHONY: docs build test testacc rauthy-up rauthy-down print-rauthy-version fmt vet

# Rauthy version the provider tracks and the acceptance tests boot. Bump it
# together with the github.com/fluencelabs/rauthy-go release that tracks the
# same version.
RAUTHY_VERSION ?= 0.36.2

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
# version bump cannot leave the acceptance matrix on the old release.
print-rauthy-version:
	@echo $(RAUTHY_VERSION)

docs:
	tfplugindocs generate --provider-name rauthy

fmt:
	go fmt ./...
