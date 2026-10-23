//go:build desktop && !windows

package desktop

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func platformSaveFileDialog(ctx context.Context, options runtime.SaveDialogOptions) (string, error) {
	return runtime.SaveFileDialog(ctx, options)
}

func platformOpenFileDialog(ctx context.Context, options runtime.OpenDialogOptions) (string, error) {
	return runtime.OpenFileDialog(ctx, options)
}

func platformOpenMultipleFilesDialog(ctx context.Context, options runtime.OpenDialogOptions) ([]string, error) {
	return runtime.OpenMultipleFilesDialog(ctx, options)
}

func platformOpenDirectoryDialog(ctx context.Context, options runtime.OpenDialogOptions) (string, error) {
	return runtime.OpenDirectoryDialog(ctx, options)
}
