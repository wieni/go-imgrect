package main

import (
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/wieni/go-imgrect/canny"
	"github.com/wieni/go-tls/simplehttp"

	"github.com/lazywei/go-opencv/opencv"
)

var (
	maxCPU    int
	errLogger *log.Logger
)

func init() {
	maxCPU = runtime.NumCPU()
	errLogger = log.New(os.Stderr, "", log.LstdFlags)
}

func rects(
	reader io.Reader,
	amount int,
	minWidth,
	minHeight float64,
	preview io.Writer,
) ([]*canny.PercentRectangle, error) {
	if amount < 1 {
		amount = 1
	}

	_img, origWidth, origHeight, err := canny.Load(reader, 800)
	if err != nil {
		return nil, err
	}

	defer _img.Release()
	width := _img.Width()
	height := _img.Height()

	if minWidth < 1 {
		minWidth = float64(origWidth) * minWidth
	}

	if minHeight < 1 {
		minHeight = float64(origHeight) * minHeight
	}

	minWidth *= float64(width) / float64(origWidth)
	minHeight *= float64(height) / float64(origHeight)

	var rects canny.Rectangles
	var img *opencv.IplImage

	for threshold := 3.0; threshold < 36; threshold += 3 {
		img = canny.Canny(_img, threshold, 3, true)
		defer img.Release()

		_rects := canny.FindRects(img, int(minWidth), int(minHeight))
		sort.Sort(_rects)
		rects = append(rects, _rects...)
		rects = canny.FilterOverlap(rects, amount)

		if len(rects) >= amount {
			break
		}
	}

	if len(rects) < amount {
		amount = len(rects)
	}
	rects = rects[:amount]

	if preview == nil {
		return rects.ToPercentRectangles(
			width,
			height,
			origWidth,
			origHeight,
		), nil
	}

	goimg := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := _img.Get1D(width*y + x).Val()[0]
			goimg.Set(x, y, color.Gray{uint8(c)})
		}
	}

	r := 127 / float64(len(rects))
	for i, rect := range rects {
		c := color.Gray{127 + uint8(r*float64(i))}
		for x := rect.Min.X; x < rect.Max.X; x++ {
			goimg.Set(x, rect.Min.Y, c)
			goimg.Set(x, rect.Max.Y-1, c)
		}

		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			goimg.Set(rect.Min.X, y, c)
			goimg.Set(rect.Max.X-1, y, c)
		}
	}

	jpeg.Encode(preview, goimg, nil)
	return rects.ToPercentRectangles(
		width,
		height,
		origWidth,
		origHeight,
	), nil
}

func main() {
	_port := flag.Int("p", 8080, "Port to listen on.")

	flag.Parse()
	port := strconv.Itoa(*_port)

	l := log.New(os.Stderr, "http|", 0)
	server := simplehttp.FromHTTPServer(
		&http.Server{
			ReadTimeout:  time.Second * 2,
			WriteTimeout: time.Second * 5,
		},
		router,
		l,
	)

	errLogger.Fatal(server.Start(":"+port, false))
}
