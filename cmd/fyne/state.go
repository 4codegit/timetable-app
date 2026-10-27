package main

import "context"

var activeSchoolID int

func setActiveSchoolID(id int) {
	activeSchoolID = id
}

// runningGeneration — незавершённая генерация. Guard от второго параллельного
// запуска: обе горутины писали бы ReplaceSchedule, и результат «победителя»
// молча затирал бы первый.
//
// Доступ только из UI-потока (Fyne): как и activeSchoolID, без мьютекса.
// Горутина генерации общается с UI через fyne.Do, schoolID получает по значению.
type runningGeneration struct {
	schoolID int
	cancel   context.CancelFunc
}

var activeGeneration *runningGeneration
