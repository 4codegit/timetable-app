#ifndef ORTOOLS_BIND_H
#define ORTOOLS_BIND_H

#ifdef _WIN32
#define ORTOOLS_EXPORT __declspec(dllexport)
#else
#define ORTOOLS_EXPORT
#endif

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  int ctype;        // 0 teacher_unavailable, 1 class_unavailable, 2 room_unavailable,
                    // 3 max_consecutive, 4 lunch_break, 5 max_lessons_per_day, 6 min_lessons_per_day
  int entity_type;  // 0 teacher, 1 class, 2 room, 3 school
  int entity_id;
  int day;
  int slot_start;
  int slot_end;
  int value;        // numeric parameter (max/min count, or window bound)
  int is_hard;
} CConstraint;

typedef struct {
  int count;
  int* lesson_ids;
  int* class_ids;
  int* teacher_ids;
  int* subject_ids;
  int* room_ids;
  int* days;
  int* slots;
} ScheduleResult;

/* Runs OR-Tools CP-SAT. Returns NULL if infeasible / unavailable.
   Arrays are caller-owned except the returned ScheduleResult (freed via free_schedule_result).
   room_type / lesson_req_type use 0 for "any", otherwise a stable integer id per type string.

   Предпочтительная точка входа — ortools_solve_out: C не выделяет памяти,
   решение пишется в буферы вызывающей стороны (Go), ручной free не нужен.
   Совместимая ortools_solve оставлена для старых сборок. */
ORTOOLS_EXPORT ScheduleResult* ortools_solve(
   int num_lessons,
   const int* lesson_hours,
   const int* lesson_class,
   const int* lesson_teacher,
   const int* lesson_subject,
   const int* lesson_req_type,
   int num_rooms,
   const int* room_ids,
   const int* room_type,
   int days_per_week,
   int slots_per_day,
   int num_constraints,
   const CConstraint* constraints,
   int time_limit_ms,
   int workers);

ORTOOLS_EXPORT void free_schedule_result(ScheduleResult* r);

/* ortools_solve_out: то же решение, но в выходные буферы (по одному на
   поле), которые выделяет вызывающая сторона. capacity — вместимость
   КАЖДОГО буфера (число вхождений уроков, Go считает его так же), запись
   ограничена capacity. Возвращает 1, если решение есть (тогда *out_count —
   число записей; day=-1 у неразмещённых), иначе 0 и *out_count = 0. */
ORTOOLS_EXPORT int ortools_solve_out(
   int num_lessons,
   const int* lesson_hours,
   const int* lesson_class,
   const int* lesson_teacher,
   const int* lesson_subject,
   const int* lesson_req_type,
   int num_rooms,
   const int* room_ids,
   const int* room_type,
   int days_per_week,
   int slots_per_day,
   int num_constraints,
   const CConstraint* constraints,
   int time_limit_ms,
   int workers,
   int* out_lesson,
   int* out_class,
   int* out_teacher,
   int* out_subject,
   int* out_room,
   int* out_day,
   int* out_slot,
   int capacity,
   int* out_count);

#ifdef __cplusplus
}
#endif

#endif
