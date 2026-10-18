#include "bind.h"

#include <chrono>
#include <cstdlib>
#include <limits>
#include <map>
#include <vector>

#include "ortools/sat/cp_model.h"
#include "ortools/sat/cp_model_solver.h"
#include "ortools/sat/sat_parameters.pb.h"

using namespace operations_research::sat;
using operations_research::Domain;

namespace {
struct Occ {
  int lesson;
  int cls;
  int teacher;
  int subject;
  int req_type;  // 0 = any
};
}  // namespace

namespace {

// Результат: по одной записи на вхождение урока (-1 в day = не размещён).
struct Solution {
  bool ok = false;
  std::vector<int> lesson, cls, teacher, subject, room, day, slot;
};

Solution solve_impl(int num_lessons, const int* lesson_hours,
                    const int* lesson_class, const int* lesson_teacher,
                    const int* lesson_subject, const int* lesson_req_type,
                    int num_rooms, const int* room_ids, const int* room_type,
                    int days_per_week, int slots_per_day, int num_constraints,
                    const CConstraint* constraints, int time_limit_ms,
                    int workers) {
  CpModelBuilder cp;

  std::vector<Occ> occ;
  for (int i = 0; i < num_lessons; ++i) {
    int h = lesson_hours[i];
    if (h < 1) h = 1;
    int rt = lesson_req_type ? lesson_req_type[i] : 0;
    for (int k = 0; k < h; ++k) {
      occ.push_back({i, lesson_class[i], lesson_teacher[i], lesson_subject[i],
                     rt});
    }
  }
  const int O = static_cast<int>(occ.size());
  const int D = days_per_week, S = slots_per_day, R = num_rooms;
  if (O == 0 || D == 0 || S == 0 || R == 0) return Solution{};

  // entity -> occurrence indices
  std::map<int, std::vector<int>> teacherOccs, classOccs, lessonOccs;
  std::vector<int> schoolOccs;
  for (int o = 0; o < O; ++o) {
    teacherOccs[occ[o].teacher].push_back(o);
    classOccs[occ[o].cls].push_back(o);
    lessonOccs[occ[o].lesson].push_back(o);
    schoolOccs.push_back(o);
  }

  // x[occurrence][day][slot][room]
  std::vector<std::vector<std::vector<std::vector<BoolVar>>>> x(
      O, std::vector<std::vector<std::vector<BoolVar>>>(
             D, std::vector<std::vector<BoolVar>>(
                    S, std::vector<BoolVar>(R))));
  for (int o = 0; o < O; ++o)
    for (int d = 0; d < D; ++d)
      for (int s = 0; s < S; ++s)
        for (int r = 0; r < R; ++r) x[o][d][s][r] = cp.NewBoolVar();

  // each occurrence placed exactly once
  for (int o = 0; o < O; ++o) {
    std::vector<BoolVar> vars;
    for (int d = 0; d < D; ++d)
      for (int s = 0; s < S; ++s)
        for (int r = 0; r < R; ++r) vars.push_back(x[o][d][s][r]);
    cp.AddAtMostOne(vars);
  }

  // room-type compatibility (req_type 0 means any)
  for (int o = 0; o < O; ++o) {
    int rt = occ[o].req_type;
    if (rt == 0) continue;
    for (int d = 0; d < D; ++d)
      for (int s = 0; s < S; ++s)
        for (int r = 0; r < R; ++r) {
          int rtype = room_type[r];
          if (rtype != 0 && rtype != rt) {
            cp.AddEquality(x[o][d][s][r], false);
          }
        }
  }

  auto cellVars = [&](const std::vector<int>& occs, int d, int s) {
    std::vector<BoolVar> v;
    for (int o : occs)
      for (int r = 0; r < R; ++r) v.push_back(x[o][d][s][r]);
    return v;
  };
  auto windowVars = [&](const std::vector<int>& occs, int d, int a, int b) {
    std::vector<BoolVar> v;
    for (int s = a; s <= b && s < S; ++s)
      for (int o : occs)
        for (int r = 0; r < R; ++r) v.push_back(x[o][d][s][r]);
    return v;
  };

  // Подгруппы: child → parent (из student_group ограничений). Пары
  // подгрупп одного родителя могут делить кабинет (половины класса).
  std::map<int, int> subParent;
  for (int ci = 0; ci < num_constraints; ++ci) {
    if (constraints[ci].ctype == 9) {
      subParent[constraints[ci].entity_id] = constraints[ci].value;
    }
  }

  // Семейные группы для уникальности кабинета: подгруппы одного родителя
  // — ОДНА группа (половины класса делят кабинет законно); родитель и
  // каждый посторонний класс — сами по себе. «В кабинете не более одной
  // семьи» эквивалентно полному перебору пар уроков между разными
  // семьями, но ЛИНЕЙНО по числу уроков: старый попарный цикл давал
  // O(D·S·R·O²) констрейнтов (сотни миллионов на большой школе — сборка
  // модели съедала весь тайм-лимит), семейные бакеты дают O(D·S·R·(O+F)).
  int nextFam = 0;
  std::map<int, int> famOfCls;          // cls → семейный id
  std::map<int, int> kidsFamOfParent;   // parent → семейный id его подгрупп
  for (int o = 0; o < O; ++o) {
    int cls = occ[o].cls;
    if (famOfCls.count(cls)) continue;
    auto it = subParent.find(cls);
    if (it != subParent.end()) {
      int parent = it->second;
      if (!kidsFamOfParent.count(parent))
        kidsFamOfParent[parent] = nextFam++;
      famOfCls[cls] = kidsFamOfParent[parent];
    } else {
      famOfCls[cls] = nextFam++;
    }
  }
  // fam → вхождения уроков этой семьи
  std::map<int, std::vector<int>> famOccs;
  for (int o = 0; o < O; ++o)
    famOccs[famOfCls[occ[o].cls]].push_back(o);

  // hard uniqueness: teacher / class at most one per (day,slot); room at
  // most one FAMILY per (day,slot,room) — половины одной семьи вмещаются.
  for (int d = 0; d < D; ++d) {
    for (int s = 0; s < S; ++s) {
      for (const auto& kv : teacherOccs)
        if (!kv.second.empty()) cp.AddAtMostOne(cellVars(kv.second, d, s));
      for (const auto& kv : classOccs)
        if (!kv.second.empty()) cp.AddAtMostOne(cellVars(kv.second, d, s));
      for (int r = 0; r < R; ++r) {
        std::vector<BoolVar> famPresent;
        for (const auto& fk : famOccs) {
          BoolVar present;
          bool fresh = true;
          if (fk.second.size() == 1) {
            present = x[fk.second[0]][d][s][r]; // единственный урок — он и есть семья
          } else {
            present = cp.NewBoolVar();
            std::vector<LinearExpr> vars;
            vars.reserve(fk.second.size());
            for (int o : fk.second) vars.push_back(x[o][d][s][r]);
            cp.AddMaxEquality(present, vars);
          }
          famPresent.push_back(present);
        }
        if (famPresent.size() > 1) cp.AddAtMostOne(famPresent);
      }
    }
  }

  // constraint application
  auto occSetOf = [&](int et, int id) -> const std::vector<int>* {
    if (et == 0) return &teacherOccs[id];
    if (et == 1) return &classOccs[id];
    if (et == 3) return &schoolOccs;
    return nullptr;
  };

  std::vector<LinearExpr> penalties;
  const int afternoon = (S > 4) ? 4 : (S - 1);

  for (int ci = 0; ci < num_constraints; ++ci) {
    const CConstraint& c = constraints[ci];
    int start = 0, end = S - 1;
    if (c.slot_start >= 0) start = c.slot_start;
    if (c.slot_end >= 0) end = c.slot_end;
    if (end >= S) end = S - 1;

    auto rangeSlots = [&](int day) -> std::vector<int> {
      std::vector<int> out;
      if (day >= 0) {
        for (int s = start; s <= end; ++s) out.push_back(s);
      } else {
        for (int d = 0; d < D; ++d)
          for (int s = start; s <= end; ++s) out.push_back(d * 1000 + s);
      }
      return out;
    };

    switch (c.ctype) {
      case 0:  // teacher_unavailable
      case 1: {  // class_unavailable
        const std::vector<int>* set = occSetOf(c.entity_type, c.entity_id);
        if (!set) break;
        for (int d = 0; d < D; ++d) {
          if (c.day >= 0 && d != c.day) continue;
          for (int s = start; s <= end; ++s)
            for (int o : *set)
              for (int r = 0; r < R; ++r) cp.AddEquality(x[o][d][s][r], false);
        }
        break;
      }
      case 2: {  // room_unavailable
        int room = c.entity_id;
        if (room < 0 || room >= R) break;
        for (int d = 0; d < D; ++d) {
          if (c.day >= 0 && d != c.day) continue;
          for (int s = start; s <= end; ++s)
            for (int o = 0; o < O; ++o) cp.AddEquality(x[o][d][s][room], false);
        }
        break;
      }
      case 3: {  // max_consecutive
        if (c.value <= 0) break;
        const std::vector<int>* set = occSetOf(c.entity_type, c.entity_id);
        if (!set) break;
        for (int d = 0; d < D; ++d) {
          for (int a = 0; a + c.value < S; ++a) {
            std::vector<BoolVar> w = windowVars(*set, d, a, a + c.value);
            cp.AddLinearConstraint(LinearExpr::Sum(w), Domain(0, c.value));
          }
        }
        break;
      }
      case 9: {  // student_group: делёный предмет — обе половинки МОГУТ
                 // идти параллельно (один предмет, разные учителя).
                 // Запрещаем только пары РАЗНЫХ предметов:
                 // урок родителя (subject != subject подгруппы) не может
                 // пересечься с уроком подгруппы.
        int child = c.entity_id, parent = c.value, subj = c.slot_start;
        auto pit = classOccs.find(parent);
        auto cit = classOccs.find(child);
        if (pit == classOccs.end() || cit == classOccs.end()) break;
        for (int po : pit->second)
          for (int co : cit->second) {
            if (occ[po].subject == occ[co].subject) continue; // делёный предмет
            for (int d = 0; d < D; ++d)
              for (int s = 0; s < S; ++s) {
                std::vector<BoolVar> vars;
                for (int r = 0; r < R; ++r) vars.push_back(x[po][d][s][r]);
                for (int r = 0; r < R; ++r) vars.push_back(x[co][d][s][r]);
                cp.AddAtMostOne(vars);
              }
          }
        break;
      }
      case 10: {  // simultaneous_groups: половинки делёного урока —
                  // ОБЯЗАНЫ быть в одном слоте (класс делится на группы).
        int halfIdx = c.entity_id, baseIdx = c.value;
        auto hit = lessonOccs.find(halfIdx);
        auto bit = lessonOccs.find(baseIdx);
        if (hit == lessonOccs.end() || bit == lessonOccs.end()) break;
        for (int d = 0; d < D; ++d)
          for (int s = 0; s < S; ++s) {
            std::vector<BoolVar> v1, v2;
            for (int o : hit->second) for (int r = 0; r < R; ++r) v1.push_back(x[o][d][s][r]);
            for (int o : bit->second) for (int r = 0; r < R; ++r) v2.push_back(x[o][d][s][r]);
            if (v1.empty() || v2.empty()) continue;
            LinearExpr sum1 = LinearExpr::Sum(v1);
            LinearExpr sum2 = LinearExpr::Sum(v2);
            // sum1 == sum2  =>  sum1 - sum2 == 0
            cp.AddEquality(sum1 - sum2, 0);
          }
        break;
      }
      case 11: {  // max_consecutive_subject: не более N уроков ОДНОГО
                  // предмета подряд в день у класса (entity: subject или
                  // school = все предметы). value = макс. подряд.
        if (c.value >= 1) {
          // Группы вхождений по (класс, предмет) в области действия.
          std::map<long long, std::vector<int>> groups;
          for (int o = 0; o < O; ++o) {
            if (c.entity_type == 4 && occ[o].subject != c.entity_id) continue;
            long long key = (long long)occ[o].cls * 100000 + occ[o].subject;
            groups[key].push_back(o);
          }
          for (const auto& kv : groups) {
            for (int d = 0; d < D; ++d) {
              for (int a = 0; a + c.value < S; ++a) {
                std::vector<BoolVar> w = windowVars(kv.second, d, a, a + c.value);
                cp.AddLinearConstraint(LinearExpr::Sum(w), Domain(0, c.value));
              }
            }
          }
        }
        break;
      }
      case 4: {  // lunch_break (require a free slot in [start,end])
        const std::vector<int>* set = occSetOf(c.entity_type, c.entity_id);
        if (!set) break;
        int window = end - start + 1;
        if (window <= 0) break;
        for (int d = 0; d < D; ++d) {
          std::vector<BoolVar> w = windowVars(*set, d, start, end);
          cp.AddLinearConstraint(LinearExpr::Sum(w), Domain(0, window - 1));
        }
        break;
      }
      case 5: {  // max_lessons_per_day
        if (c.value <= 0) break;
        const std::vector<int>* set = occSetOf(c.entity_type, c.entity_id);
        if (!set) break;
        for (int d = 0; d < D; ++d) {
          std::vector<BoolVar> w = cellVars(*set, d, 0);
          for (int s = 1; s < S; ++s) {
            std::vector<BoolVar> more = cellVars(*set, d, s);
            w.insert(w.end(), more.begin(), more.end());
          }
          cp.AddLinearConstraint(LinearExpr::Sum(w), Domain(0, c.value));
        }
        break;
      }
      case 6: {  // min_lessons_per_day (soft)
        if (c.value <= 0) break;
        const std::vector<int>* set = occSetOf(c.entity_type, c.entity_id);
        if (!set) break;
        for (int d = 0; d < D; ++d) {
          std::vector<BoolVar> w = cellVars(*set, d, 0);
          for (int s = 1; s < S; ++s) {
            std::vector<BoolVar> more = cellVars(*set, d, s);
            w.insert(w.end(), more.begin(), more.end());
          }
          IntVar p = cp.NewIntVar(Domain(0, c.value)).WithName("minpen");
          LinearExpr lhs = LinearExpr::Sum(w);
          lhs = lhs + LinearExpr(p);
          cp.AddLinearConstraint(lhs, Domain(c.value, std::numeric_limits<int64_t>::max()));
          penalties.push_back(LinearExpr(p));
        }
        break;
      }
      case 7: {  // prefer_morning (soft): penalize afternoon placements
        for (int o = 0; o < O; ++o)
          for (int d = 0; d < D; ++d)
            for (int s = afternoon; s < S; ++s)
              for (int r = 0; r < R; ++r) penalties.push_back(LinearExpr(x[o][d][s][r]));
        break;
      }
      default:
        break;
    }
  }

  // Максимизируем количество размещённых уроков (школа может быть
  // перегружена — кабинетов меньше, чем нужно для всех уроков). Цель
  // собираем из компактных IntVar (по одной на урок), а не из сотен
  // тысяч bool-термов — иначе сборка/прешолв съедали весь тайм-лимит.
  // Штрафы добавляются с весом (O+1): размещение важнее штрафа.
  std::vector<IntVar> placedVars;
  for (int o = 0; o < O; ++o) {
    std::vector<BoolVar> vars;
    for (int d = 0; d < D; ++d)
      for (int s = 0; s < S; ++s)
        for (int r = 0; r < R; ++r) vars.push_back(x[o][d][s][r]);
    IntVar placed = cp.NewIntVar(Domain(0, 1));
    cp.AddEquality(LinearExpr::Sum(vars), placed);
    placedVars.push_back(placed);
  }
  LinearExpr obj = -LinearExpr::Sum(placedVars) * static_cast<int64_t>(O + 1);
  for (size_t i = 0; i < penalties.size(); ++i) obj = obj + penalties[i];
  cp.Minimize(obj);

  SatParameters params;
  params.set_max_time_in_seconds(static_cast<double>(time_limit_ms) / 1000.0);
  if (workers > 0) params.set_num_search_workers(workers);
  // Максимум размещённых за лимит времени. stop_after_first_solution
  // тут враг: первое решение оставляет часть уроков неразмещённой,
  // и SolvePrecise откатывается на эвристику, не знающую про
  // одновременность половинок делёного урока.

  const auto t_build0 = std::chrono::steady_clock::now();
  CpModelProto model_proto = cp.Build();
  const double build_sec =
      std::chrono::duration<double>(std::chrono::steady_clock::now() - t_build0).count();
  const CpSolverResponse response = SolveWithParameters(model_proto, params);
  {
    // Диагностика в stdout сервера: статус, время сборки модели и решения.
    std::fprintf(stderr, "[ortools] status=%d build=%.1fs wall_time=%.1fs\n",
                 static_cast<int>(response.status()), build_sec,
                 response.wall_time());
  }
  if (response.status() != CpSolverStatus::OPTIMAL &&
      response.status() != CpSolverStatus::FEASIBLE) {
    return Solution{};
  }

  Solution sol;
  sol.ok = true;
  sol.lesson.resize(O);
  sol.cls.resize(O);
  sol.teacher.resize(O);
  sol.subject.resize(O);
  sol.room.resize(O);
  sol.day.assign(O, -1);
  sol.slot.assign(O, -1);
  for (int o = 0; o < O; ++o) {
    bool found = false;
    for (int d = 0; d < D && !found; ++d) {
      for (int s = 0; s < S && !found; ++s) {
        for (int r = 0; r < R && !found; ++r) {
          if (response.solution(x[o][d][s][r].index()) > 0) {
            sol.lesson[o] = occ[o].lesson;
            sol.cls[o] = occ[o].cls;
            sol.teacher[o] = occ[o].teacher;
            sol.subject[o] = occ[o].subject;
            sol.room[o] = room_ids[r];
            sol.day[o] = d;
            sol.slot[o] = s;
            found = true;
          }
        }
      }
    }
    // Перегруженная школа: урок не удалось разместить — day остаётся -1,
    // Go-сторона пропускает запись (иначе мусор ломал бы FOREIGN KEY).
  }
  return sol;
}

// Копирование решения в буферы, выделенные вызывающей стороной (Go).
// Возвращает 1, если решение есть, и *out_count = число вхождений.
// Буферы вмещают не меньше вхождений (Go считает их так же), но запись
// всё равно ограничена capacity — выход за буфер Go-кучи недопустим.
static int write_solution(const Solution& sol, int* out_lesson, int* out_class,
                          int* out_teacher, int* out_subject, int* out_room,
                          int* out_day, int* out_slot, int capacity,
                          int* out_count) {
  if (!sol.ok) {
    if (out_count) *out_count = 0;
    return 0;
  }
  const int n = static_cast<int>(sol.lesson.size());
  const int m = (n < capacity) ? n : capacity;
  for (int i = 0; i < m; ++i) {
    out_lesson[i] = sol.lesson[i];
    out_class[i] = sol.cls[i];
    out_teacher[i] = sol.teacher[i];
    out_subject[i] = sol.subject[i];
    out_room[i] = sol.room[i];
    out_day[i] = sol.day[i];
    out_slot[i] = sol.slot[i];
  }
  if (out_count) *out_count = m;
  return 1;
}

}  // namespace

// Основная точка входа: решение пишется в буферы вызывающей стороны,
// C не выделяет памяти вовсе (нет ручного free и риска утечки);
// на Windows exe (DLL-путь) это же устраняет конверсию uintptr →
// unsafe.Pointer. Возвращает 1 при наличии решения, иначе 0.
extern "C" ORTOOLS_EXPORT int ortools_solve_out(
    int num_lessons, const int* lesson_hours, const int* lesson_class,
    const int* lesson_teacher, const int* lesson_subject,
    const int* lesson_req_type, int num_rooms, const int* room_ids,
    const int* room_type, int days_per_week, int slots_per_day,
    int num_constraints, const CConstraint* constraints, int time_limit_ms,
    int workers, int* out_lesson, int* out_class, int* out_teacher,
    int* out_subject, int* out_room, int* out_day, int* out_slot,
    int capacity, int* out_count) {
  Solution sol = solve_impl(num_lessons, lesson_hours, lesson_class,
                            lesson_teacher, lesson_subject, lesson_req_type,
                            num_rooms, room_ids, room_type, days_per_week,
                            slots_per_day, num_constraints, constraints,
                            time_limit_ms, workers);
  return write_solution(sol, out_lesson, out_class, out_teacher, out_subject,
                        out_room, out_day, out_slot, capacity, out_count);
}

// Совместимая точка входа (malloc-версия) — оставлена для старых
// сборок; новые Go-обёртки используют ortools_solve_out.
extern "C" ORTOOLS_EXPORT ScheduleResult* ortools_solve(
    int num_lessons, const int* lesson_hours, const int* lesson_class,
    const int* lesson_teacher, const int* lesson_subject,
    const int* lesson_req_type, int num_rooms, const int* room_ids,
    const int* room_type, int days_per_week, int slots_per_day,
    int num_constraints, const CConstraint* constraints, int time_limit_ms,
    int workers) {
  Solution sol = solve_impl(num_lessons, lesson_hours, lesson_class,
                            lesson_teacher, lesson_subject, lesson_req_type,
                            num_rooms, room_ids, room_type, days_per_week,
                            slots_per_day, num_constraints, constraints,
                            time_limit_ms, workers);
  if (!sol.ok) return nullptr;
  const int n = static_cast<int>(sol.lesson.size());
  ScheduleResult* res =
      static_cast<ScheduleResult*>(malloc(sizeof(ScheduleResult)));
  res->count = n;
  res->lesson_ids = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->class_ids = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->teacher_ids = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->subject_ids = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->room_ids = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->days = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  res->slots = static_cast<int*>(malloc(sizeof(int) * (n > 0 ? n : 1)));
  for (int i = 0; i < n; ++i) {
    res->lesson_ids[i] = sol.lesson[i];
    res->class_ids[i] = sol.cls[i];
    res->teacher_ids[i] = sol.teacher[i];
    res->subject_ids[i] = sol.subject[i];
    res->room_ids[i] = sol.room[i];
    res->days[i] = sol.day[i];
    res->slots[i] = sol.slot[i];
  }
  return res;
}

extern "C" void free_schedule_result(ScheduleResult* r) {
  if (!r) return;
  free(r->lesson_ids);
  free(r->class_ids);
  free(r->teacher_ids);
  free(r->subject_ids);
  free(r->room_ids);
  free(r->days);
  free(r->slots);
  free(r);
}
