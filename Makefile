libs = $(addprefix -l,$(subst .a,,$(subst vendor_c/lib/lib,,$(wildcard vendor_c/lib/*.a))))
env = CGO_CFLAGS="-I$(CURDIR)/vendor_c/include" CGO_LDFLAGS="-L$(CURDIR)/vendor_c/lib $(libs)" PKG_CONFIG_PATH="$(CURDIR)/vendor_c/lib/pkgconfig"

example:
	$(env) go run ./astiencoder -v -j examples/$(example).json

build:
	$(env) go build -o $(GOPATH)/bin/astiencoder ./astiencoder

server:
	$(env) go run ./astiencoder -v

test:
	$(env) go test -cover -v ./...

version:
	$(env) go run ./astiencoder version

install-ffmpeg:
	mkdir -p vendor_c/src
	git clone https://github.com/FFmpeg/FFmpeg vendor_c/src/ffmpeg
	cd vendor_c/src/ffmpeg && git checkout n4.0.2
	cd vendor_c/src/ffmpeg && ./configure --prefix=../.. $(configure)
	cd vendor_c/src/ffmpeg && make
	cd vendor_c/src/ffmpeg && make install
