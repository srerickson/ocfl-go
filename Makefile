.PHONY: s3-up s3-down s3-test s3-clean test-all

# Ports and credentials for the MinIO S3 test service.
S3_PORT ?= 9000
OCFL_TEST_S3 ?= http://localhost:$(S3_PORT)
export AWS_ACCESS_KEY_ID ?= ocfltest
export AWS_SECRET_ACCESS_KEY ?= ocfltest
export AWS_REGION ?= us-east-1

# ── MinIO S3 test environment ──────────────────────────────────────────────
# Start the MinIO container in the background.
s3-up:
	docker compose -f docker-compose.yml up -d --wait

# Stop the MinIO container.
s3-down:
	docker compose -f docker-compose.yml down

# Remove the MinIO container and its data volume.
s3-clean:
	docker compose -f docker-compose.yml down -v

# Run the S3-backed tests (requires s3-up first).
s3-test:
	OCFL_TEST_S3=$(OCFL_TEST_S3) go test -count=1 ./fs/s3/...

# ── All tests ──────────────────────────────────────────────────────────────
# Run the full test suite, including S3-backed tests.
test-all: s3-up
	OCFL_TEST_S3=$(OCFL_TEST_S3) go test -count=1 ./...
	$(MAKE) s3-down
