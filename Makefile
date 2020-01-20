libs = $(addprefix -l,$(subst .a,,$(subst tmp/lib/lib,,$(wildcard tmp/lib/*.a))))
env = CGO_CFLAGS="-I$(CURDIR)/tmp/include" CGO_LDFLAGS="-L$(CURDIR)/tmp/lib $(libs)" PKG_CONFIG_PATH="$(CURDIR)/tmp/lib/pkgconfig"

example:
	$(env) go run ./astiencoder -j examples/$(example).json

build:
	$(env) go build -o $(GOPATH)/bin/astiencoder ./astiencoder

server:
	$(env) go run ./astiencoder

test:
	$(env) go test -cover -v ./...

version:
	$(env) go run ./astiencoder version

install-ffmpeg:
	mkdir -p tmp/src
	git clone https://github.com/FFmpeg/FFmpeg tmp/src/ffmpeg
	cd tmp/src/ffmpeg && git checkout n4.1.1
	cd tmp/src/ffmpeg && ./configure --prefix=../.. $(configure)
	cd tmp/src/ffmpeg && make
	cd tmp/src/ffmpeg && make install
