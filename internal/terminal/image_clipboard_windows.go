//go:build windows

package terminal

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"runtime"
	"syscall"
	"unsafe"
)

const fileCFDIB = 8

// setImageClipboard publishes the picture twice: as the registered "PNG"
// format, which keeps transparency for Office, the browsers and Telegram, and
// as CF_DIB, the format every Win32 program down to Paint understands.
func setImageClipboard(ctx context.Context, pngData []byte) error {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return err
	}
	dib := clipboardDIB(img)
	if err := ctx.Err(); err != nil {
		return err
	}
	fileClipboardMu.Lock()
	defer fileClipboardMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := fileOpenClipboard.Find(); err != nil {
		return err
	}
	opened, _, _ := fileOpenClipboard.Call(0)
	if opened == 0 {
		return fmt.Errorf("OpenClipboard failed")
	}
	defer fileCloseClipboard.Call()
	fileEmptyClipboard.Call()

	dibHandle, err := fileGlobalBytes(dib)
	if err != nil {
		return err
	}
	if r, _, _ := fileSetClipboardData.Call(fileCFDIB, dibHandle); r == 0 {
		fileGlobalFree.Call(dibHandle)
		return fmt.Errorf("SetClipboardData(CF_DIB) failed")
	}
	name, _ := syscall.UTF16PtrFromString("PNG")
	if format, _, _ := fileRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(name))); format != 0 {
		if handle, err := fileGlobalBytes(pngData); err == nil {
			if r, _, _ := fileSetClipboardData.Call(format, handle); r == 0 {
				fileGlobalFree.Call(handle)
			}
		}
	}
	return nil
}

// clipboardDIB lays img out as a packed CF_DIB: a BITMAPINFOHEADER followed
// by 32-bit BGRA rows, bottom row first.
func clipboardDIB(img image.Image) []byte {
	b := img.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	const headerSize = 40
	out := make([]byte, headerSize+w*h*4)
	binary.LittleEndian.PutUint32(out[0:], headerSize)
	binary.LittleEndian.PutUint32(out[4:], uint32(int32(w))) // #nosec G115 -- image dimensions fit int32.
	binary.LittleEndian.PutUint32(out[8:], uint32(int32(h))) // #nosec G115 -- positive height: bottom-up rows.
	binary.LittleEndian.PutUint16(out[12:], 1)               // planes
	binary.LittleEndian.PutUint16(out[14:], 32)              // bits per pixel
	binary.LittleEndian.PutUint32(out[20:], uint32(w*h*4))   // #nosec G115 -- BI_RGB image size.
	pix := out[headerSize:]
	for y := 0; y < h; y++ {
		src := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
		dst := pix[(h-1-y)*w*4 : (h-y)*w*4]
		for x := 0; x < w; x++ {
			dst[x*4+0] = src[x*4+2]
			dst[x*4+1] = src[x*4+1]
			dst[x*4+2] = src[x*4+0]
			dst[x*4+3] = src[x*4+3]
		}
	}
	return out
}
