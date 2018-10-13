package main

import (
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/asticode/go-astiencoder"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func serverCustomHandler(r *httprouter.Router, e *astiencoder.Encoder) {
	r.POST("/api/workflows", func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get name
		var name string
		if name = r.FormValue("name"); len(name) == 0 {
			astiencoder.WriteJSONError(rw, http.StatusBadRequest, errors.New("astiencoder: name is mandatory"))
			return
		}

		// Parse job
		var f multipart.File
		var err error
		if f, _, err = r.FormFile("job"); err != nil {
			if err != http.ErrMissingFile {
				astiencoder.WriteJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: getting form file failed"))
			} else {
				astiencoder.WriteJSONError(rw, http.StatusBadRequest, errors.New("astiencoder: job is mandatory"))
			}
			return
		}

		// Unmarshal
		var j Job
		if err = json.NewDecoder(f).Decode(&j); err != nil {
			astiencoder.WriteJSONError(rw, http.StatusBadRequest, errors.Wrap(err, "astiencoder: unmarshaling job failed"))
			return
		}

		// Add workflow
		if _, err := addWorkflow(name, j, e); err != nil {
			astiencoder.WriteJSONError(rw, http.StatusBadRequest, errors.Wrapf(err, "astiencoder: adding workflow %s failed", name))
			return
		}
	})
}
