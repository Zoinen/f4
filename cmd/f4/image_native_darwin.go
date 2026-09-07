//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework CoreFoundation -framework CoreGraphics -framework ImageIO

#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <ImageIO/ImageIO.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	uint8_t *pixels;
	size_t width;
	size_t height;
	size_t stride;
	int error;
} f4_apple_image;

static f4_apple_image f4_decode_apple_image(const uint8_t *bytes, size_t length,
											size_t max_pixels) {
	f4_apple_image result = {0};
	CFDataRef data = NULL;
	CGImageSourceRef source = NULL;
	CGImageRef image = NULL;
	CGColorSpaceRef colour = NULL;
	CGContextRef context = NULL;
	CFNumberRef max_dimension = NULL;
	CFDictionaryRef options = NULL;

	if (bytes == NULL || length == 0) {
		result.error = 1;
		goto done;
	}
	data = CFDataCreate(kCFAllocatorDefault, bytes, (CFIndex)length);
	if (data == NULL) {
		result.error = 2;
		goto done;
	}
	source = CGImageSourceCreateWithData(data, NULL);
	if (source == NULL || CGImageSourceGetCount(source) == 0) {
		result.error = 3;
		goto done;
	}

	// Creating a thumbnail at the source dimensions asks ImageIO for the
	// original pixels while also applying the camera's orientation metadata.
	int dimension = 32768;
	max_dimension = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &dimension);
	const void *keys[] = {
		kCGImageSourceCreateThumbnailFromImageAlways,
		kCGImageSourceCreateThumbnailWithTransform,
		kCGImageSourceThumbnailMaxPixelSize,
		kCGImageSourceShouldCacheImmediately,
	};
	const void *values[] = {
		kCFBooleanTrue,
		kCFBooleanTrue,
		max_dimension,
		kCFBooleanTrue,
	};
	options = CFDictionaryCreate(kCFAllocatorDefault, keys, values, 4,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	image = CGImageSourceCreateThumbnailAtIndex(source, 0, options);
	if (image == NULL) {
		result.error = 4;
		goto done;
	}

	result.width = CGImageGetWidth(image);
	result.height = CGImageGetHeight(image);
	if (result.width == 0 || result.height == 0 ||
		result.width > SIZE_MAX / result.height ||
		result.width * result.height > max_pixels ||
		result.width > SIZE_MAX / 4) {
		result.error = 5;
		goto done;
	}
	result.stride = result.width * 4;
	if (result.height > SIZE_MAX / result.stride) {
		result.error = 5;
		goto done;
	}
	result.pixels = calloc(result.height, result.stride);
	if (result.pixels == NULL) {
		result.error = 6;
		goto done;
	}

	colour = CGColorSpaceCreateDeviceRGB();
	context = CGBitmapContextCreate(result.pixels, result.width, result.height, 8,
		result.stride, colour,
		(CGImageAlphaInfo)(kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big));
	if (context == NULL) {
		result.error = 7;
		goto done;
	}
	CGContextDrawImage(context, CGRectMake(0, 0, result.width, result.height), image);

done:
	if (context != NULL) CGContextRelease(context);
	if (colour != NULL) CGColorSpaceRelease(colour);
	if (image != NULL) CGImageRelease(image);
	if (options != NULL) CFRelease(options);
	if (max_dimension != NULL) CFRelease(max_dimension);
	if (source != NULL) CFRelease(source);
	if (data != NULL) CFRelease(data);
	if (result.error != 0 && result.pixels != NULL) {
		free(result.pixels);
		result.pixels = NULL;
	}
	return result;
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/unxed/vtui"
)

const appleImageDecoder = "apple-imageio"

func decodeImageWithAppleImageIO(ctx context.Context, data []byte) (*vtui.ImageSurface, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("there is nothing to decode")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	decoded := C.f4_decode_apple_image(
		(*C.uint8_t)(unsafe.Pointer(&data[0])),
		C.size_t(len(data)),
		C.size_t(imageMaxPixels),
	)
	if decoded.pixels == nil {
		return nil, fmt.Errorf("ImageIO could not decode the image (error %d)", int(decoded.error))
	}
	defer C.free(unsafe.Pointer(decoded.pixels))
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	width, height, stride := int(decoded.width), int(decoded.height), int(decoded.stride)
	pixels := C.GoBytes(unsafe.Pointer(decoded.pixels), C.int(decoded.height*decoded.stride))
	surface := vtui.NewImageSurfaceFromPix(width, height, stride, pixels)
	if !surface.Valid() {
		return nil, fmt.Errorf("ImageIO produced invalid image geometry")
	}
	return surface, nil
}

func init() {
	RegisterImageDecoder(ImageDecoder{
		Name:       appleImageDecoder,
		Priority:   20,
		Extensions: []string{"heic", "heif", "hif"},
		DecodeCtx:  decodeImageWithAppleImageIO,
	})
}
