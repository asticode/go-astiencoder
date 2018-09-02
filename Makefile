dev:
	CGO_CFLAGS="-I$(CURDIR)/vendor_c/include" CGO_LDFLAGS="-L$(CURDIR)/vendor_c/lib" go run astiencoder/*.go -v -j testdata/job.json

version:
	CGO_CFLAGS="-I$(CURDIR)/vendor_c/include" CGO_LDFLAGS="-L$(CURDIR)/vendor_c/lib" go run astiencoder/*.go version

install-ffmpeg:
	mkdir -p vendor_c/src
	git clone https://github.com/FFmpeg/FFmpeg vendor_c/src/ffmpeg
	cd vendor_c/src/ffmpeg && git checkout n4.0.2
	cd vendor_c/src/ffmpeg && ./configure --prefix=../..
	cd vendor_c/src/ffmpeg && make
	cd vendor_c/src/ffmpeg && make install
