CONTAINER_BIN=docker
RUN_CONTAINER=$(CONTAINER_BIN) run --rm --security-opt label=disable
MINIO_CONTAINER=minio-chaparral-test

.PHONY: minio-start minio-stop

minio-start:
	$(RUN_CONTAINER) -d --name $(MINIO_CONTAINER)\
		-p 9000:9000 \
		-v "$(shell pwd)/testdata/minio:/data" \
		quay.io/minio/minio server /data

minio-stop:
	$(CONTAINER_BIN) stop $(MINIO_CONTAINER)
