//go:build windows

package winshell

import (
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
)

const shellIconSize = 16

func attachNodeIcon(node *Node) {
	if node == nil || node.ParsingName == "" {
		return
	}
	pixels, width, height, err := shellIconRGBA(node.ParsingName)
	if err != nil {
		return
	}
	node.IconRGBA = pixels
	node.IconWidth = width
	node.IconHeight = height
}

func shellIconRGBA(parsingName string) ([]byte, int, int, error) {
	item, err := openShellItem(parsingName)
	if err != nil {
		return nil, 0, 0, err
	}
	defer item.Release()
	var factory *win32.IShellItemImageFactory
	hr := item.QueryInterface(&win32.IID_IShellItemImageFactory, unsafe.Pointer(&factory))
	if win32.FAILED(hr) || factory == nil {
		return nil, 0, 0, shellHRESULT("open Windows Shell icon factory", hr)
	}
	defer factory.Release()

	var bitmap win32.HBITMAP
	hr = factory.GetImage(win32.SIZE{Cx: shellIconSize, Cy: shellIconSize},
		win32.SIIGBF_ICONONLY|win32.SIIGBF_BIGGERSIZEOK, &bitmap)
	if win32.FAILED(hr) || bitmap == 0 {
		return nil, 0, 0, shellHRESULT("read Windows Shell icon", hr)
	}
	defer win32.DeleteObject(win32.HGDIOBJ(bitmap))

	info := win32.BITMAPINFO{BmiHeader: win32.BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(win32.BITMAPINFOHEADER{})),
		BiWidth:       shellIconSize,
		BiHeight:      -shellIconSize,
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win32.BI_RGB,
	}}
	bgra := make([]byte, shellIconSize*shellIconSize*4)
	dc := win32.GetDC(0)
	if dc == 0 {
		return nil, 0, 0, shellHRESULT("create Windows Shell icon device context", -1)
	}
	lines := win32.GetDIBits(dc, bitmap, 0, shellIconSize, unsafe.Pointer(&bgra[0]), &info, win32.DIB_RGB_COLORS)
	win32.ReleaseDC(0, dc)
	if lines == 0 {
		return nil, 0, 0, shellHRESULT("convert Windows Shell icon", -1)
	}
	rgba := make([]byte, len(bgra))
	alphaPresent := false
	for index := 0; index < len(bgra); index += 4 {
		rgba[index+0] = bgra[index+2]
		rgba[index+1] = bgra[index+1]
		rgba[index+2] = bgra[index+0]
		rgba[index+3] = bgra[index+3]
		alphaPresent = alphaPresent || bgra[index+3] != 0
	}
	if !alphaPresent {
		for index := 0; index < len(rgba); index += 4 {
			if rgba[index] != 0 || rgba[index+1] != 0 || rgba[index+2] != 0 {
				rgba[index+3] = 255
			}
		}
	}
	return rgba, shellIconSize, shellIconSize, nil
}
