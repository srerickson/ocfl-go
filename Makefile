.PHONY: startminio stopminio

startminio:
	##
	## checking podman install
	##
	which podman
	podman pull quay.io/minio/minio:latest
	podman run --name ocfl-test -d --rm -p 9000:9000 -p 9001:9001 minio/minio server /data --console-address ":9001"
	podman exec ocfl-test mkdir /data/ocfl-test

stopminio:
	##
	## stoping minio
	##
	which podman
	podman stop ocfl-test
	