dev:
	go run astiencoder/*.go -v -j testdata/job.json

version:
	go run astiencoder/*.go version