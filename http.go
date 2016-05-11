package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/wieni/go-imgrect/canny"
	"github.com/wieni/go-tls/simplehttp"
)

type response struct {
	Msg interface{} `json:"msg"`
}

func router(r *http.Request, l *log.Logger) (simplehttp.HandleFunc, int) {
	switch r.Method {
	case "POST":
		return serveRects, 0
	case "GET":
		return serveRects, 0
	default:
		return nil, http.StatusMethodNotAllowed
	}

	return nil, 0
}

func serveRects(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, err error) {
	maxFormSize := int64(60 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxFormSize)
	defer r.Body.Close()

	var file io.ReadCloser
	file, err = getRequestFile(r)
	if err != nil {
		errStatus = http.StatusNotAcceptable
		return
	}
	defer file.Close()

	width := getFormFloat(r, "w", 1)
	height := getFormFloat(r, "h", 1)

	var preview io.Writer
	if r.URL.Query().Get("preview") != "" {
		preview = w
	}

	headers := w.Header()
	headers.Set("Content-Type", "application/json")
	if preview != nil {
		headers.Set("Content-Type", "image/jpeg")
	}

	var re []*canny.PercentRectangle
	re, err = rects(file, 5, width, height, preview)
	if err == canny.ErrLoadFailed {
		errStatus = http.StatusUnsupportedMediaType
		return
	}

	if err != nil || preview != nil {
		return
	}

	var m []byte
	m, err = json.Marshal(&response{re})
	if err != nil {
		return
	}

	_, err = w.Write(m)
	return
}

func getRequestFile(r *http.Request) (file io.ReadCloser, err error) {
	if r.Method == "POST" {
		file, _, err = r.FormFile("file")
		if err == nil {
			return
		}
	}

	if r.Method == "GET" || err == http.ErrMissingFile {
		url := r.FormValue("url")
		if url == "" {
			// url = r.URL.Query().Get("url")
			// if url == "" {
			err = errors.New("No url")
			return
			//}
		}

		var resp *http.Response
		resp, err = http.Get(url)
		if err != nil {
			return
		}

		file = resp.Body
	}

	return
}

func getFormFloat(r *http.Request, key string, fallback float64) float64 {
	val := r.FormValue(key)
	intVal, err := strconv.ParseFloat(val, 64)
	if err != nil {
		intVal = fallback
	}

	return intVal
}

func mustJSON(v interface{}) []byte {
	ret, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return ret
}

func mustJSONErr(err string) []byte {
	return mustJSON(map[string]string{"err": err})
}
