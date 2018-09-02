`astiencoder` is an attempt to create an open source Go encoder based on `ffmpeg` C bindings.

Goals are:

- provide basic encoding needs
- provide full hardware implementation
- allow communicating with the encoder through a websocket

# FFMpeg C bindings

Right now this project is using [these bindings](https://github.com/asticode/goav).

Here's why:

1) [the base project](https://github.com/giorgisio/goav) is not maintained anymore
2) [this project](https://github.com/targodan/ffgopeg) is a hard fork of #1 but has gone a very different route
3) [this project](https://github.com/amarburg/goav) is the best fork of #1 but the last commit is not the most recent one (October 2017)
4) [this project](https://github.com/ioblank/goav) is a fork of #3 with a few interesting additional commits
5) [this project](https://github.com/koropets/goav) is a fork of #3 with the latest commits (November 2017)
6) [this project](https://github.com/alon-ne/goav) has a very nice set of examples

Therefore I've forked #5, added commits from #4 and removed deprecated functions.