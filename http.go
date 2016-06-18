package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
			return serveBounded, 0
		}
	default:
		return nil, http.StatusMethodNotAllowed
	}

	return nil, 0
}

func serveHelp(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, err error) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write(helpText)
	return
}

func serveOption(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, err error) {
	return
}

func serveBounded(w http.ResponseWriter, r *http.Request, l *log.Logger) (errStatus int, herr error) {
	maxFormSize := int64(60 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxFormSize)
	defer r.Body.Close()
	w.Header().Set("Content-Type", "application/json")

	i := 0
	rects := make([]*percentRectangle, 0, 2)

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

	file, err := getRequestFile(r, "file", "url")
	if err != nil {
		errStatus = http.StatusNotAcceptable
		herr = err
		return
	}
	defer file.Close()

	bounds, err := bounded(file, rects)
	if err == canny.ErrInvalidBounds {
		errStatus = http.StatusNotAcceptable
		return
	}

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
	file, err = getRequestFile(r, "file", "url")
	if err != nil {
		errStatus = http.StatusNotAcceptable
		return
	}
	defer file.Close()

	font, _ := getRequestFile(r, "font", "fonturl")
	if font != nil {
		defer font.Close()
	}

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

	fontSize := getFormFloat(r, "fontsize", 0)
	fontText := r.FormValue("text")

	var re []*percentRectangle
	re, err = weighted(file, font, fontSize, fontText, 5, width, height, preview)
	if err == canny.ErrLoadFailed {
		errStatus = http.StatusUnsupportedMediaType
		return
	}

	if err != nil || preview != nil {
		return
	}

	return 0, json.NewEncoder(w).Encode(&response{re})
}

func getRequestFile(r *http.Request, fileField, urlField string) (file io.ReadCloser, err error) {
	if r.Method == "POST" {
		file, _, err = r.FormFile(fileField)
		if err == nil {
			return
		}
	}

	if r.Method == "GET" || err == http.ErrMissingFile {
		url := r.FormValue(urlField)
		if url == "" {
			err = errors.New("No url")
			return
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

func getBound(r *http.Request, index int) (*percentRectangle, error) {
	raw := strings.Split(r.FormValue(fmt.Sprintf("b%d", index)), ",")
	if len(raw) != 4 {
		return nil, errors.New("Invalid rectangle spec")
	}

	var ints [4]int
	var values [4]float64
	for i := range raw {
		val, err := strconv.ParseFloat(raw[i], 64)
		//integ, err := strconv.Atoi(raw[i])
		if err != nil {
			return nil, err
		}

		if val < 1 {
			values[i] = val
			continue
		}

		ints[i] = int(val)
	}

	return &percentRectangle{
		Min: &percentPoint{ints[0], ints[1], values[0], values[1]},
		Max: &percentPoint{ints[2], ints[3], values[2], values[3]},
	}, nil
}
