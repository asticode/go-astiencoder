package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func main() {
	// Set logger
	log.SetFlags(0)

	// Read web dir
	fs, err := ioutil.ReadDir("web")
	if err != nil {
		log.Fatal(fmt.Errorf("main: reading web dir failed: %w", err))
	}

	// Loop through files
	cs := make(map[string][]byte)
	for _, f := range fs {
		// Get content
		b, err := ioutil.ReadFile("web/" + f.Name())
		if err != nil {
			log.Fatal(fmt.Errorf("main: reading %s file failed: %w", f.Name(), err))
		}

		// Store content
		cs[f.Name()] = b
	}

	// Build content
	c := bytes.ReplaceAll(cs["index.html"], []byte("/* style */"), cs["index.css"])
	c = bytes.ReplaceAll(c, []byte("/* script */"), cs["index.js"])

	// Build bytes
	var bs []string
	for _, b := range c {
		bs = append(bs, fmt.Sprintf("%#x", b))
	}

	// Create bind file
	f, err := os.Create("server_bind.go")
	if err != nil {
		log.Fatal(fmt.Errorf("main: creating bind file failed: %w", err))
	}
	defer f.Close()

	// Write
	if _, err = f.Write([]byte(`package astiencoder

import "net/http"

func (s *Server) serveHomepage() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Write
		if _, err := rw.Write([]byte{` + strings.Join(bs, ",") + `}); err != nil {
			s.l.Errorf("astiencoder: writing failed: %w")
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}`)); err != nil {
		log.Fatal(fmt.Errorf("main: writing failed: %w", err))
	}
}
