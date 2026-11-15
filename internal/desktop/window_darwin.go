//go:build darwin && cgo

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework UserNotifications
#include <stdlib.h>
void listodoRun(const char *url);
int listodoSave(const void *data, int length);
char *listodoChooseFolder(void);
void listodoPrepareRunNotifications(void);
void listodoNotifyRunFinished(const char *id, const char *title, const char *body, int turn);
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/adaptive-scale/listodo/internal/model"
)

// AppKit requires the process's original OS thread. Lock during package init,
// before Go's scheduler can move main onto a different thread.
func init() { runtime.LockOSThread() }

func RunWindow(url string) error {
	u := C.CString(url)
	defer C.free(unsafe.Pointer(u))
	C.listodoRun(u)
	return nil
}

func saveBackup(data []byte) (bool, error) {
	b := C.CBytes(data)
	defer C.free(b)
	result := C.listodoSave(b, C.int(len(data)))
	if result < 0 {
		return false, fmt.Errorf("could not write the backup file")
	}
	return result == 1, nil
}

func chooseFolder() (string, error) {
	path := C.listodoChooseFolder()
	if path == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}

func prepareRunNotifications() { C.listodoPrepareRunNotifications() }

func notifyRunFinished(operation model.Operation) {
	provider := "Codex"
	if operation.Provider == "claude" {
		provider = "Claude Code"
	}
	status := "finished"
	if operation.Status == "failed" {
		status = "failed"
	}
	id := C.CString(operation.ID)
	title := C.CString(provider + " run " + status)
	body := C.CString(operation.Title)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	C.listodoNotifyRunFinished(id, title, body, C.int(operation.Turn))
}
