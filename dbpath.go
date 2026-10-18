package main

import (
	"os"
	"path/filepath"
)

// dbPath — где хранить базу: рядом с исполняемым файлом (на Windows —
// рядом с timetable.exe), если папка доступна для записи; иначе в
// текущей рабочей папке.
func dbPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		p := filepath.Join(dir, "timetable.db")
		if f, err := os.CreateTemp(dir, ".wtest"); err == nil {
			f.Close()
			os.Remove(f.Name())
			return p
		}
	}
	return "./timetable.db"
}
