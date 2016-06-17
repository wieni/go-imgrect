package canny

import (
	"errors"
	"image"
	"io"
	"io/ioutil"
	"unsafe"

	"github.com/lazywei/go-opencv/opencv"
)

// ErrLoadFailed will be returned if given imagedata can not be loaded
// by opencv
var ErrLoadFailed = errors.New("Image failed to load")

// ErrInvalidBounds will be returned if the given bounds do not fully overlap
// with the given image
var ErrInvalidBounds = errors.New("Invalid bounds")

// Rectangles is a slice of Rectangles
type Rectangles []*image.Rectangle

func (r Rectangles) Len() int      { return len(r) }
func (r Rectangles) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r Rectangles) Less(i, j int) bool {
	return r[i].Dx()*r[i].Dy() > r[j].Dx()*r[j].Dy()
}

func minInt(n, m int) int {
	if n < m {
		return n
	}

	return m
}

func maxInt(n, m int) int {
	if n > m {
		return n
	}

	return m
}

func matSum(mat []int, width int) []int {
	sum := make([]int, len(mat))
	height := len(mat) / width

	for x := 0; x < width; x++ {
		sum[x] = mat[x]
	}

	for y := 1; y < height; y++ {
		sum[y*width] = mat[y*width]
	}

	for y := 1; y < height; y++ {
		for x := 1; x < width; x++ {
			if mat[width*y+x] != 0 {
				sum[width*y+x] = minInt(
					sum[width*y+(x-1)],
					minInt(
						sum[width*(y-1)+x],
						sum[width*(y-1)+(x-1)],
					),
				) + 1
			}

		}
	}

	return sum
}

func matRects(sum []int, width, minWidth, minHeight int) Rectangles {
	height := len(sum) / width

	var curr int
	var west int
	var xoffset int
	var yoffset int
	var rects Rectangles

	for y := height - 1; y > 0; y-- {
		for x := width - 1; x > 0; x-- {
			curr = sum[width*y+x]
			west = sum[width*y+x-1]

			if curr <= 1 || west > curr {
				continue
			}

			yoffset = curr - 1
			xoffset = curr - 1
			// Find identical squares to the left
			for xw := x - 1; xw >= 0; xw-- {
				if sum[width*y+xw] == curr {
					xoffset++
					continue
				}

				break
			}

			// Find acceptable squares even further to the left
			ratio := 0.6
			for xw := x - (xoffset + 1); xw >= 0; xw-- {
				n := sum[width*y+xw]
				if n < curr && float64(curr)*ratio <= float64(n) {
					yoffset = n - 1
					xoffset++
					continue
				}

				break
			}

			if xoffset < minWidth || yoffset < minHeight {
				continue
			}

			for cy := y - yoffset; cy <= y; cy++ {
				for cx := x - xoffset; cx <= x; cx++ {
					sum[width*cy+cx] = 0
				}
			}

			rect := image.Rect(x-xoffset, y-yoffset, x+1, y+1)
			if x-xoffset < 0 {
				panic(1)
			}
			if y-yoffset < 0 {
				panic(2)
			}
			rects = append(rects, &rect)
		}

	}

	return rects
}

func fromByteSlice(data []byte) *opencv.IplImage {
	// passing an empty slice to CreateMatHeader will fail HARD.
	if len(data) == 0 {
		return nil
	}

	buf := opencv.CreateMatHeader(1, len(data), opencv.CV_8U)
	buf.SetData(unsafe.Pointer(&data[0]), opencv.CV_AUTOSTEP)
	defer buf.Release()

	return opencv.DecodeImage(unsafe.Pointer(buf), opencv.CV_LOAD_IMAGE_GRAYSCALE)
}

// Load as grayscale en resize
func Load(reader io.Reader, maxSize int) (*opencv.IplImage, int, int, error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, 0, 0, err
	}

	src := fromByteSlice(data)
	if src == nil {
		return nil, 0, 0, ErrLoadFailed
	}

	origWidth := src.Width()
	origHeight := src.Height()
	w := origWidth
	h := origHeight
	r := float64(w) / float64(h)
	if w > maxSize {
		w = maxSize
		h = int(float64(maxSize) / r)
	}

	if h > maxSize {
		w = int(float64(maxSize) * r)
		h = maxSize
	}

	if w == origWidth && h == origHeight {
		return src, origWidth, origHeight, nil
	}
	defer src.Release()

	dst := opencv.Resize(src, w, h, 0)

	return dst, origWidth, origHeight, nil
}

// CropBounds crops a single image into multiple defined by bounds.
func CropBounds(img *opencv.IplImage, bounds []*image.Rectangle) ([]*opencv.IplImage, error) {
	imgs := make([]*opencv.IplImage, len(bounds))
	w := img.Width()
	h := img.Height()

	for i, b := range bounds {
		if b.Min.X < 0 ||
			b.Max.X < 0 ||
			b.Min.Y < 0 ||
			b.Max.Y < 0 ||
			b.Min.X > w ||
			b.Max.X > w ||
			b.Min.Y > h ||
			b.Max.Y > h {
			return nil, ErrInvalidBounds
		}

		imgs[i] = opencv.Crop(img, b.Min.X, b.Min.Y, b.Dx(), b.Dy())
	}

	return imgs, nil
}

// Canny the image
func Canny(src *opencv.IplImage, threshold, ratio float64, clone bool) *opencv.IplImage {
	dst := src
	if clone {
		dst = src.Clone()
	}

	blur := minInt(src.Width(), src.Height()) / 20
	opencv.Smooth(dst, dst, opencv.CV_BLUR, blur, blur, 0, 0)
	opencv.Canny(dst, dst, threshold, threshold*ratio, 3)

	return dst
}

// FindRects in the given cannied image
func FindRects(cannied *opencv.IplImage, minWidth, minHeight int) Rectangles {
	width := cannied.Width()
	height := cannied.Height()
	mat := make([]int, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if cannied.Get1D(width*y + x).Val()[0] == 0 {
				mat[width*y+x] = 1
			}
		}
	}

	return matRects(matSum(mat, width), width, minWidth, minHeight)
}

// FilterOverlap removes rectangles that overlap with larger ones.
func FilterOverlap(rects Rectangles, limit int) Rectangles {
	ret := make(Rectangles, 0, minInt(limit, len(rects)))
	for i := 0; i < len(rects); i++ {
		collides := false
		r0 := rects[i]
		for _, r1 := range ret {
			if r0.Overlaps(*r1) {
				collides = true
				break
			}
		}

		if !collides {
			ret = append(ret, r0)
			if len(ret) >= limit {
				break
			}
		}
	}

	return ret
}
