//go:build desktop && !windows

package desktop

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func platformSaveFileDialog(ctx context.Context, options runtime.SaveDialogOptions) (string, error) {
	return runtime.SaveFileDialog(ctx, options)
}
