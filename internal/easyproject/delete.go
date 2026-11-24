package easyproject

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeletedDirName is the folder under the projects directory a deleted
// project's folder is moved into, so a mistake can be undone by hand.
const DeletedDirName = ".deleted"

// ErrOutsideProjects refuses to move a folder that is not a project folder
// directly under the projects directory: the record could say anything,
// and deleting must never touch the person's other files.
var ErrOutsideProjects = errors.New("the folder is not a project folder in the projects directory")

// DeleteFolder moves a project's folder into base/.deleted, under its own
// name and the time, and answers where it went; "" when the folder was
// already gone. It moves only a real folder (never a link) whose parent,
// with links resolved, is base itself.
func DeleteFolder(base, folder string, now time.Time) (string, error) {
	if strings.TrimSpace(folder) == "" || strings.TrimSpace(base) == "" {
		return "", ErrOutsideProjects
	}
	folder = filepath.Clean(folder)
	info, err := os.Lstat(folder)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrOutsideProjects
	}
	name := filepath.Base(folder)
	if name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", ErrOutsideProjects
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", ErrOutsideProjects
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(folder))
	if err != nil || realParent != realBase {
		return "", ErrOutsideProjects
	}

	trash := filepath.Join(realBase, DeletedDirName)
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return "", err
	}
	stamp := now.Local().Format("20060102-150405")
	for i := 1; i <= maxFolderTries; i++ {
		dest := filepath.Join(trash, name+"-"+stamp)
		if i > 1 {
			dest = fmt.Sprintf("%s-%d", dest, i)
		}
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		if err := os.Rename(filepath.Join(realParent, name), dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("no free name for %q under %s", name, trash)
}
