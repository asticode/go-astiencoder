`astiencoder` is an open source video encoder written in GO and based on [ffmpeg](https://github.com/FFmpeg/FFmpeg) C bindings.

DISCLAIMER: the API can and will change without backward compatibility.

Right now this project has only been tested on FFMpeg 4.0.2.

# Why use this project when I can use `ffmpeg` binary?

In most cases you won't need this project as the `ffmpeg` binary is pretty awesome.

However, this project could be useful to you if you want to:

- integrate your video encoder in a GO ecosystem
- visualize your encoding workflow and statuses/stats of nodes in real time
- communicate with the encoder through a web socket to tweak behaviours in real time
- build your own video encoder and take control of its workflow

# How do I install this project?
## FFMpeg

In order to use the `ffmpeg` C bindings, you need to install ffmpeg. To do so, run the following command:

```
$ make install-ffmpeg
```

In some cases, you'll need to enable/disable libs explicitly. To do so, use the `configure` placeholder. For instance:

```
$ make install-ffmpeg configure="--enable-libx264 --enable-gpl"
```

## Astiencoder

Simply run the following command:

```
$ go get github.com/asticode/go-astiencoder/...
```

You can check everything is working properly by running:

```
$ make version
```

# How do I run the out-of-the-box encoder?

- 2 run modes
- how to use the UI + play with nodes

# How can I run examples?

Examples are located in the `examples` folder. If you want to run a specific example, run the following command:

```
$ make example=<name of the example>
```

File outputs will be written in the `examples/tmp` folder.

# How is this project structured?

This project has 3 important packages:

- at the root of the project, package `astiencoder` provides the framework to build an encoder that can build workflows made up of nodes. At this point, nodes can start/stop any kind of work.
- in folder `libav`, package `astilibav` provides the proper nodes to use the `ffmpeg` C bindings.
- in folder `astiencoder`, package `main` provides an out-of-the-box encoder using packages `astiencoder` and `astilibav` that can execute specific nodes.

The packages you use depend on your needs although I strongly recommend using the `main` package directly.

# Which ffmpeg C bindings is this project using and why?

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

# Features and roadmap

- [x] copy (remux)
- [x] mjpeg (thumbnails)

# Contribute

Contributions are more than welcome! Simply fork this project, make changes in a specific branch such as `patch-1` for instance and submit a PR.
