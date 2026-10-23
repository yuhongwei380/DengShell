//go:build desktop

package desktop

import (
	"os"
	"path/filepath"
)

// Native dialogs are modal to the main window. Only one picker may be open at
// a time: repeated handshake events or concurrent requests must not create
// nested dialogs. On Windows, platform dialogs also use an independent STA
// with a scoped cursor guard for WebView's hide-pointer-while-typing state.
//
// The starting folder is pinned as well. Without it the OS decides, based on
// what it persisted for the dialog role, which reads as jumping back to a folder
// from an earlier attempt.
func (d *Desktop) beginPicker() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pickerBusy {
		return false
	}
	d.pickerBusy = true
	return true
}
func (d *Desktop) endPicker() {
	d.mu.Lock()
	d.pickerBusy = false
	d.mu.Unlock()
}

// rememberedDir returns the last directory this kind of dialog worked in, or an
// empty string so the dialog falls back to the system default.
func (d *Desktop) rememberedDir(kind string) string {
	d.mu.Lock()
	dir := d.lastDirs[kind]
	d.mu.Unlock()
	if dir == "" {
		return ""
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// remember puts the next dialog of this kind into the directory of the path the
// user just picked; rememberFolder does the same for a picked directory.
func (d *Desktop) remember(kind, picked string) {
	if picked == "" {
		return
	}
	d.rememberFolder(kind, filepath.Dir(picked))
}
func (d *Desktop) rememberFolder(kind, dir string) {
	if dir == "" {
		return
	}
	d.mu.Lock()
	if d.lastDirs == nil {
		d.lastDirs = map[string]string{}
	}
	d.lastDirs[kind] = dir
	d.mu.Unlock()
}

// openDialog runs one native open/save dialog with the remembered folder and
// releases the lock when it closes. An empty result means cancelled or refused.
func (d *Desktop) openDialog(kind string, open func(directory string) (string, error)) (string, error) {
	if !d.beginPicker() {
		return "", nil
	}
	defer d.endPicker()
	result, err := open(d.rememberedDir(kind))
	if err == nil {
		d.remember(kind, result)
	}
	return result, err
}

// openFolderDialog is the directory-picker form: the picked directory itself is
// remembered, so the next one opens where the user last worked.
func (d *Desktop) openFolderDialog(kind string, open func(directory string) (string, error)) (string, error) {
	if !d.beginPicker() {
		return "", nil
	}
	defer d.endPicker()
	result, err := open(d.rememberedDir(kind))
	if err == nil {
		d.rememberFolder(kind, result)
	}
	return result, err
}

// openDialogMulti is the multiple-files form of openDialog.
func (d *Desktop) openDialogMulti(kind string, open func(directory string) ([]string, error)) ([]string, error) {
	if !d.beginPicker() {
		return nil, nil
	}
	defer d.endPicker()
	results, err := open(d.rememberedDir(kind))
	if err == nil && len(results) > 0 {
		d.remember(kind, results[0])
	}
	return results, err
}
