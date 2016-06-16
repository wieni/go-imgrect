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

// percentPoint contains both X, Y and %X, %Y
type percentPoint struct {
	X        int     `json:"x"`
	Y        int     `json:"y"`
	PercentX float64 `json:"%x"`
	PercentY float64 `json:"%y"`
}

func (p *percentPoint) absoluteX(srcWidth, dstWidth int) int {
	if p.PercentX == 0 {
		return int(float64(p.X) / float64(srcWidth) * float64(dstWidth))
	}

	return int(float64(dstWidth) * p.PercentX)
}

func (p *percentPoint) absoluteY(srcHeight, dstHeight int) int {
	if p.PercentY == 0 {
		return int(float64(p.Y) / float64(srcHeight) * float64(dstHeight))
	}

	return int(float64(dstHeight) * p.PercentY)
}

// percentRectangle like image.Rectangle defines a Min and Max point
type percentRectangle struct {
	Min *percentPoint `json:"min"`
	Max *percentPoint `json:"max"`
}

// toPercentRectangles returns a slice of percentRectangles
// Percentage is calculated based on srcWidth and srcHeight
// new X and Y based on dstWidth and dstHeight
func toPercentRectangles(
	r canny.Rectangles,
	srcWidth,
	srcHeight,
	dstWidth,
	dstHeight int,
) []*percentRectangle {
	rects := make([]*percentRectangle, len(r))
	sw := float64(srcWidth)
	sh := float64(srcHeight)

	for i := range r {
		minxp := float64(r[i].Min.X) / sw
		minyp := float64(r[i].Min.Y) / sh
		maxxp := float64(r[i].Max.X) / sw
		maxyp := float64(r[i].Max.Y) / sh

		rects[i] = &percentRectangle{
			Min: &percentPoint{
				int(float64(dstWidth) * minxp),
				int(float64(dstHeight) * minyp),
				minxp,
				minyp,
			},
			Max: &percentPoint{
				int(float64(dstWidth) * maxxp),
				int(float64(dstHeight) * maxyp),
				maxxp,
				maxyp,
			},
		}
	}

	return rects
}

type bound struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

type bounds []*bound

func (b bounds) Len() int      { return len(b) }
func (b bounds) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b bounds) Less(i, j int) bool {
	return b[i].Score*b[i].Score < b[j].Score*b[j].Score
}

func bounded(
	reader io.Reader,
	rects []*percentRectangle,
) (bounds, error) {
	img, w, h, err := canny.Load(reader, 1024*1024*100, 800)
	rw := img.Width()
	rh := img.Height()

	_rects := make([]*image.Rectangle, len(rects))
	for i := range rects {
		rect := image.Rect(
			rects[i].Min.absoluteX(w, rw),
			rects[i].Min.absoluteY(h, rh),
			rects[i].Max.absoluteX(w, rw),
			rects[i].Max.absoluteY(h, rh),
		)

		_rects[i] = &rect
	}

	imgs, err := canny.CropBounds(img, _rects)
	if err != nil {
		return nil, err
	}

	scores := make(bounds, len(imgs))
	for i := range imgs {
		img := canny.Canny(imgs[i], 3, 3, false)
		defer img.Release()
		scores[i] = &bound{i, img.Avg(nil).Val()[0]}
	}

	sort.Sort(scores)
	return scores, nil
}

func weighted(
	reader io.Reader,
	amount int,
	minWidth,
	minHeight float64,
	preview io.Writer,
) ([]*percentRectangle, error) {
	if amount < 1 {
		amount = 1
	}

	_img, origWidth, origHeight, err := canny.Load(reader, 1024*1024*100, 800)
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
		return toPercentRectangles(
			rects,
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
	return toPercentRectangles(
		rects,
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
			ReadTimeout:  time.Second * 10,
			WriteTimeout: time.Second * 10,
		},
		router,
		l,
	)

	server.SetHeader("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	server.SetHeader("Access-Control-Allow-Origin", "*")

	errLogger.Fatal(server.Start(":"+port, false))
}
