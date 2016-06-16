package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/wieni/go-imgrect/canny"
	"github.com/wieni/go-tls/simplehttp"
)

type response struct {
	Msg interface{} `json:"msg"`
}

func router(r *http.Request, l *log.Logger) (simplehttp.HandleFunc, int) {
	switch r.Method {
	case "OPTION":
		return serveOption, 0
	case "POST":
		fallthrough
	case "GET":
		switch strings.Trim(r.URL.Path, " /") {
		case "":
			return serveHelp, 0
		case "weighted":
			return serveRects, 0
		case "bounded":
			return serveRectsBounded, 0
		}
	default:
		return nil, http.StatusMethodNotAllowed
	}

	return nil, 0
}

func addCORSHeaders(w http.ResponseWriter) {
	header := w.Header()

	header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	header.Set("Access-Control-Allow-Origin", "*")
}

func serveHelp(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, err error) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, `
GET  /weighted?url=<http|https img url>&preview=0|1&w=0.2&h=200
POST /weighted?preview=0|1
         multipart/form-data: file=<file>
                              w=<float<1 | int>
                              h=<float<1 | int>

GET  /bounded?url=<http|https img url>&b0=x1,y1,x2,x2&b1=x1,y1,x2,x2&b<n>=x1,y1,x2,x2
POST /bounded
         multipart/form-data: file=<file>
                              b0=<int>,<int>,<int>,<int>
                              b1=<int>,<int>,<int>,<int>
                              b<n>=<int>,<int>,<int>,<int>
`)
	return
}

func serveOption(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, err error) {
	addCORSHeaders(w)
	return
}

func getBound(r *http.Request, index int) (*image.Rectangle, error) {
	raw := strings.Split(r.FormValue(fmt.Sprintf("b%d", index)), ",")
	if len(raw) != 4 {
		return nil, errors.New("Invalid rectangle spec")
	}

	var ints [4]int
	for i := range raw {
		integ, err := strconv.Atoi(raw[i])
		if err != nil {
			return nil, err
		}

		ints[i] = integ
	}

	rect := image.Rect(ints[0], ints[1], ints[2], ints[3])
	return &rect, nil
}

func serveRectsBounded(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, herr error) {
	w.Header().Set("Content-Type", "application/json")

	i := 0
	rects := make([]*image.Rectangle, 0, 2)

	for {
		rect, err := getBound(r, i)
		if err != nil {
			break
		}

		rects = append(rects, rect)
		i++
		if i > 20 {
			errStatus = http.StatusNotAcceptable
			return
		}
	}

	if len(rects) < 2 {
		errStatus = http.StatusNotAcceptable
		return
	}

	file, err := getRequestFile(r)
	if err != nil {
		errStatus = http.StatusNotAcceptable
		herr = err
		return
	}

	bounds, err := bounded(file, rects)
	if err != nil {
		herr = err
		return
	}

	return 0, json.NewEncoder(w).Encode(&response{bounds})
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

	var re []*PercentRectangle
	re, err = weighted(file, 5, width, height, preview)
	if err == canny.ErrLoadFailed {
		errStatus = http.StatusUnsupportedMediaType
		return
	}

	if err != nil || preview != nil {
		return
	}

	return 0, json.NewEncoder(w).Encode(&response{re})
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
