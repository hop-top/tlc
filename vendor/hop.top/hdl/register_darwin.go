//go:build darwin

package hdl

/*
#cgo LDFLAGS: -framework CoreServices
#include <CoreServices/CoreServices.h>
#include <stdlib.h>

OSStatus hdlRegisterScheme(const char* scheme, const char* bundleID) {
	CFStringRef s = CFStringCreateWithCString(NULL, scheme, kCFStringEncodingUTF8);
	CFStringRef b = CFStringCreateWithCString(NULL, bundleID, kCFStringEncodingUTF8);
	OSStatus status = LSSetDefaultHandlerForURLScheme(s, b);
	CFRelease(s);
	CFRelease(b);
	return status;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// ErrUnsupported satisfies the cross-platform contract; always nil on darwin.
var ErrUnsupported error

// Register sets the default handler for the given URL scheme on macOS.
// appID should be a bundle identifier (e.g. "com.example.ctxt").
func Register(scheme, appID string) error {
	if scheme == "" {
		return fmt.Errorf("hdl: scheme must not be empty")
	}
	if appID == "" {
		return fmt.Errorf("hdl: app must not be empty")
	}

	cs := C.CString(scheme)
	ca := C.CString(appID)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))

	status := C.hdlRegisterScheme(cs, ca)
	if status != 0 {
		return fmt.Errorf("hdl: LSSetDefaultHandlerForURLScheme returned status %d", int(status))
	}
	return nil
}
