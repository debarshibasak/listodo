//go:build !darwin || !cgo

package desktop

import (
	"fmt"

	"github.com/adaptive-scale/listodo/internal/model"
)

func RunWindow(url string) error {
	return fmt.Errorf("the native desktop window currently requires macOS; use --serve to open %s in a browser", url)
}

func saveBackup(data []byte) (bool, error) {
	return false, fmt.Errorf("use the browser export to download a backup on this platform")
}

func chooseFolder() (string, error) {
	return "", fmt.Errorf("enter an absolute project folder path")
}

func prepareRunNotifications()                    {}
func notifyRunFinished(operation model.Operation) {}
