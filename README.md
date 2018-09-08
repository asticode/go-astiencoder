`astiencoder` is an attempt to create an open source Go encoder based on [ffmpeg](https://github.com/FFmpeg/FFmpeg) C bindings.

Goals are:

- provide basic encoding needs
- provide full hardware acceleration implementation
- allow communicating with the encoder through a websocket
- allow developers to play with this lib to create their own specific encoder

Right now this project has only been tested on FFMpeg 4.0.2.

# Folders structure

- `astiencoder` contains the default encoder. This is a real-life example of how to use this lib.
- `data` contains data to test the default encoder more easily
- `libav` contains the `ffmpeg` components such as the `opener`, `demuxer`, etc.
- `web` contains all the html/js/css files of the UI

Files at the root of the project are the lib files

# Installation
## FFMpeg

```
$ make install-ffmpeg
```

## Astiencoder

```
$ go get github.com/asticode/go-astiencoder/...
```

You can check everything is working properly by running:

```
$ make version
```

# FFMpeg C bindings

Right now this project is using [these bindings](https://github.com/asticode/goav).

Here's why:

1) [the base project](https://github.com/giorgisio/goav) is not maintained anymore
2) [this project](https://github.com/targodan/ffgopeg) is a hard fork of #1 but has gone a very different route
3) [this project](https://github.com/selfmodify/goav) is a fork of #1 but I'm experiencing lots of panics
4) [this project](https://github.com/amarburg/goav) is the best fork of #1 even though the last commit is not recent
5) [this project](https://github.com/ioblank/goav) is a fork of #4 with interesting additional commits
6) [this project](https://github.com/koropets/goav) is a fork of #4 with interesting additional commits
7) [this project](https://github.com/alon-ne/goav) has a very nice set of examples

Therefore I've forked #4, added useful commits from other forks and removed deprecated functions so that it works properly in FFMpeg 4.0.2.