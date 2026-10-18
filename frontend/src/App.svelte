<script>
        import {
                Greet, CreateSchool, ListSchools, DeleteSchool, SchoolHasLessons, SchoolHasSchedule,
                CreateTeacher, ListTeachers, CreateSubject, ListSubjects,
                CreateClass, ListClasses, CreateRoom, ListRooms,
                CreateLesson, ListLessons, DeleteLesson, UpdateLesson,
                CreateConstraint, ListConstraints, DeleteConstraint,
                DeleteTeacher, DeleteSubject, DeleteClass, DeleteRoom, DeleteScheduleEntry, SaveExport,
                UpdateTeacher, UpdateSubject, UpdateClass, UpdateRoom,
                Generate, GeneratePrecise, MoveEntry, SwapEntries, ReplaceSchedule, ListSchedule, ExportAll, ImportAll, ScheduleCSV, GetSchoolSettings, UpdateSchoolSettings, HasPreciseSolver, ExportPDF
        } from "../wailsjs/go/main/App";
        import { onMount } from "svelte";

        let schools = [];
        let activeSchoolID = 0;
        let newSchoolName = "Моя школа";
        let tab = "refs";
        const APP_VERSION = "1.11.1";
        let msg = "";

        let teachers = [], subjects = [], classes = [], rooms = [], lessons = [], constraints = [], schedule = [];

        // O(1)-резолверы имён: раньше subjName/teachName/... делали
        // Array.find по справочнику на КАЖДУЮ ячейку сетки (сотни тысяч
        // сканов на большом расписании). Сигнатуры (list, id) сохранены —
        // список остаётся синтаксической зависимостью для реактивности.
        let subjectById = new Map(), teacherById = new Map(), classById = new Map(), roomById = new Map();
        $: {
                subjectById = new Map(subjects.map((s) => [s.id, s]));
                teacherById = new Map(teachers.map((t) => [t.id, t]));
                classById = new Map(classes.map((c) => [c.id, c]));
                roomById = new Map(rooms.map((r) => [r.id, r]));
        }
        // Подгруппы класса: classID → [id подгрупп]. Для cellSubs —
        // раньше фильтровали весь список классов на каждый вызов.
        let kidsOf = new Map();
        $: {
                const m = new Map();
                for (const c of classes) {
                        if (c.subgroup_of) {
                                if (!m.has(c.subgroup_of)) m.set(c.subgroup_of, []);
                                m.get(c.subgroup_of).push(c.id);
                        }
                }
                kidsOf = m;
        }

        // form models
        let t = { name: "", short_name: "", max_hours_per_week: 30 };
        let s = { name: "", short_name: "", requires_room_type: "any" };
        let c = { name: "", room_id: 0, subgroup_of: null };
        let r = { name: "", room_type: "any" };
        let l = { class_id: 0, subject_id: 0, teacher_id: 0, hours_per_week: 1 };
        let curClass = 0;
        // Инлайн-редактирование справочников: ✎ переводит строку в режим
        // правки, ✓ сохраняет в БД, ✗ откатывает (перезагрузкой справочников).
        let editing = null; // { kind, id }
        function startEdit(kind, id) { editing = { kind, id }; }
        async function cancelEdit() { editing = null; await reloadRefs(); }
        async function saveEdit(kind, x) {
                if (kind === "teacher") await UpdateTeacher({ id: x.id, school_id: x.school_id, name: x.name, short_name: x.short_name, max_hours_per_week: x.max_hours_per_week || 0 });
                else if (kind === "subject") await UpdateSubject({ id: x.id, school_id: x.school_id, name: x.name, short_name: x.short_name, requires_room_type: x.requires_room_type || "any" });
                else if (kind === "class") await UpdateClass({ id: x.id, school_id: x.school_id, name: x.name, room_id: x.room_id || 0 });
                else if (kind === "room") await UpdateRoom({ id: x.id, school_id: x.school_id, name: x.name, room_type: x.room_type || "any" });
                editing = null;
                await reloadRefs();
                flash("Изменения сохранены");
        }
        // Визуальный редактор запретов: выбираем учителя/класс/кабинет и
        // кликаем по ячейкам сетки — запрет ставится/снимается сразу.
        let consPickKind = "teacher";
        let consPickId = 0;
        $: consPickList = consPickKind === "teacher" ? teachers
                : consPickKind === "class" ? classes : rooms;
        // Эффективная сущность: выбранная или первая из списка (без записи
        // обратно в состояние — иначе цикл зависимостей).
        $: consPickEntity = consPickList.find((x) => x.id === consPickId) || consPickList[0] || null;
        function setPickKind(kind) { consPickKind = kind; }
        function pickName(x) { return x.name || ""; }
        // Ограничения «недоступен», покрывающие ячейку (day, slot).
        function coveringConstraints(day, slot) {
                if (!consPickEntity) return [];
                return constraints.filter((c) =>
                        c.entity_type === consPickKind && c.entity_id === consPickEntity.id && c.is_hard &&
                        (c.type === "teacher_unavailable" || c.type === "class_unavailable" || c.type === "room_unavailable") &&
                        (c.day_of_week === null || c.day_of_week === day) &&
                        (c.timeslot_start === null || slot >= c.timeslot_start) &&
                        (c.timeslot_end === null || slot <= c.timeslot_end));
        }
        function isForbidden(day, slot) { return coveringConstraints(day, slot).length > 0; }
        async function toggleForbidden(day, slot) {
                if (!consPickEntity) { flash("Сначала добавьте учителя/класс/кабинет в справочники."); return; }
                const covering = coveringConstraints(day, slot);
                if (covering.length) {
                        for (const c of covering) await DeleteConstraint(c.id);
                        flash("Запрет снят: " + dayName(day) + " П" + (slot + 1));
                } else {
                        await CreateConstraint({
                                type: consPickKind + "_unavailable", entity_type: consPickKind,
                                entity_id: consPickEntity.id, day_of_week: day,
                                timeslot_start: slot, timeslot_end: slot,
                                is_hard: true, school_id: activeSchoolID,
                        });
                        flash("Запрещено: " + dayName(day) + " П" + (slot + 1));
                }
                await reloadRefs();
        }
        function toggleConDay(d, on) {
                con.days = on ? [...con.days, d].sort((a, b) => a - b) : con.days.filter((x) => x !== d);
        }
        // Правило «один предмет — не более N подряд» (max_consecutive_subject).
        let ruleSubject = 0, ruleMax = 2;
        async function addRunRule() {
                const n = Math.max(1, Math.min(8, ruleMax || 0));
                if (!n) { flash("Укажите число подряд (например 2)"); return; }
                await CreateConstraint({
                        school_id: activeSchoolID,
                        type: "max_consecutive_subject",
                        entity_type: ruleSubject ? "subject" : "school",
                        entity_id: ruleSubject || 0,
                        weight: n,
                        is_hard: true,
                });
                await reloadRefs();
                flash("Правило добавлено: " + (ruleSubject ? subjName(subjects, ruleSubject) : "все предметы") + " — не более " + n + " подряд");
        }

        let days = 6, slots = 8;
        // Учебные дни недели (чекбоксы в «Ограничениях»): бит 0 = Пн …
        // бит 6 = Вс. 0 — до загрузки настроек (тогда активны первые days).
        let schoolDaysMask = 0;
        const DAY_NAMES = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
        // Дни, которые рисуем в сетках/печати/PDF: включённые в маску,
        // а без маски — первые days дней (легаси-поведение).
        $: activeDayIdx = schoolDaysMask > 0
                ? DAY_NAMES.map((_, d) => d).filter((d) => schoolDaysMask & (1 << d))
                : Array.from({ length: Math.min(days, 7) }, (_, d) => d);
        let bellPeriods = [];
        let genResult = null;
        let usePrecise = false;
        // hasPreciseSolver is loaded once from the backend so the UI can tell the
        // user the truth: if the binary was NOT compiled with -tags ortools, the
        // "точный CP-SAT (OR-Tools)" checkbox silently runs the pure-Go fallback.
        let hasPreciseSolver = true;
        async function detectPreciseSolver() {
                try { hasPreciseSolver = await HasPreciseSolver(); }
                catch (e) { hasPreciseSolver = true; /* old binary: assume yes */ }
        }
        // "school" is the aSc-style overview: every class as a compact
        // mini-table, all on one screen. Its data kind is still "class".
        let viewMode = "school";
        $: kind = viewMode === "school" ? "class" : viewMode;
        $: overviewMode = viewMode === "school";
        // Порядок строк классов: родитель, сразу за ним его подгруппы.
        function orderClasses(list) {
                const kids = {};
                list.forEach((c) => {
                        if (c.subgroup_of) (kids[c.subgroup_of] = kids[c.subgroup_of] || []).push(c);
                });
                const out = [];
                for (const c of list) {
                        if (c.subgroup_of) continue;
                        out.push(c);
                        (kids[c.id] || []).sort((a, b) => String(a.label).localeCompare(String(b.label))).forEach((s) => out.push(s));
                }
                for (const c of list) if (!out.includes(c)) out.push(c);
                return out;
        }
        $: rows = kind === "teacher"
                ? teachers.map((t) => ({ id: t.id, label: t.name }))
                : kind === "room"
                ? rooms.map((r) => ({ id: r.id, label: r.name }))
                : orderClasses(classes.map((c) => ({ id: c.id, label: c.name, subgroup_of: c.subgroup_of }))).filter((c) => !c.subgroup_of);

        // PDF следует выбранному «Виду» на экране — что видите, то и в файле.
        $: exportMode = viewMode;
        let pageSize = "A2";
        let orientation = "landscape";
        let compact = false;
        let pdfShowTeacher = true;
        let pdfShowRoom = true;
        let pdfWeekdaysOnly = false;
        let pdfBW = false;
        let classPage = 0;
        const classesPerPage = 10;
        $: visibleRows = overviewMode
                ? rows
                : kind === "class"
                ? rows.slice(classPage * classesPerPage, classPage * classesPerPage + classesPerPage)
                : rows;
        $: totalClassPages = Math.max(1, Math.ceil(classes.length / classesPerPage));

        // Reactive grid. THE root-cause fix for the "drag & drop does nothing
        // until you switch tabs" bug: the old template computed each cell via
        // {@const cell = cellAt(...)} inside {#each Array(days)}. Those blocks
        // only depended on `days`/`slots` — NOT on `schedule` — so after a move
        // Svelte never recomputed the cells and the grid looked frozen even
        // though the backend had already persisted the change.
        //
        // IMPORTANT Svelte detail: reactive dependencies are collected
        // SYNTACTICALLY from the statement. cellAt() reads `schedule` and
        // `conflictIDs` internally, but the compiler cannot see inside the
        // function — so we pass them as explicit arguments to make them real
        // dependencies. Any change to the schedule now rebuilds the grid.
        //
        // Индекс расписания: `${kind}|${id}|${day}|${slot}` → записи в
        // порядке schedule. Раньше cellAt/cellSubs сканировали ВЕСЬ schedule
        // на каждую ячейку (а cellSubs ещё и звал cellAt дважды на каждую
        // запись — квадрат на каждый рендер сетки). Реактивный стейтмент
        // стоит ПЕРЕД $: grid: cellAt читает scheduleIndex, и grid должен
        // пересчитаться после него.
        function cellKey(kind, id, day, slot) { return kind + "|" + id + "|" + day + "|" + slot; }
        function idxPut(idx, kind, id, d, s, en) {
                const k = cellKey(kind, id, d, s);
                const arr = idx.get(k);
                if (arr) arr.push(en); else idx.set(k, [en]);
        }
        function buildScheduleIndex(schedule) {
                const idx = new Map();
                const pos = new Map(); // id записи → позиция в schedule (порядок ячеек)
                for (let i = 0; i < schedule.length; i++) {
                        const en = schedule[i];
                        pos.set(en.id, i);
                        idxPut(idx, "class", en.class_id, en.day_of_week, en.timeslot, en);
                        idxPut(idx, "teacher", en.teacher_id, en.day_of_week, en.timeslot, en);
                        if (en.room_id) idxPut(idx, "room", en.room_id, en.day_of_week, en.timeslot, en);
                }
                return { idx, pos };
        }
        $: scheduleIndex = buildScheduleIndex(schedule);
        $: grid = buildGrid(schedule, visibleRows, kind, activeDayIdx, slots, conflictIDs);
        function buildGrid(schedule, visibleRows, viewMode, activeDayIdx, slots, conflictIDs) {
                void schedule; void conflictIDs; // (dependencies — see comment above)
                return visibleRows.map((row) => ({
                        id: row.id,
                        label: row.label,
                        cells: activeDayIdx.map((di) =>
                                Array.from({ length: slots }, (_, si) => cellAt(viewMode, row.id, di, si)))
                }));
        }

        function flash(m) { msg = m; setTimeout(() => msg = "", 3000); }

        async function loadSchools() {
                schools = (await ListSchools()) || [];
                if (schools.length && !activeSchoolID) activeSchoolID = schools[0].id;
                await loadSettings();
        }
        async function deleteSchool() {
                if (!activeSchoolID) return;
                const name = schools.find((s) => s.id === activeSchoolID)?.name || "";
                if (!await confirmAction("Удалить школу «" + name + "»? Будут удалены ВСЕ её данные: классы, учителя, предметы, кабинеты, уроки и расписание.")) return;
                await DeleteSchool(activeSchoolID);
                activeSchoolID = 0;
                await loadSchools();
                flash("Школа удалена");
        }
        async function createSchool() {
                if (!newSchoolName.trim()) { flash("Введите название школы"); return; }
                const sc = await CreateSchool(newSchoolName);
                activeSchoolID = sc.id;
                await loadSchools();
                flash("Школа создана");
        }
        async function loadSettings() {
                if (!activeSchoolID) return;
                try {
                        const raw = await GetSchoolSettings(activeSchoolID);
                        const st = raw ? JSON.parse(raw) : {};
                        if (st.days > 0) days = st.days;
                        if (st.slots > 0) slots = st.slots;
                        schoolDaysMask = st.days_mask > 0
                                ? st.days_mask
                                : (1 << Math.min(days, 7)) - 1;
                        // Звонки не сбрасываем в пустоту при несовпадении
                        // длины: дополняем/обрезаем до текущего числа уроков.
                        if (Array.isArray(st.periods)) {
                                const cur = st.periods.slice();
                                while (cur.length < slots) cur.push({ start: "", end: "" });
                                bellPeriods = cur.slice(0, slots);
                        } else {
                                bellPeriods = Array.from({ length: slots }, () => ({ start: "", end: "" }));
                        }
                } catch (e) {
                        bellPeriods = Array.from({ length: slots }, () => ({ start: "", end: "" }));
                }
        }
        // bellPeriods всегда имеет ровно slots записей: иначе bind:value
        // в таблице звонков читал несуществующий элемент и «Сохранить»
        // мог записать в базу пустые звонки.
        $: if (bellPeriods.length !== slots) {
                const cur = bellPeriods.slice();
                while (cur.length < slots) cur.push({ start: "", end: "" });
                bellPeriods = cur.slice(0, Math.max(0, slots));
        }
        async function saveSettings() {
                const periods = [];
                for (let i = 0; i < slots; i++) periods.push({ start: normalizeTime(bellPeriods[i]?.start || ""), end: normalizeTime(bellPeriods[i]?.end || "") });
                const st = { days, slots, days_mask: schoolDaysMask, periods };
                await UpdateSchoolSettings(activeSchoolID, JSON.stringify(st));
                bellPeriods = periods;
                flash("Настройки сохранены");
        }
        // Время звонков — всегда 24-часовое ЧЧ:ММ. Терпит ввод «830»,
        // «8:30», «8.30», «08:30»; пустое остаётся пустым.
        function normalizeTime(v) {
                let s = String(v || "").trim().replace(/[.,; ]+/g, ":");
                if (!s) return "";
                const digits = s.replace(/\D/g, "");
                if (!s.includes(":") && (digits.length === 3 || digits.length === 4)) {
                        s = digits.slice(0, digits.length - 2) + ":" + digits.slice(-2);
                }
                const m = s.match(/^(\d{1,2}):(\d{1,2})$/);
                if (!m) return "";
                const h = Math.min(23, +m[1]), mm = Math.min(59, +m[2]);
                return String(h).padStart(2, "0") + ":" + String(mm).padStart(2, "0");
        }
        // Переключение учебного дня (чекбокс в «Ограничениях»): последний
        // включённый день снять нельзя — расписание без дней не строится.
        // Меняется сразу везде: сетка, генерация, печать и PDF.
        function toggleSchoolDay(d, on) {
                const next = on ? (schoolDaysMask | (1 << d)) : (schoolDaysMask & ~(1 << d));
                if (next === 0) { flash("Хотя бы один учебный день должен быть включён"); return; }
                schoolDaysMask = next;
                if (activeSchoolID) {
                        const periods = [];
                        for (let i = 0; i < slots; i++) periods.push({ start: normalizeTime(bellPeriods[i]?.start || ""), end: normalizeTime(bellPeriods[i]?.end || "") });
                        UpdateSchoolSettings(activeSchoolID, JSON.stringify({ days, slots, days_mask: schoolDaysMask, periods })).catch(() => {});
                }
        }
        function onSlotsChange() {
                const cur = bellPeriods.slice();
                while (cur.length < slots) cur.push({ start: "", end: "" });
                bellPeriods = cur.slice(0, slots);
        }
        async function reloadRefs() {
                if (!activeSchoolID) return;
                const res = await Promise.all([
                        ListTeachers(activeSchoolID), ListSubjects(activeSchoolID),
                        ListClasses(activeSchoolID), ListRooms(activeSchoolID),
                        ListLessons(activeSchoolID), ListConstraints(activeSchoolID)
                ]);
                [teachers, subjects, classes, rooms, lessons, constraints] = res.map((x) => x || []);
        }
        async function reloadSchedule() {
                if (!activeSchoolID) return;
                schedule = (await ListSchedule(activeSchoolID)) || [];
                // conflictIDs пересчитывается реактивно из schedule+constraints
        }

        let conflictIDs = new Set();
        // Красным помечаем и двойные брони, и уроки, вставшие на запрещённую
        // ячейку жёсткого ограничения «недоступен» (учитель/класс/кабинет).
        $: conflictIDs = computeConflictIDs(schedule, constraints);
        function violatesUnavailable(c, e) {
                if (c.entity_type === "teacher" ? e.teacher_id !== c.entity_id
                        : c.entity_type === "class" ? e.class_id !== c.entity_id
                        : e.room_id !== c.entity_id) return false;
                if (c.day_of_week != null && e.day_of_week !== c.day_of_week) return false;
                if (c.timeslot_start != null && e.timeslot < c.timeslot_start) return false;
                if (c.timeslot_end != null && e.timeslot > c.timeslot_end) return false;
                return true;
        }
        function computeConflictIDs(schedule, constraints) {
                const ids = new Set();
                // Учитель/кабинет: группируем по (значение, день, слот).
                const maps = { teacher_id: {}, class_id: {}, room_id: {} };
                for (const e of schedule) {
                        const k = e.day_of_week * 1000 + e.timeslot;
                        const tv = e.teacher_id, rv = e.room_id;
                        if (tv) {
                                if (!maps.teacher_id[tv]) maps.teacher_id[tv] = {};
                                if (!maps.teacher_id[tv][k]) maps.teacher_id[tv][k] = [];
                                maps.teacher_id[tv][k].push(e.id);
                        }
                        if (rv) {
                                if (!maps.room_id[rv]) maps.room_id[rv] = {};
                                if (!maps.room_id[rv][k]) maps.room_id[rv][k] = [];
                                maps.room_id[rv][k].push(e.id);
                        }
                        // Класс: пары уроков в одном слоте — конфликт, если
                        // тела пересекаются (целый класс vs его подгруппа);
                        // две подгруппы одного родителя disjoint — можно параллельно.
                        if (!maps.class_id[e.class_id]) maps.class_id[e.class_id] = {};
                        if (!maps.class_id[e.class_id][k]) maps.class_id[e.class_id][k] = [];
                        maps.class_id[e.class_id][k].push(e.id);
                }
                for (const f of ["teacher_id"]) {
                        for (const v in maps[f]) {
                                for (const k in maps[f][v]) {
                                        if (maps[f][v][k].length > 1) maps[f][v][k].forEach((id) => ids.add(id));
                                }
                        }
                }
                // Кабинет: попарно; уроки одной семьи класса (родитель +
                // его подгруппы) могут делить кабинет — половины класса вмещаются
                for (const rv in maps.room_id) {
                        const arr = maps.room_id[rv];
                        for (let i = 0; i < arr.length; i++) {
                                for (let j = i + 1; j < arr.length; j++) {
                                        const a = arr[i], b = arr[j];
                                        const famA = classBody(a.class_id), famB = classBody(b.class_id);
                                        const sameFamily = famA.includes(b.class_id) || famB.includes(a.class_id);
                                        if (!sameFamily) {
                                                ids.add(a.id); ids.add(b.id);
                                        }
                                }
                        }
                }
                // Классы: дубль = один и тот же класс дважды в слоте.
                // Родитель + подгруппа параллельно — законно (деление класса).
                for (const cv in maps.class_id) {
                        for (const k in maps.class_id[cv]) {
                                if (maps.class_id[cv][k].length > 1) maps.class_id[cv][k].forEach((id) => ids.add(id));
                        }
                }
                for (const c of constraints) {
                        if (!c.is_hard) continue;
                        if (c.type !== "teacher_unavailable" && c.type !== "class_unavailable" && c.type !== "room_unavailable") continue;
                        for (const e of schedule) if (violatesUnavailable(c, e)) ids.add(e.id);
                }
                // Делёные уроки: запись половинки, у которой в этом же слоте
                // нет сестринской записи, стоит не параллельно — конфликт.
                const twinOfLesson = {};
                for (const [lid, t] of dividedTwinOf) twinOfLesson[lid] = t.id;
                const halfAt = {};
                for (const e of schedule) {
                        if (!twinOfLesson[e.lesson_id]) continue;
                        const k = e.lesson_id + ":" + e.day_of_week + ":" + e.timeslot;
                        halfAt[k] = (halfAt[k] || 0) + 1;
                }
                for (const e of schedule) {
                        const twinId = twinOfLesson[e.lesson_id];
                        if (!twinId) continue;
                        if (!halfAt[twinId + ":" + e.day_of_week + ":" + e.timeslot]) ids.add(e.id);
                }
                return ids;
        }

        let report = { conflicts: [], unplaced: [], overloads: [], violations: [], classStats: [], reasonGroups: [], totalHours: 0, placedHours: 0, missingHours: 0, placedPct: 100 };
        function computeConflictReport() {
                // Та же логика: учитель/кабинет по значению, классы —
                // попарным пересечением тел (целый класс vs подгруппа).
                const byKey = { teacher_id: {}, class_id: {}, room_id: {} };
                for (const e of schedule) {
                        const key = e.day_of_week * 1000 + e.timeslot;
                        for (const f of ["teacher_id", "room_id"]) {
                                const v = e[f];
                                if (!v) continue;
                                (byKey[f][v + ":" + key] = byKey[f][v + ":" + key] || []).push(e);
                        }
                        const rv = e.room_id;
                        if (rv) (byKey.room_id[rv + ":" + key] = byKey.room_id[rv + ":" + key] || []).push(e);
                }
                const labels = { teacher_id: "Учитель", class_id: "Класс", room_id: "Кабинет" };
                const conflicts = [];
                // Классы: дубль = один и тот же класс дважды в слоте
                // (родитель + подгруппа параллельно — законно).
                for (const cv in byKey.class_id) {
                        for (const key in byKey.class_id[cv]) {
                                const arr = byKey.class_id[cv][key];
                                if (arr.length > 1) {
                                        const kk = Number(key.split(":")[1]);
                                        const day = Math.floor(kk / 1000), slot = kk % 1000;
                                        conflicts.push({
                                                type: "Класс", day, slot,
                                                items: arr.map((e) => ({
                                                        subject: subjName(subjects, e.subject_id),
                                                        who: className(classes, e.class_id)
                                                }))
                                        });
                                }
                        }
                }
                const placedByLesson = {};
                for (const e of schedule) placedByLesson[e.lesson_id] = (placedByLesson[e.lesson_id] || 0) + 1;
                const teacherHours = {};
                for (const e of schedule) teacherHours[e.teacher_id] = (teacherHours[e.teacher_id] || 0) + 1;
                const totalCells = activeDayIdx.length * slots;

                // Занятость по ячейкам: «ид:день:слот» в Set (O(1)-проверка).
                const teacherAt = new Set(), classAt = new Set(), roomAt = new Set();
                // Нагрузка класса с учётом подгрупп (подгруппа суммируется в родителя).
                const classFamilyPlaced = {};
                const familyOf = {};
                for (const c of classes) familyOf[c.id] = c.subgroup_of ? c.subgroup_of : c.id;
                for (const e of schedule) {
                        const k = e.day_of_week + ":" + e.timeslot;
                        if (e.teacher_id) teacherAt.add(e.teacher_id + ":" + k);
                        classAt.add(e.class_id + ":" + k);
                        if (e.room_id) roomAt.add(e.room_id + ":" + k);
                        const fam = familyOf[e.class_id] ?? e.class_id;
                        classFamilyPlaced[fam] = (classFamilyPlaced[fam] || 0) + 1;
                }
                // Ячейки, запрещённые жёсткими ограничениями «недоступен», по сущности (кешируем).
                const bannedCache = new Map();
                function bannedCells(entityType, entityId) {
                        const key = entityType + ":" + entityId;
                        if (bannedCache.has(key)) return bannedCache.get(key);
                        const cells = new Set();
                        for (const c of constraints) {
                                if (!c.is_hard) continue;
                                if (c.type !== entityType + "_unavailable") continue;
                                if (c.entity_type !== entityType || c.entity_id !== entityId) continue;
                                const affectedDays = c.day_of_week != null ? [c.day_of_week] : activeDayIdx;
                                const start = c.timeslot_start ?? 0;
                                const end = c.timeslot_end ?? (slots - 1);
                                for (const d of affectedDays) for (let s = start; s <= end; s++) cells.add(d + ":" + s);
                        }
                        bannedCache.set(key, cells);
                        return cells;
                }
                // Классы, делящие с данным классом одну ячейку (родитель ↔ его подгруппы) —
                // та же семантика, что buildClassSets в солвере.
                function classBody(classId) {
                        const out = [classId];
                        for (const kid of (kidsOf.get(classId) || [])) out.push(kid);
                        const c = classById.get(classId);
                        if (c && c.subgroup_of) out.push(c.subgroup_of);
                        return out;
                }
                // Кабинеты, допустимые для предмета — зеркало allowedRooms() в солвере.
                function allowedRoomIds(subj) {
                        const rt = subj && subj.requires_room_type ? subj.requires_room_type : "any";
                        let out = rooms.filter((r) => rt === "any" || r.room_type === "any" || r.room_type === rt);
                        if (out.length === 0 && rooms.length > 0) out = rooms.slice();
                        return out;
                }
                // Диагностика как в aSc/FET: проходим ВСЕ ячейки недели и считаем,
                // чем занята каждая — чтобы сказать не «не размещено», а ПОЧЕМУ.
                function unplacedDiagnose(l) {
                        const subj = subjectById.get(l.subject_id);
                        const body = classBody(l.class_id);
                        const allowed = allowedRoomIds(subj);
                        const bt = bannedCells("teacher", l.teacher_id);
                        const bcs = body.map((b) => bannedCells("class", b));
                        let free = 0, busyT = 0, busyC = 0, busyR = 0, banned = 0;
                        for (const d of activeDayIdx) {
                                for (let s = 0; s < slots; s++) {
                                        const k = d + ":" + s;
                                        if (teacherAt.has(l.teacher_id + ":" + k)) { busyT++; continue; }
                                        if (body.some((b) => classAt.has(b + ":" + k))) { busyC++; continue; }
                                        if (bt.has(k)) { banned++; continue; }
                                        if (bcs.some((set) => set.has(k))) { banned++; continue; }
                                        if (!allowed.some((r) => !roomAt.has(r.id + ":" + k))) { busyR++; continue; }
                                        free++;
                                }
                        }
                        return { free, busyT, busyC, busyR, banned, total: totalCells, subj };
                }
                // Причина неразмещения урока — конкретная, с числами по ячейкам.
                function unplacedReason(l) {
                        const dg = unplacedDiagnose(l);
                        const t = teacherById.get(l.teacher_id);
                        const c = classById.get(l.class_id);
                        if (rooms.length === 0) return "В школе не задано ни одного кабинета — размещать некуда";

                        // Делёная пара: половинки ходят только вместе, одна без другой не встаёт.
                        const twin = dividedTwinOf.get(l.id);
                        if (twin && (placedByLesson[twin.id] || 0) < (twin.hours_per_week || 0)) {
                                const twinCls = classById.get(twin.class_id);
                                return `Делёный урок: парная половина для ${twinCls ? twinCls.name : "?"} не размещена — половинки ходят только парой`;
                        }

                        if (dg.free > 0) {
                                return `Есть ${dg.free} свободных окон из ${dg.total}, но солвер не успел их занять за отведённое время`;
                        }
                        // Свободных окон нет — разбираем, чем занята неделя.
                        const parts = [];
                        if (dg.busyT) parts.push(`учитель занят в ${dg.busyT}`);
                        if (dg.busyC) parts.push(`класс занят в ${dg.busyC}`);
                        if (dg.busyR) parts.push(`нет свободного кабинета в ${dg.busyR}`);
                        if (dg.banned) parts.push(`запрещено ограничениями в ${dg.banned}`);
                        const detail = parts.length ? " (" + parts.join(", ") + ")" : "";
                        const top = Math.max(dg.busyT, dg.busyC, dg.busyR, dg.banned);
                        let head = `Свободных окон нет ни в одной из ${dg.total} ячеек`;
                        if (top === dg.busyT) head = `Учитель ${t ? t.name : "?"} занят во всех ${dg.total} ячейках`;
                        else if (top === dg.busyC) head = `Класс ${c ? c.name : "?"} занят во всех ${dg.total} ячейках`;
                        else if (top === dg.busyR) head = `Нет свободного кабинета ни в одну из ${dg.total} ячеек`;
                        else if (top === dg.banned) head = `Все ${dg.total} ячеек закрыты ограничениями «недоступен»`;
                        return head + detail;
                }
                const unplaced = lessons.filter((l) => (placedByLesson[l.id] || 0) < (l.hours_per_week || 0))
                        .map((l) => ({ subject: subjName(subjects, l.subject_id), cls: className(classes, l.class_id), need: l.hours_per_week, got: placedByLesson[l.id] || 0, reason: unplacedReason(l) }));

                // ---- Сводка для панели расстановки ----
                const totalHours = lessons.reduce((n, l) => n + (l.hours_per_week || 0), 0);
                const placedHours = schedule.length;
                const missingHours = Math.max(0, totalHours - placedHours);
                const placedPct = totalHours ? Math.round((placedHours / totalHours) * 100) : 100;
                const classNeeds = {};
                for (const l of lessons) {
                        const fam = familyOf[l.class_id] ?? l.class_id;
                        classNeeds[fam] = (classNeeds[fam] || 0) + (l.hours_per_week || 0);
                }
                const classStats = Object.keys(classNeeds).map((id) => {
                        const cid = Number(id);
                        const need = classNeeds[id];
                        const placed = classFamilyPlaced[cid] || 0;
                        return { name: className(classes, cid), need, placed, missing: Math.max(0, need - placed) };
                }).sort((a, b) => b.missing - a.missing || a.name.localeCompare(b.name));
                // Одинаковые причины сворачиваем в один пункт со счётчиком.
                const reasonGroups = [];
                for (const u of unplaced) {
                        let g = reasonGroups.find((x) => x.reason === u.reason);
                        if (!g) { g = { reason: u.reason, items: [] }; reasonGroups.push(g); }
                        g.items.push(u);
                }
                reasonGroups.sort((a, b) => b.items.length - a.items.length);
                const overloads = teachers.filter((t) => t.max_hours_per_week && (teacherHours[t.id] || 0) > t.max_hours_per_week)
                        .map((t) => ({ name: t.name, max: t.max_hours_per_week, got: teacherHours[t.id] || 0 }));
                const violations = [];
                for (const c of constraints) {
                        if (!c.is_hard) continue;
                        if (c.type !== "teacher_unavailable" && c.type !== "class_unavailable" && c.type !== "room_unavailable") continue;
                        for (const e of schedule) {
                                if (violatesUnavailable(c, e)) {
                                        violations.push({ day: e.day_of_week, slot: e.timeslot, what: constraintTypeLabel(c.type) + " · " + constraintEntityLabel(c) });
                                }
                        }
                }
                // Делёные уроки: половинки без пары в своём слоте.
                for (const e of schedule) {
                        const twinId = dividedTwinOf.get(e.lesson_id)?.id || 0;
                        if (!twinId) continue;
                        const hasTwin = schedule.some((x) => x.lesson_id === twinId
                                && x.day_of_week === e.day_of_week && x.timeslot === e.timeslot);
                        if (!hasTwin) {
                                conflicts.push({
                                        type: "Деление",
                                        day: e.day_of_week, slot: e.timeslot,
                                        items: [{ subject: subjName(subjects, e.subject_id), who: className(classes, e.class_id) + " — половинка без пары" }]
                                });
                        }
                }
                return { conflicts, unplaced, overloads, violations, classStats, reasonGroups, totalHours, placedHours, missingHours, placedPct };
        }
        // Same syntactic-dependency rule as buildGrid above: pass the state
        // this report is derived from as explicit arguments.
        $: report = computeConflictReport(schedule, lessons, subjects, teachers, classes, rooms, constraints, slots, activeDayIdx);

        let history = [];
        async function pushHistory() {
                history.push(JSON.parse(JSON.stringify(schedule)));
                if (history.length > 50) history.shift();
        }
        async function undo() {
                if (!history.length) return;
                const prev = history.pop();
                await ReplaceSchedule(activeSchoolID, prev);
                await reloadSchedule();
                flash("Отменено");
        }

        $: if (activeSchoolID) { reloadRefs(); reloadSchedule(); }

        async function addTeacher() {
                if (!t.name.trim()) { flash("Введите имя учителя"); return; }
                await CreateTeacher({ ...t, school_id: activeSchoolID });
                t = { name: "", short_name: "", max_hours_per_week: 30 };
                await reloadRefs();
        }
        async function addSubject() {
                if (!s.name.trim()) { flash("Введите название предмета"); return; }
                await CreateSubject({ ...s, school_id: activeSchoolID });
                s = { name: "", short_name: "", requires_room_type: "any" };
                await reloadRefs();
        }
        async function addClass() {
                if (!c.name.trim()) { flash("Введите название класса"); return; }
                await CreateClass({ ...c, school_id: activeSchoolID });
                c = { name: "", room_id: 0, subgroup_of: null };
                await reloadRefs();
        }
        async function addRoom() {
                if (!r.name.trim()) { flash("Введите название кабинета"); return; }
                await CreateRoom({ ...r, school_id: activeSchoolID });
                r = { name: "", room_type: "any" };
                await reloadRefs();
        }
        async function addLesson() {
                if (!l.class_id || !l.subject_id || !l.teacher_id) { flash("Выберите класс, предмет и учителя"); return; }
                const hours = Math.max(1, Math.min(40, l.hours_per_week || 1));
                await CreateLesson({ ...l, school_id: activeSchoolID, hours_per_week: hours });
                l = { class_id: 0, subject_id: 0, teacher_id: 0, hours_per_week: 1 };
                await reloadRefs();
        }
        async function addLessonForClass() {
                if (!curClass || !l.subject_id || !l.teacher_id) { flash("Выберите класс, предмет и учителя"); return; }
                await CreateLesson({ school_id: activeSchoolID, class_id: curClass, subject_id: l.subject_id, teacher_id: l.teacher_id, hours_per_week: l.hours_per_week || 1 });
                l = { class_id: 0, subject_id: 0, teacher_id: 0, hours_per_week: 1 };
                await reloadRefs();
        }
        async function updateLesson(x) {
                try {
                        await UpdateLesson({ id: x.id, school_id: x.school_id, class_id: x.class_id, subject_id: x.subject_id, teacher_id: x.teacher_id, hours_per_week: x.hours_per_week || 1 });
                } catch (e) { flash("Ошибка обновления урока: " + (e && e.message ? e.message : e)); }
        }
        async function removeLesson(id) { if (!await confirmAction("Удалить урок?")) return; await pushHistory(); await DeleteLesson(id); await reloadRefs(); flash("Урок удалён"); }
        // window.confirm не работает в Wails (WebView2 гасит JS-диалоги),
        // поэтому подтверждение — собственное модальное окно, одинаковое
        // и в браузерном, и в десктопном режиме.
        let confirmBox = null; // { message, confirmLabel }
        let confirmResolve = null;
        function confirmAction(message, confirmLabel = "Удалить") {
                return new Promise((resolve) => {
                        confirmBox = { message, confirmLabel };
                        confirmResolve = resolve;
                });
        }
        function settleConfirm(answer) {
                if (confirmResolve) confirmResolve(answer);
                confirmBox = null;
                confirmResolve = null;
        }
        async function removeTeacher(id) { if (!await confirmAction("Удалить учителя? Все его уроки тоже будут удалены.")) return; await pushHistory(); await DeleteTeacher(id); await reloadRefs(); flash("Учитель удалён"); }
        async function removeSubject(id) { if (!await confirmAction("Удалить предмет? Связанные уроки тоже будут удалены.")) return; await pushHistory(); await DeleteSubject(id); await reloadRefs(); flash("Предмет удалён"); }
        async function removeClass(id) { if (!await confirmAction("Удалить класс? Все уроки и расписание класса будут удалены.")) return; await pushHistory(); await DeleteClass(id); await reloadRefs(); flash("Класс удалён"); }
        async function removeRoom(id) { if (!await confirmAction("Удалить кабинет? Связанные ячейки расписания тоже будут удалены.")) return; await pushHistory(); await DeleteRoom(id); await reloadRefs(); await reloadSchedule(); flash("Кабинет удалён"); }
        async function removeConstraint(id) { await DeleteConstraint(id); await reloadRefs(); flash("Ограничение удалено"); }
        async function removeEntry(id) { await pushHistory(); await DeleteScheduleEntry(id); await reloadSchedule(); }

        async function generate() {
                await pushHistory();
                if (!lessons.length) { flash("Нет уроков в учебном плане — добавьте их на вкладке «Уроки»."); history.pop(); return; }
                const occurrences = lessons.reduce((a, l) => a + (l.hours_per_week || 0), 0);
                if (!usePrecise && occurrences > 200) {
                        usePrecise = true;
                        if (hasPreciseSolver) {
                                flash("Крупная школа: включён точный CP-SAT (OR-Tools).");
                        } else {
                                flash("Крупная школа: OR-Tools недоступен в этой сборке — используется быстрый эвристический решатель (может не разместить все уроки).");
                        }
                }
                try {
                        genResult = (usePrecise ? await GeneratePrecise(activeSchoolID, days, slots, schoolDaysMask) : await Generate(activeSchoolID, days, slots, schoolDaysMask)) || {};
                        await reloadSchedule();
                        flash(`Размещено ${genResult.placed}/${genResult.total}, нарушений (мягких): ${genResult.violations}`);
                } catch (e) {
                        flash("Ошибка генерации: " + (e && e.message ? e.message : e));
                }
        }
        async function generatePrecise() {
                await pushHistory();
                if (!lessons.length) { flash("Нет уроков в учебном плане — добавьте их на вкладке «Уроки»."); history.pop(); return; }
                try {
                        genResult = await GeneratePrecise(activeSchoolID, days, slots, schoolDaysMask);
                        await reloadSchedule();
                        flash(`CP-SAT: размещено ${genResult.placed}/${genResult.total}`);
                } catch (e) {
                        flash("Ошибка генерации: " + (e && e.message ? e.message : e));
                }
        }
        // applyMove performs a true optimistic move/swap.
        //
        // Old behavior: await backend → update local schedule → reload from
        // backend. That meant the cell visually "froze" for the whole Wails +
        // SQLite round trip and, if the user navigated away during that window,
        // the optimistic update never rendered — so they had to switch tabs to
        // see the change.
        //
        // New behavior:
        //   1. Snapshot the current schedule (for rollback) and update the
        //      local `schedule` array IMMEDIATELY so the grid re-renders on the
        //      very next microtask, before any IPC.
        //   2. THEN call the backend. The DB is the source of truth, so on the
        //      error path we restore the snapshot so the UI matches the DB.
        //   3. On success we DO NOT call reloadSchedule() — the optimistic
        //      state already matches what the DB just persisted, so a fresh
        //      round trip would only waste time and could momentarily flicker
        //      the table. This is the key fix for the "need to navigate away
        //      and back" symptom.
        async function applyMove(id, kind, rowId, day, slot) {
                const src = schedule.find(en => en.id === id);
                if (!src) {
                        flash("⚠ Не найден исходный урок — возможно, расписание изменилось. Обновите вкладку.");
                        return;
                }
                // Source's own row identifier (its class/teacher/room id).
                const srcRowId = kind === "class" ? src.class_id : kind === "teacher" ? src.teacher_id : src.room_id;
                // Подгруппа может двигаться в строке своего родительского класса.
                const srcParent = kind === "class" ? subParentOf(srcRowId) : null;
                if (srcRowId !== rowId && srcParent !== rowId) {
                        const rowKindLabel = kind === "class" ? "классами" : kind === "teacher" ? "учителями" : "кабинетами";
                        flash(`⚠ Нельзя перемещать урок между ${rowKindLabel} (это нарушило бы структуру расписания). Только в пределах одной строки.`);
                        return;
                }
                // Find target in same row at the destination cell.
                // Ячейка класса вмещает ДО ДВУХ уроков: целоклассовый +
                // подгруппа, или две подгруппы параллельно (деление класса).
                const target = schedule.find(en => {
                        if (en.id === id) return false;
                        if (en.day_of_week !== day || en.timeslot !== slot) return false;
                        if (kind === "class") return en.class_id === rowId || subParentOf(en.class_id) === rowId;
                        if (kind === "teacher") return en.teacher_id === rowId;
                        return en.room_id === rowId;
                });
                const cellCount = schedule.filter(en => en.id !== id && en.day_of_week === day && en.timeslot === slot &&
                        (en.class_id === rowId || subParentOf(en.class_id) === rowId)).length;
                if (kind === "class" && cellCount >= 2) {
                        flash("⚠ Ячейка заполнена: максимум два урока (класс + подгруппа).");
                        return;
                }

                // Делёный урок (модель aSc): половинки одного предмета у
                // подгрупп одного родителя перемещаются ПАРОЙ, иначе
                // параллельность ломается (половинка остаётся без пары).
                const twinLesson = kind === "class" ? dividedTwinLesson(src.lesson_id) : null;
                if (kind !== "class" && twinLesson) {
                        flash("⚠ Делёный урок перемещается парой — двигайте его в сетке класса.");
                        return;
                }
                if (twinLesson) {
                        const twinEntry = schedule.find((en) => en.lesson_id === twinLesson.id
                                && en.day_of_week === src.day_of_week && en.timeslot === src.timeslot);
                        if (twinEntry) {
                                const famCount = schedule.filter((en) => en.id !== src.id
                                        && en.day_of_week === day && en.timeslot === slot
                                        && (en.class_id === rowId || subParentOf(en.class_id) === rowId)).length;
                                if (famCount > 0) {
                                        flash("⚠ Делёный урок перемещается парой — целевая ячейка должна быть пустой.");
                                        return;
                                }
                                const snapshot = schedule.slice();
                                await pushHistory();
                                schedule = schedule.map((en) => (en.id === src.id || en.id === twinEntry.id)
                                        ? { ...en, day_of_week: day, timeslot: slot }
                                        : en);
                                flash(`✓ Делёный урок перемещён парой → ${dayName(day)} П${slot + 1}`);
                                try {
                                        await MoveEntry(src.id, day, slot);
                                        await MoveEntry(twinEntry.id, day, slot);
                                } catch (err) {
                                        schedule = snapshot;
                                        flash(`⚠ Не удалось переместить пару: ${err && err.message ? err.message : err}`);
                                        return;
                                }
                                try { schedule = (await ListSchedule(activeSchoolID)) || []; } catch (e) { /* optimistic state */ }
                                return;
                        }
                        // Пары в исходном слоте нет (состояние уже разъединено,
                        // подсвечено красным) — одиночный перенос позволяет
                        // «приклеить» половинку обратно к её паре.
                }

                // Snapshot for rollback if the backend rejects the change.
                const snapshot = schedule.slice();
                await pushHistory();

                // 1. Optimistic UI update — fires reactivity immediately, before
                //    any IPC. This is the fix for the "drag does nothing until
                //    you switch tabs" bug.
                if (target) {
                        schedule = schedule.map((en) => en.id === src.id
                                ? { ...en, day_of_week: day, timeslot: slot }
                                : en.id === target.id
                                ? { ...en, day_of_week: src.day_of_week, timeslot: src.timeslot }
                                : en);
                        // conflictIDs пересчитывается реактивно из schedule+constraints
                        flash(`✓ Поменяли местами: ${cellLabelShort(src)} (${dayName(src.day_of_week)} П${src.timeslot + 1}) ⟷ ${cellLabelShort(target)} (${dayName(day)} П${slot + 1})`);
                } else {
                        schedule = schedule.map((en) => en.id === id
                                ? { ...en, day_of_week: day, timeslot: slot }
                                : en);
                        // conflictIDs пересчитывается реактивно из schedule+constraints
                        flash(`✓ Перемещено: ${cellLabelShort(src)} → ${dayName(day)} П${slot + 1}`);
                }

                // 2. Persist to the backend. On error, roll back to the
                //    pre-move state so the grid matches the DB again.
                try {
                        if (target) {
                                await SwapEntries(src.id, day, slot, target.id, src.day_of_week, src.timeslot);
                        } else {
                                await MoveEntry(id, day, slot);
                        }
                } catch (err) {
                        // Restore the snapshot and let the user know the move
                        // was rejected (e.g. DB constraint, lost race with
                        // another edit). Do NOT silently overwrite — the user
                        // must see their move was rolled back.
                        schedule = snapshot;
                        // conflictIDs пересчитывается реактивно из schedule+constraints
                        flash(`⚠ Не удалось переместить: ${err && err.message ? err.message : err}`);
                        return;
                }
                // 3. Success. The optimistic state already matches what the DB
                //    just persisted, but we still quietly re-read the schedule
                //    once so the UI is guaranteed to equal the DB (covers any
                //    serialization nuance). The grid is fully reactive now, so
                //    this refresh renders instantly instead of "freezing" like
                //    the old template did.
                try {
                        schedule = (await ListSchedule(activeSchoolID)) || [];
                        // conflictIDs пересчитывается реактивно из schedule+constraints
                } catch (e) { /* keep the optimistic state on refresh failure */ }
        }
        // ------------------------------------------------------------------
        // Drag & Drop engine (v2).
        //
        // Must work inside the Wails webview (WebView2 / WKWebView) with
        // mouse AND touch/pen. Key properties:
        //   * setPointerCapture on the pressed cell → every later pointer
        //     event is delivered to us no matter where the pointer goes;
        //   * hit-testing is done with getBoundingClientRect scans, never
        //     elementFromPoint (unreliable during pointer capture);
        //   * the cell under the pointer is highlighted (drop-target) so the
        //     user SEES where the lesson will land;
        //   * movement < 6px counts as a click (select / place-to-cell);
        //   * Escape cancels a drag in progress;
        //   * near the viewport edges we auto-scroll the grid.
        // ------------------------------------------------------------------
        let selectedEntry = null;
        let pendingDrag = null;   // { id, kind, rowId, day, slot, label, pointerId }
        let startPos = null;      // pointer position where the drag started
        let dragging = false;
        let ghost = null;         // { label, x, y } — floating label under the cursor
        let dropTarget = null;    // { rowId, day, slot } — highlighted destination cell
        let lastPointer = { x: 0, y: 0 };
        let dropCheckPending = false;

        // Engine-independent hit-test: find the schedule cell whose bounding
        // rect contains the pointer.
        function hitCell(x, y) {
                const tds = document.querySelectorAll("td[data-cell]");
                for (let i = 0; i < tds.length; i++) {
                        const r = tds[i].getBoundingClientRect();
                        if (x >= r.left && x <= r.right && y >= r.top && y <= r.bottom) return tds[i];
                }
                return null;
        }

        function readCellAttrs(td) {
                const rowId = parseInt(td.getAttribute("data-row"));
                const day = parseInt(td.getAttribute("data-day"));
                const slot = parseInt(td.getAttribute("data-slot"));
                if (isNaN(rowId) || isNaN(day) || isNaN(slot)) return null;
                return { rowId, day, slot };
        }

        function setDropTargetFromPoint(x, y) {
                if (!pendingDrag) { dropTarget = null; return; }
                const td = hitCell(x, y);
                const dst = td ? readCellAttrs(td) : null;
                // Never highlight the source cell itself.
                if (dst && dst.rowId === pendingDrag.rowId && dst.day === pendingDrag.day && dst.slot === pendingDrag.slot) {
                        dropTarget = null;
                        return;
                }
                dropTarget = dst;
        }

        // Throttle hit-testing to one scan per animation frame.
        function scheduleDropCheck() {
                if (dropCheckPending) return;
                dropCheckPending = true;
                requestAnimationFrame(() => {
                        dropCheckPending = false;
                        setDropTargetFromPoint(lastPointer.x, lastPointer.y);
                });
        }

        // Auto-scroll window (vertical) and the grid container (horizontal)
        // while dragging near an edge. The loop self-stops when dragging ends.
        function autoScrollTick() {
                if (!dragging) return;
                const m = 48, sp = 14;
                if (lastPointer.y < m) window.scrollBy(0, -sp);
                else if (lastPointer.y > window.innerHeight - m) window.scrollBy(0, sp);
                const sc = document.querySelector(".grid-scroll");
                if (sc) {
                        const r = sc.getBoundingClientRect();
                        if (lastPointer.x > r.left && lastPointer.x < r.right) {
                                if (lastPointer.x < r.left + m) sc.scrollLeft -= sp;
                                else if (lastPointer.x > r.right - m) sc.scrollLeft += sp;
                        }
                }
                requestAnimationFrame(autoScrollTick);
        }

        function onPointerDown(e, cell, kind, rowId, day, slot) {
                if (e.pointerType === "mouse" && e.button !== 0) return;
                if (e.target && e.target.closest && e.target.closest(".cell-x")) return;
                e.preventDefault();
                pendingDrag = { id: cell ? cell.id : null, kind, rowId, day, slot, label: cell ? cell.label : "", pointerId: e.pointerId };
                dragging = false;
                ghost = null;
                dropTarget = null;
                startPos = { x: e.clientX, y: e.clientY };
                lastPointer = { x: e.clientX, y: e.clientY };
                // Capture the pointer on the pressed cell: guarantees that
                // pointermove/pointerup keep coming even when the cursor
                // leaves the <td> (this is the piece the previous attempt was
                // missing — without it some drags silently died mid-move).
                try {
                        if (e.pointerId !== undefined && e.target && e.target.setPointerCapture) {
                                e.target.setPointerCapture(e.pointerId);
                        }
                } catch (err) { /* non-fatal */ }
        }
        function onPointerMove(e) {
                if (!pendingDrag) return;
                if (pendingDrag.pointerId !== undefined && e.pointerId !== undefined && e.pointerId !== pendingDrag.pointerId) return;
                lastPointer = { x: e.clientX, y: e.clientY };
                const dx = e.clientX - startPos.x, dy = e.clientY - startPos.y;
                if (!dragging && Math.hypot(dx, dy) > 6) {
                        dragging = true;
                        requestAnimationFrame(autoScrollTick);
                }
                if (dragging) {
                        ghost = { label: pendingDrag.label, x: e.clientX, y: e.clientY };
                        scheduleDropCheck();
                }
        }
        async function onPointerUp(e) {
                if (!pendingDrag) { dragging = false; ghost = null; dropTarget = null; return; }
                const pd = pendingDrag;
                const wasDrag = dragging;
                const dst = dropTarget;
                pendingDrag = null;
                dragging = false;
                ghost = null;
                dropTarget = null;
                if (wasDrag && pd.id) {
                        // Prefer the highlighted target; fall back to a fresh
                        // hit-test at the release point.
                        let target = dst;
                        if (!target && e.clientX !== undefined) {
                                const td = hitCell(e.clientX, e.clientY);
                                target = td ? readCellAttrs(td) : null;
                        }
                        if (target) {
                                if (target.day === pd.day && target.slot === pd.slot && target.rowId === pd.rowId) return;
                                try { await applyMove(pd.id, pd.kind, target.rowId, target.day, target.slot); }
                                catch (err) { flash("Ошибка: " + (err && err.message ? err.message : err)); }
                        }
                        return;
                }
                // Click without drag: select or click-to-move
                if (!pd.id) {
                        if (selectedEntry) {
                                try { await applyMove(selectedEntry.id, pd.kind, pd.rowId, pd.day, pd.slot); }
                                catch (err) { flash("Ошибка: " + (err && err.message ? err.message : err)); }
                                selectedEntry = null;
                        }
                        return;
                }
                if (selectedEntry && selectedEntry.id !== pd.id) {
                        try { await applyMove(selectedEntry.id, pd.kind, pd.rowId, pd.day, pd.slot); }
                        catch (err) { flash("Ошибка: " + (err && err.message ? err.message : err)); }
                        selectedEntry = null;
                } else {
                        selectedEntry = (selectedEntry && selectedEntry.id === pd.id) ? null : { id: pd.id };
                }
        }
        function onPointerCancel() {
                try {
                        if (pendingDrag && pendingDrag.pointerId !== undefined) {
                                const el = document.elementFromPoint(lastPointer.x, lastPointer.y);
                                if (el && el.releasePointerCapture) el.releasePointerCapture(pendingDrag.pointerId);
                        }
                } catch (err) { /* ignore */ }
                pendingDrag = null; dragging = false; ghost = null; dropTarget = null; startPos = null;
        }
        function onGlobalKeydown(e) {
                if (e.key === "Escape") {
                        if (dragging || pendingDrag) { pendingDrag = null; dragging = false; ghost = null; dropTarget = null; }
                        else if (selectedEntry) selectedEntry = null;
                }
        }

        // Attach pointer listeners to window in the CAPTURE phase (a table
        // cell can otherwise consume pointerup in the embedded webview) and
        // clean up on destroy.
        function attachDragListeners() {
                document.addEventListener("pointermove", onPointerMove, true);
                document.addEventListener("pointerup", onPointerUp, true);
                document.addEventListener("pointercancel", onPointerCancel, true);
                window.addEventListener("keydown", onGlobalKeydown, true);
        }
        function detachDragListeners() {
                document.removeEventListener("pointermove", onPointerMove, true);
                document.removeEventListener("pointerup", onPointerUp, true);
                document.removeEventListener("pointercancel", onPointerCancel, true);
                window.removeEventListener("keydown", onGlobalKeydown, true);
        }
        onMount(() => { attachDragListeners(); detectPreciseSolver(); return () => detachDragListeners(); });
        function cellAt(kind, id, day, slot) {
                const arr = scheduleIndex.idx.get(cellKey(kind, id, day, slot));
                if (!arr || !arr.length) return null;
                const e = arr[0];
                return {
                        id: e.id,
                        subject_id: e.subject_id,
                        teacher_id: e.teacher_id,
                        room_id: e.room_id,
                        conflict: conflictIDs.has(e.id),
                        label: subjName(subjects, e.subject_id) + " (" + teachName(teachers, e.teacher_id) + ") " + (roomById.get(e.room_id)?.name || "")
                };
        }

        // cellSubs: остальные уроки в той же ячейке (параллельные подгруппы,
        // либо дубль-конфликт того же учителя/кабинета). Первая запись под
        // ключом — сама ячейка (cellAt) и исключается; для класса ячейка
        // родителя дополнительно читает ключи его подгрупп.
        function cellSubs(kind, id, day, slot) {
                const { idx, pos } = scheduleIndex;
                const self = idx.get(cellKey(kind, id, day, slot)) || [];
                const excludeId = self.length ? self[0].id : -1;
                if (kind !== "class") {
                        return self.filter((en) => en.id !== excludeId);
                }
                const merged = self.slice();
                for (const kid of (kidsOf.get(id) || [])) {
                        const kidArr = idx.get(cellKey("class", kid, day, slot));
                        if (kidArr) for (const en of kidArr) merged.push(en);
                }
                merged.sort((a, b) => pos.get(a.id) - pos.get(b.id));
                return merged.filter((en) => en.id !== excludeId);
        }
        // Тело класса: сам класс + его подгруппы.
        function classBody(classId) {
                return [classId, ...(kidsOf.get(classId) || [])];
        }
        function subParentOf(classId) {
                return classById.get(classId)?.subgroup_of || null;
        }
        // Делёная пара (модель aSc): у урока подгруппы есть «сестринский»
        // урок ТОГО ЖЕ предмета у другой подгруппы того же родителя.
        // Половинки обязаны стоять параллельно и двигаться как группа.
        // Индекс строится реактивно — раньше dividedTwinLesson делал
        // два Array.find по урокам на каждый вызов (квадрат в подсчёте
        // конфликтов).
        let dividedTwinOf = new Map();
        $: {
                void classes; // зависимость: subParentOf читает classById
                const m = new Map();
                const byKey = new Map(); // `${parent}|${subject}` → первый урок семьи
                for (const l of lessons) {
                        const parent = subParentOf(l.class_id);
                        if (!parent) continue;
                        const key = parent + "|" + l.subject_id;
                        const first = byKey.get(key);
                        if (first) {
                                // пара ссылается друг на друга; при 3+ подгруппах
                                // первый остаётся со второй (как в исходном поиске)
                                m.set(l.id, first);
                                if (!m.has(first.id)) m.set(first.id, l);
                        } else {
                                byKey.set(key, l);
                        }
                }
                dividedTwinOf = m;
        }
        function dividedTwinLesson(lessonId) {
                return dividedTwinOf.get(lessonId) || null;
        }
        // Компактная подпись для обзора «вся школа»: краткое имя предмета
        // (short_name, если задан) — полный контекст в подсказке ячейки.
        function isSubRow(id) {
                const c = classById.get(id);
                return !!(c && c.subgroup_of);
        }
        // Имя родителя, если класс — подгруппа.
        function subParentName(id) {
                const c = classById.get(id);
                if (!c || !c.subgroup_of) return null;
                return classById.get(c.subgroup_of)?.name || null;
        }
        function subjShort(list, id) {
                const s = subjectById.get(id);
                if (!s) return "?";
                return s.short_name || s.name;
        }
        // Клик по заголовку мини-таблицы — открыть этот класс отдельно
        // (в подробном виде с drag&drop), на нужной странице пагинации.
        function focusRow(id) {
                viewMode = "class";
                const idx = classes.findIndex((c) => c.id === id);
                if (idx >= 0) classPage = Math.floor(idx / classesPerPage);
        }

        async function exportJSON() {
                const snap = await ExportAll(activeSchoolID);
                await saveFile("school.json", JSON.stringify(snap, null, 2), "application/json", false);
        }
        async function importJSON(ev) {
                const text = await ev.target.files[0].text();
                const restored = await ImportAll(text);
                // An imported backup can create a new school with a different
                // local ID. Show that restored school immediately.
                if (restored && restored.id) activeSchoolID = restored.id;
                await loadSchools();
                await reloadRefs();
                await reloadSchedule();
                flash("Импортировано");
        }
        async function exportCSV() {
                const csv = await ScheduleCSV(activeSchoolID, days, slots);
                await saveFile("schedule.csv", csv, "text/csv", false);
        }

        // exportPDF asks the Go backend to render the PDF (via internal/pdf +
        // signintech/gopdf + an embedded DejaVu Sans font for Cyrillic) and
        // returns it as base64. We pass it straight to the existing SaveExport
        // flow that writes to ~/Downloads.
        //
        // Old behavior (jsPDF) was unreliable inside the Wails WebKit webview
        // (datauristring output could yield a syntactically valid but empty
        // PDF) and the default fonts had no Cyrillic glyphs. Moving the
        // rendering to Go fixes both.
        async function exportPDF() {
                try {
                        const options = {
                                mode: exportMode,
                                page_size: pageSize,
                                orientation: orientation,
                                show_teacher: pdfShowTeacher,
                                show_room: pdfShowRoom,
                                weekdays_only: pdfWeekdaysOnly,
                                bw: pdfBW,
                                days: days,
                                slots: slots,
                                days_mask: schoolDaysMask,
                        };
                        const b64 = await ExportPDF(activeSchoolID, JSON.stringify(options));
                        if (!b64) throw new Error("PDF не содержит данных");
                        const defName = "Расписание_" + fileSlug(pdfSchoolName()) + "_" + new Date().toISOString().slice(0, 10) + ".pdf";
                        try {
                                await saveFile(defName, b64, "application/pdf", true);
                        } catch (err) {
                                flash("Ошибка сохранения PDF: " + (err && err.message ? err.message : err));
                        }
                } catch (e) {
                        flash("Ошибка генерации PDF: " + (e && e.message ? e.message : e));
                }
        }
        // Насыщенные цвета чипов — тот же список, что в app.go для PDF.
        // Белый текст читается на любом из этих цветов.
        function subjectColor(sid) {
                const palette = ["#3b82f6","#10b981","#f59e0b","#8b5cf6","#ec4899","#06b6d4","#6366f1","#14b8a6","#f97316","#a855f7","#0ea5e9","#ca8a04","#65a30d","#db2777"];
                return palette[Math.abs((sid || 0)) % palette.length];
        }
        function pdfSchoolName() {
                return (schools.find(s => s.id === activeSchoolID)?.name) || "Школа";
        }
        function fileSlug(s) {
                return String(s).replace(/[\\/:*?"<>|]/g, "_").replace(/\s+/g, "_");
        }

        let saveModal = null; // { filename, content, mime, isBase64 }
        async function saveFile(filename, content, mime, isBase64) {
                saveModal = { filename, content, mime, isBase64 };
        }
        async function confirmSave() {
                if (!saveModal) return;
                const name = saveModal.filename || "export";
                const b64 = saveModal.isBase64 ? saveModal.content : btoa(unescape(encodeURIComponent(saveModal.content)));
                try {
                        const path = await SaveExport(name, b64);
                        flash("Сохранено: " + path);
                } catch (e) {
                        flash("Ошибка сохранения: " + (e && e.message ? e.message : e));
                }
                saveModal = null;
        }
        function cancelSave() { saveModal = null; }
        // Справочники гоняются обычным JSON, не CSV: меньше зависимостей,
        // формат совпадает с тем, что и так ходит через API. В файл пишем
        // имена вместо id — файл переносим между школами и базами.
        async function downloadRefsJSON(entity) {
                let data;
                if (entity === "teachers") data = teachers.map((t) => ({ name: t.name, short_name: t.short_name || "", max_hours_per_week: t.max_hours_per_week || 0 }));
                else if (entity === "subjects") data = subjects.map((s) => ({ name: s.name, short_name: s.short_name || "", requires_room_type: s.requires_room_type || "any" }));
                else if (entity === "rooms") data = rooms.map((r) => ({ name: r.name, room_type: r.room_type || "any" }));
                else if (entity === "classes") data = classes.map((c) => ({ name: c.name, room: c.room_id ? (roomName(rooms, c.room_id) || "") : "", subgroup_of: c.subgroup_of ? (className(classes, c.subgroup_of) || "") : "" }));
                else if (entity === "lessons") data = lessons.map((l) => ({ class: className(classes, l.class_id), subject: subjName(subjects, l.subject_id), teacher: teachName(teachers, l.teacher_id), hours_per_week: l.hours_per_week }));
                else if (entity === "periods") data = bellPeriods.slice(0, slots).map((p) => ({ start: p.start || "", end: p.end || "" }));
                else return;
                await saveFile(entity + ".json", JSON.stringify(data, null, 2), "application/json", false);
        }
        async function importRefsJSON(entity, e) {
                const file = e.target.files && e.target.files[0];
                if (!file) return;
                try {
                        const items = JSON.parse(await file.text());
                        if (!Array.isArray(items)) throw new Error("ожидался JSON-массив");
                        let n = 0;
                        for (const it of items) {
                                if (entity === "teachers") {
                                        if (!it.name) continue;
                                        await CreateTeacher({ school_id: activeSchoolID, name: String(it.name), short_name: String(it.short_name || ""), max_hours_per_week: Number(it.max_hours_per_week) || 30 });
                                } else if (entity === "subjects") {
                                        if (!it.name) continue;
                                        await CreateSubject({ school_id: activeSchoolID, name: String(it.name), short_name: String(it.short_name || ""), requires_room_type: String(it.requires_room_type || "any") });
                                } else if (entity === "rooms") {
                                        if (!it.name) continue;
                                        await CreateRoom({ school_id: activeSchoolID, name: String(it.name), room_type: String(it.room_type || "any") });
                                } else if (entity === "classes") {
                                        if (!it.name) continue;
                                        let room_id = 0;
                                        if (it.room) { const r = rooms.find((x) => x.name === it.room); if (r) room_id = r.id; }
                                        let subgroup_of = null;
                                        if (it.subgroup_of) { const p = classes.find((x) => x.name === it.subgroup_of); if (p) subgroup_of = p.id; }
                                        const created = await CreateClass({ school_id: activeSchoolID, name: String(it.name), room_id, subgroup_of });
                                        classes = [...classes, created]; // родитель должен быть виден следующим строкам файла
                                } else if (entity === "lessons") {
                                        const c = classes.find((x) => x.name === it.class);
                                        const s = subjects.find((x) => x.name === it.subject);
                                        const t = teachers.find((x) => x.name === it.teacher || (x.short_name && x.short_name === it.teacher));
                                        if (!c || !s || !t) throw new Error("не найдено в справочниках: " + JSON.stringify(it));
                                        await CreateLesson({ school_id: activeSchoolID, class_id: c.id, subject_id: s.id, teacher_id: t.id, hours_per_week: Number(it.hours_per_week) || 1 });
                                } else if (entity === "periods") {
                                        // массив звонков заменяет расписание звонков целиком
                                        const ps = items.map((p) => ({ start: String(p.start || ""), end: String(p.end || "") }));
                                        await UpdateSchoolSettings(activeSchoolID, JSON.stringify({ days, slots: ps.length, days_mask: schoolDaysMask, periods: ps }));
                                        await loadSettings();
                                        n = ps.length;
                                        break;
                                }
                                n++;
                        }
                        await reloadRefs();
                        flash("Импортировано: " + n);
                } catch (err) {
                        flash("Ошибка импорта: " + (err && err.message ? err.message : err));
                } finally {
                        e.target.value = "";
                }
        }

        // Резолверы имён — O(1) через map (list остаётся в сигнатуре как
        // синтаксическая зависимость реактивности, сам не используется).
        function className(list, id) { const x = classById.get(id); return x ? x.name : "?"; }
        function roomName(list, id) { const x = roomById.get(id); return x ? x.name : "?"; }
        function subjName(list, id) { const x = subjectById.get(id); return x ? x.name : "?"; }
        function teachName(list, id) { const x = teacherById.get(id); return x ? (x.short_name || x.name) : "?"; }
        function dayName(d) { return ["Пн","Вт","Ср","Чт","Пт","Сб","Вс"][d] || ("Д" + (d + 1)); }
        function sI(i) { return i + 1; }
        function periodLabel(si) { const p = bellPeriods[si]; return p && p.start ? p.start + "–" + p.end : ""; }
        function slotLabel(si) {
                const lbl = periodLabel(si);
                return "П" + (si + 1) + (lbl ? " " + lbl : "");
        }

        // -- Constraint UI helpers --
        const CONSTRAINT_FIELDS = {
                teacher_unavailable:    { day: true,  slots: true,  value: false, valueLabel: "" },
                class_unavailable:      { day: true,  slots: true,  value: false, valueLabel: "" },
                room_unavailable:       { day: true,  slots: true,  value: false, valueLabel: "" },
                max_consecutive:        { day: false, slots: false, value: true,  valueLabel: "Макс. подряд" },
                max_consecutive_subject:{ day: false, slots: false, value: true,  valueLabel: "не более подряд" },
                lunch_break:            { day: false, slots: true,  value: false, valueLabel: "" },
                max_lessons_per_day:    { day: false, slots: false, value: true,  valueLabel: "Макс. в день" },
                min_lessons_per_day:    { day: false, slots: false, value: true,  valueLabel: "Мин. в день" },
                prefer_morning:         { day: false, slots: true,  value: false, valueLabel: "" },
                max_gaps:               { day: false, slots: false, value: true,  valueLabel: "Макс. окон" },
        };
        const CONSTRAINT_LABELS = {
                teacher_unavailable:    "Учитель недоступен",
                class_unavailable:      "Класс недоступен",
                room_unavailable:       "Кабинет недоступен",
                max_consecutive:        "Макс. подряд уроков",
                max_consecutive_subject:"Один предмет — не более N подряд",
                lunch_break:            "Обеденный перерыв",
                max_lessons_per_day:    "Макс. уроков в день",
                min_lessons_per_day:    "Мин. уроков в день",
                prefer_morning:         "Желательно утро",
                max_gaps:               "Макс. окон",
        };
        function constraintTypeLabel(t) { return CONSTRAINT_LABELS[t] || t; }
        function constraintFields(t) { return CONSTRAINT_FIELDS[t] || { day: false, slots: false, value: false, valueLabel: "" }; }
        function constraintEntityLabel(c) {
                if (c.entity_type === "school") return "вся школа";
                if (c.entity_type === "subject") return subjName(subjects, c.entity_id);
                const list = c.entity_type === "teacher" ? teachers : c.entity_type === "class" ? classes : rooms;
                const item = list.find(x => x.id === c.entity_id);
                return item ? item.name : "#" + c.entity_id;
        }
        function constraintDetail(c) {
                const f = constraintFields(c.type);
                const parts = [];
                if (f.day && c.day_of_week != null) parts.push(dayName(c.day_of_week));
                if (f.slots) {
                        if (c.timeslot_start != null && c.timeslot_end != null) {
                                parts.push("П" + (c.timeslot_start + 1) + "–П" + (c.timeslot_end + 1));
                        } else if (c.timeslot_start != null) {
                                parts.push("с П" + (c.timeslot_start + 1));
                        } else if (c.timeslot_end != null) {
                                parts.push("по П" + (c.timeslot_end + 1));
                        }
                }
                if (f.value) parts.push(String(c.weight));
                return parts.join(", ");
        }
        function constraintSummary(c) {
                const detail = constraintDetail(c);
                return constraintTypeLabel(c.type) + " · " + constraintEntityLabel(c) + (detail ? " (" + detail + ")" : "") + " · " + (c.is_hard ? "🔒 жёсткое" : "📝 мягкое");
        }

        // -- DnD helper for compact labels in flash --
        function cellLabelShort(e) {
                if (!e) return "?";
                return subjName(subjects, e.subject_id) + " (" + teachName(teachers, e.teacher_id) + ")";
        }

        loadSchools();
</script>

<div class="app">
        <aside class="sidebar">
                <div class="brand">📅 <span>Timetable</span></div>
                <nav class="nav">
                        <button class:active={tab === "refs"} on:click={() => tab = "refs"}><span class="ico">📚</span>Справочники</button>
                        <button class:active={tab === "lessons"} on:click={() => tab = "lessons"}><span class="ico">📝</span>Учебный план</button>
                        <button class:active={tab === "constraints"} on:click={() => tab = "constraints"}><span class="ico">⚠️</span>Ограничения</button>
                        <button class:active={tab === "settings"} on:click={() => tab = "settings"}><span class="ico">⚙️</span>Настройки</button>
                        <button class:active={tab === "schedule"} on:click={() => tab = "schedule"}><span class="ico">🗓️</span>Расписание</button>
                </nav>
        </aside>

        <div class="content">
                <header class="topbar">
                        <div class="school">
                                <select bind:value={activeSchoolID} on:change={async () => { await reloadRefs(); await loadSettings(); }}>
                                        {#each schools as sc}<option value={sc.id}>{sc.name}</option>{/each}
                                </select>
                                {#if schools.length === 0}<span class="muted">нет школ</span>{/if}
                                <input class="school-new" bind:value={newSchoolName} placeholder="Новая школа" />
                                <button class="primary sm" on:click={createSchool}>+ Школа</button>
                                {#if activeSchoolID}
                                <button class="danger sm" on:click={deleteSchool} title="Удалить текущую школу со всеми данными">🗑</button>
                                {/if}
                        </div>
                        {#if msg}<div class="toast">{msg}</div>{/if}
                        <span class="ver">v{APP_VERSION}</span>
                </header>

                <main>
                        {#if tab === "refs"}
                                <div class="cards">
                                        <section class="card">
                                                <div class="card-head">
                                                        <h2>Учителя</h2>
                                                        <div class="csvbar">
                                                                <button class="mini" on:click={() => downloadRefsJSON('teachers')}>⬇ JSON</button>
                                                                <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('teachers', e)} /></label>
                                                        </div>
                                                </div>
                                                <div class="row">
                                                        <input bind:value={t.name} placeholder="Имя" />
                                                        <input bind:value={t.short_name} placeholder="Кратко" />
                                                        <input type="number" bind:value={t.max_hours_per_week} title="Часов/нед" />
                                                        <button class="primary" on:click={addTeacher}>+</button>
                                                </div>
                                                <ul class="list">{#each teachers as x}<li>
                                                        {#if editing && editing.kind === "teacher" && editing.id === x.id}
                                                                <input class="edit" bind:value={x.name} placeholder="Имя" />
                                                                <input class="edit w-s" bind:value={x.short_name} placeholder="Кратко" />
                                                                <input class="edit w-xs" type="number" bind:value={x.max_hours_per_week} title="Часов/нед" />
                                                                <button class="primary sm" on:click={() => saveEdit("teacher", x)} title="Сохранить">✓</button>
                                                                <button class="sm" on:click={cancelEdit} title="Отмена">✗</button>
                                                        {:else}
                                                                <span class="li-text">{x.name} <small>({x.short_name})</small> <span class="muted">— {x.max_hours_per_week}ч/нед</span></span>
                                                                <button class="ghost-ico sm" on:click={() => startEdit("teacher", x.id)} title="Редактировать">✎</button>
                                                                <button class="danger sm" on:click={() => removeTeacher(x.id)}>✕</button>
                                                        {/if}
                                                </li>{/each}</ul>
                                        </section>

                                        <section class="card">
                                                <div class="card-head">
                                                        <h2>Предметы</h2>
                                                        <div class="csvbar">
                                                                <button class="mini" on:click={() => downloadRefsJSON('subjects')}>⬇ JSON</button>
                                                                <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('subjects', e)} /></label>
                                                        </div>
                                                </div>
                                                <div class="row">
                                                        <input bind:value={s.name} placeholder="Название" />
                                                        <input bind:value={s.short_name} placeholder="Кратко" />
                                                        <button class="primary" on:click={addSubject}>+</button>
                                                </div>
                                                <ul class="list">{#each subjects as x}<li>
                                                        {#if editing && editing.kind === "subject" && editing.id === x.id}
                                                                <input class="edit" bind:value={x.name} placeholder="Название" />
                                                                <input class="edit w-s" bind:value={x.short_name} placeholder="Кратко" />
                                                                <button class="primary sm" on:click={() => saveEdit("subject", x)} title="Сохранить">✓</button>
                                                                <button class="sm" on:click={cancelEdit} title="Отмена">✗</button>
                                                        {:else}
                                                                <span class="li-text">{x.name} <small>({x.short_name})</small></span>
                                                                <button class="ghost-ico sm" on:click={() => startEdit("subject", x.id)} title="Редактировать">✎</button>
                                                                <button class="danger sm" on:click={() => removeSubject(x.id)}>✕</button>
                                                        {/if}
                                                </li>{/each}</ul>
                                        </section>

                                        <section class="card">
                                                <div class="card-head">
                                                        <h2>Классы</h2>
                                                        <div class="csvbar">
                                                                <button class="mini" on:click={() => downloadRefsJSON('classes')}>⬇ JSON</button>
                                                                <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('classes', e)} /></label>
                                                        </div>
                                                </div>
                                                <div class="row">
                                                        <input bind:value={c.name} placeholder="10А" />
                                                        <select bind:value={c.room_id} title="Домашний кабинет класса"><option value={0}>— без кабинета —</option>{#each rooms as rm}<option value={rm.id}>{rm.name}</option>{/each}</select>
                                                        <select bind:value={c.subgroup_of}><option value={null}>— целый класс —</option>{#each classes as x}<option value={x.id}>{x.name} (подгруппа)</option>{/each}</select>
                                                        <button class="primary" on:click={addClass}>+</button>
                                                </div>
                                                <ul class="list">{#each classes as x}<li>
                                                        {#if editing && editing.kind === "class" && editing.id === x.id}
                                                                <input class="edit" bind:value={x.name} placeholder="10А" />
                                                                <select class="edit" bind:value={x.room_id} title="Домашний кабинет"><option value={0}>— без кабинета —</option>{#each rooms as rm}<option value={rm.id}>{rm.name}</option>{/each}</select>
                                                                <button class="primary sm" on:click={() => saveEdit("class", x)} title="Сохранить">✓</button>
                                                                <button class="sm" on:click={cancelEdit} title="Отмена">✗</button>
                                                        {:else}
                                                                <span class="li-text">{x.name}{#if x.room_id}<span class="muted"> — каб. {rooms.find((rm) => rm.id === x.room_id)?.name || ""}</span>{/if}{x.subgroup_of ? " · подгруппа" : ""}</span>
                                                                <button class="ghost-ico sm" on:click={() => startEdit("class", x.id)} title="Редактировать">✎</button>
                                                                <button class="danger sm" on:click={() => removeClass(x.id)}>✕</button>
                                                        {/if}
                                                </li>{/each}</ul>
                                        </section>

                                        <section class="card">
                                                <div class="card-head">
                                                        <h2>Кабинеты</h2>
                                                        <div class="csvbar">
                                                                <button class="mini" on:click={() => downloadRefsJSON('rooms')}>⬇ JSON</button>
                                                                <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('rooms', e)} /></label>
                                                        </div>
                                                </div>
                                                <div class="row">
                                                        <input bind:value={r.name} placeholder="301" />
                                                        <button class="primary" on:click={addRoom}>+</button>
                                                </div>
                                                <ul class="list">{#each rooms as x}<li>
                                                        {#if editing && editing.kind === "room" && editing.id === x.id}
                                                                <input class="edit" bind:value={x.name} placeholder="301" />
                                                                <button class="primary sm" on:click={() => saveEdit("room", x)} title="Сохранить">✓</button>
                                                                <button class="sm" on:click={cancelEdit} title="Отмена">✗</button>
                                                        {:else}
                                                                <span class="li-text">{x.name}</span>
                                                                <button class="ghost-ico sm" on:click={() => startEdit("room", x.id)} title="Редактировать">✎</button>
                                                                <button class="danger sm" on:click={() => removeRoom(x.id)}>✕</button>
                                                        {/if}
                                                </li>{/each}</ul>
                                        </section>
                                </div>
                        {:else if tab === "lessons"}
                                <section class="card">
                                        <div class="card-head">
                                                <h2>Учебный план (уроки)</h2>
                                                <div class="csvbar">
                                                        <button class="mini" on:click={() => downloadRefsJSON('lessons')}>⬇ JSON</button>
                                                        <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('lessons', e)} /></label>
                                                </div>
                                        </div>
                                        <div class="lesson-form">
                                                <select bind:value={curClass}><option value={0}>Выберите класс…</option>{#each classes as x}<option value={x.id}>{x.name}</option>{/each}</select>
                                        </div>
                                        {#if curClass}
                                                {@const cl = lessons.filter((x) => x.class_id === curClass)}
                                                <table class="data">
                                                        <thead><tr><th>Предмет</th><th>Учитель</th><th>Ч/нед</th><th></th></tr></thead>
                                                        <tbody>
                                                                {#each cl as x}
                                                                        <tr>
                                                                                <td>{subjName(subjects, x.subject_id)}</td>
                                                                                <td><select bind:value={x.teacher_id} on:change={() => updateLesson(x)}>{#each teachers as t}<option value={t.id}>{t.name}</option>{/each}</select></td>
                                                                                <td><input type="number" min="1" max="40" bind:value={x.hours_per_week} on:change={() => updateLesson(x)} /></td>
                                                                                <td class="act"><button class="danger sm" on:click={() => removeLesson(x.id)}>✕</button></td>
                                                                        </tr>
                                                                {/each}
                                                                <tr class="addrow">
                                                                        <td><select bind:value={l.subject_id}><option value={0}>Предмет</option>{#each subjects as s}<option value={s.id}>{s.name}</option>{/each}</select></td>
                                                                        <td><select bind:value={l.teacher_id}><option value={0}>Учитель</option>{#each teachers as t}<option value={t.id}>{t.name}</option>{/each}</select></td>
                                                                        <td><input type="number" min="1" max="40" bind:value={l.hours_per_week} placeholder="Ч/нед" /></td>
                                                                        <td class="act"><button class="primary sm" on:click={addLessonForClass}>+ Добавить</button></td>
                                                                </tr>
                                                        </tbody>
                                                </table>
                                                {#if cl.length === 0}<p class="muted">У этого класса пока нет уроков. Добавьте предмет и учителя выше.</p>{/if}
                                        {:else}
                                                <p class="muted">Выберите класс, чтобы увидеть и редактировать его учебный план (предмет + учитель + часы в неделю).</p>
                                        {/if}
                                </section>
                        {:else if tab === "constraints"}
                                <section class="card">
                                        <div class="card-head"><h2>Ограничения</h2></div>
                                        <div class="school-days">
                                                <span class="field-label">Учебные дни недели:</span>
                                                {#each DAY_NAMES as dn, d}
                                                        <label class="chk" title={activeDayIdx.includes(d) ? "Учебный день — снять галочку, чтобы убрать день из расписания" : "Выходной — включить день в расписание"}>
                                                                <input type="checkbox"
                                                                        checked={(schoolDaysMask & (1 << d)) !== 0}
                                                                        on:change={(e) => toggleSchoolDay(d, e.currentTarget.checked)} /> {dn}
                                                        </label>
                                                {/each}
                                        </div>
                                        <p class="hint">Снятые дни сразу исчезают из сетки расписания, генерации и PDF — как выходные. Последний оставшийся день снять нельзя.</p>

                                        <div class="pick-bar">
                                                <div class="pick-kind">
                                                        <button class:active={consPickKind === "teacher"} on:click={() => setPickKind("teacher")}>Учитель</button>
                                                        <button class:active={consPickKind === "class"} on:click={() => setPickKind("class")}>Класс</button>
                                                        <button class:active={consPickKind === "room"} on:click={() => setPickKind("room")}>Кабинет</button>
                                                </div>
                                                <select value={consPickEntity ? consPickEntity.id : 0} on:change={(e) => consPickId = +e.currentTarget.value}>
                                                        {#each consPickList as x (x.id)}<option value={x.id}>{pickName(x)}</option>{/each}
                                                </select>
                                                <span class="hint pick-hint">Клик по ячейке — запретить или разрешить это время.</span>
                                        </div>
                                        <table class="pick-grid">
                                                <thead><tr><th class="d"></th>{#each Array(slots) as _, si}<th>{#if periodLabel(si)}{periodLabel(si)}{:else}П{si + 1}{/if}</th>{/each}</tr></thead>
                                                <tbody>
                                                        {#each activeDayIdx as d (d)}
                                                                <tr>
                                                                        <td class="day">{dayName(d)}</td>
                                                                        {#each Array(slots) as _, si}
                                                                                <td class="slot" class:forbidden={isForbidden(d, si)}
                                                                                        on:click={() => toggleForbidden(d, si)}
                                                                                        title={isForbidden(d, si) ? "Клик — разрешить" : "Клик — запретить"}>{#if isForbidden(d, si)}✕{/if}</td>
                                                                        {/each}
                                                                </tr>
                                                        {/each}
                                                </tbody>
                                        </table>


                                        <ul class="list constraints-list">
                                                {#each constraints as x}
                                                        <li>
                                                                <span class="con-summary">{constraintSummary(x)}</span>
                                                                <button class="danger sm" on:click={() => removeConstraint(x.id)}>✕</button>
                                                        </li>
                                                {/each}
                                        </ul>

                                        <div class="rule-form">
                                                <span class="field-label">Правило:</span>
                                                <select bind:value={ruleSubject}>
                                                        <option value={0}>Все предметы</option>
                                                        {#each subjects as s}<option value={s.id}>{s.name}</option>{/each}
                                                </select>
                                                <label class="rule-max">не более <input type="number" min="1" max="8" bind:value={ruleMax} /> подряд в день</label>
                                                <button class="primary sm" on:click={addRunRule}>+ Правило</button>
                                        </div>
                                        <p class="hint">Как в aSc Timetables: ученики не получают один и тот же предмет N уроков подряд (например, информатика 1-й, 2-й и 3-й уроки одной строкой). Половинки делёного урока в одном слоте считаются одним периодом.</p>
                                </section>
                        {:else if tab === "settings"}
                                <section class="card">
                                        <div class="card-head"><h2>Настройки школы</h2></div>
                                        <div class="row">
                                                <label>Учебных дней: <input type="number" min="1" max="7" bind:value={days} /></label>
                                                <label>Уроков в день: <input type="number" min="1" max="14" bind:value={slots} on:change={onSlotsChange} /></label>
                                                <button class="primary" on:click={saveSettings}>Сохранить</button>
                                                <span class="csvbar">
                                                        <button class="mini" on:click={() => downloadRefsJSON('periods')}>⬇ JSON звонков</button>
                                                        <label class="mini file">⬆<input type="file" accept=".json" on:change={(e) => importRefsJSON('periods', e)} /></label>
                                                </span>
                                        </div>
                                        <h3>Расписание звонков</h3>
                                        <table class="data">
                                                <thead><tr><th>Урок</th><th>Начало</th><th>Конец</th></tr></thead>
                                                <tbody>
                                                        {#each Array(slots) as _, si}
                                                                <tr>
                                                                        <td>П{si + 1}</td>
                                                                        <td><input class="bell-time" type="text" inputmode="numeric" placeholder="08:00" bind:value={bellPeriods[si].start} on:change={() => { bellPeriods[si].start = normalizeTime(bellPeriods[si].start); }} title="24-часовой формат ЧЧ:ММ" /></td>
                                                                        <td><input class="bell-time" type="text" inputmode="numeric" placeholder="08:45" bind:value={bellPeriods[si].end} on:change={() => { bellPeriods[si].end = normalizeTime(bellPeriods[si].end); }} title="24-часовой формат ЧЧ:ММ" /></td>
                                                                </tr>
                                                        {/each}
                                                </tbody>
                                        </table>
                                        <p class="hint">Время отображается в шапке расписания и в PDF. Хранится в базе (SQLite).</p>
                                </section>
                        {:else if tab === "schedule"}
                                <section class="card">
                                        <div class="gen-bar">
                                                <span class="grid-info">{days} дней · {slots} уроков в день</span>
                                                <button class="primary" on:click={generate}>⚙ Сгенерировать</button>
                                                <button on:click={undo}>↶ Отменить</button>
                                                <label class="chk" class:warn={!hasPreciseSolver} title={hasPreciseSolver ? "OR-Tools CP-SAT доступен в этой сборке" : "OR-Tools CP-SAT НЕ скомпилирован в эту сборку — будет использован быстрый эвристический решатель"}><input type="checkbox" bind:checked={usePrecise} /> точный CP-SAT (OR-Tools){#if !hasPreciseSolver} <span class="badge-warn">недоступно</span>{/if}</label>
                                                <span class="sep"></span>
                                                <label>Вид:
                                                        <select bind:value={viewMode}>
                                                                <option value="school">вся школа (обзор)</option>
                                                                <option value="class">по классам</option>
                                                                <option value="teacher">по учителям</option>
                                                                <option value="room">по кабинетам</option>
                                                        </select>
                                                </label>
                                                {#if !overviewMode}
                                                <label class="chk"><input type="checkbox" bind:checked={compact} /> компактный</label>
                                                {/if}
                                                {#if !overviewMode && kind === "class" && rows.length > classesPerPage}
                                                        <span class="pager">
                                                                <button class="sm" on:click={() => classPage = Math.max(0, classPage - 1)} disabled={classPage === 0}>‹</button>
                                                                <span>{classPage + 1}/{totalClassPages}</span>
                                                                <button class="sm" on:click={() => classPage = Math.min(totalClassPages - 1, classPage + 1)} disabled={classPage >= totalClassPages - 1}>›</button>
                                                        </span>
                                                {/if}
                                                <span class="sep"></span>
                                                <div class="export-panel">
                                                <span class="panel-label">ЭКСПОРТ</span>
                                                <label>Стр.:
                                                        <select bind:value={pageSize}>
                                                                <option value="A0">A0</option><option value="A1">A1</option><option value="A2">A2</option><option value="A3">A3</option><option value="A4">A4</option>
                                                        </select>
                                                </label>
                                                <label>Ориент.:
                                                        <select bind:value={orientation}>
                                                                <option value="landscape">альбомн.</option>
                                                                <option value="portrait">книжн.</option>
                                                        </select>
                                                </label>
                                                <label class="chk"><input type="checkbox" bind:checked={pdfShowTeacher} /> учителя</label>
                                                <label class="chk"><input type="checkbox" bind:checked={pdfShowRoom} /> кабинеты</label>
                                                <label class="chk"><input type="checkbox" bind:checked={pdfWeekdaysOnly} /> будни</label>
                                                <label class="chk"><input type="checkbox" bind:checked={pdfBW} /> ч/б</label>
                                                <button on:click={exportPDF}>⬇ PDF</button>
                                                <button on:click={exportCSV}>⬇ CSV</button>
                                                <button on:click={exportJSON}>⬇ JSON</button>
                                                <label class="file">⬆ JSON<input type="file" accept="application/json" on:change={importJSON} /></label>
                                                </div>
                                        </div>
                                        {#if genResult}<p class="status">Размещено <b>{genResult.placed}/{genResult.total}</b> · мягких нарушений: <b>{genResult.violations}</b></p>{/if}
                                        <p class="hint">Чтобы переместить урок: зажмите и перетащите ячейку в другую (drag) либо кликните урок, затем кликните целевую ячейку. Во время перетаскивания целевая ячейка подсвечивается. Повторный клик по выделенному (или Esc) снимает выделение. ✕ в ячейке — удалить.</p>
                                        {#if schedule.length === 0}
                                                <p class="empty">Расписание пусто. Добавьте уроки и нажмите «Сгенерировать».</p>
                                        {:else}
                                                {#if overviewMode}
                                                        <div class="overview">
                                                                {#each grid as row (row.id)}
                                                                        <div class="mini" class:is-sub={isSubRow(row.id)}>
                                                                                <button class="mini-title" on:click={() => focusRow(row.id)} title="Открыть этот класс отдельно">{row.label}{#if subParentName(row.id)}<span class="sub-badge">подгруппа {subParentName(row.id)}</span>{/if}</button>
                                                                                <table>
                                                                                        <thead><tr><th class="d"></th>{#each Array(slots) as _, si}<th>{#if periodLabel(si)}{periodLabel(si)}{:else}П{si + 1}{/if}</th>{/each}</tr></thead>
                                                                                        <tbody>
                                                                                                {#each row.cells as dayCells, di}
                                                                                                        <tr><td class="day">{dayName(activeDayIdx[di])}</td>{#each dayCells as cell, si}<td
                                                                                                                class:filled={!!cell}
                                                                                                                class:selected={selectedEntry && cell && selectedEntry.id === cell.id}
                                                                                                                class:drop-target={dropTarget && dropTarget.rowId === row.id && dropTarget.day === activeDayIdx[di] && dropTarget.slot === si}
                                                                                                                title={cell ? cell.label : ''}
                                                                                                                data-cell
                                                                                                                data-day={activeDayIdx[di]}
                                                                                                                data-slot={si}
                                                                                                                data-row={row.id}
                                                                                                                data-kind={kind}
                                                                                                                on:pointerdown={(e) => onPointerDown(e, cell, kind, row.id, activeDayIdx[di], si)}>{#if cell}<div class="chip" class:half={cellSubs("class", row.id, activeDayIdx[di], si).length > 0} class:conflict={cell.conflict} style="background:{cell.conflict ? '#dc2626' : subjectColor(cell.subject_id)}">{subjShort(subjects, cell.subject_id)}</div>{/if}{#each cellSubs("class", row.id, activeDayIdx[di], si) as sc}<div class="chip half" style="background:{subjectColor(sc.subject_id)}" title="Подгруппа {classes.find((c) => c.id === sc.class_id)?.name || ''}">{subjShort(subjects, sc.subject_id)}</div>{/each}</td>{/each}</tr>
                                                                                                {/each}
                                                                                        </tbody>
                                                                                </table>
                                                                        </div>
                                                                {/each}
                                                        </div>
                                                {:else}
                                                <div class="grid-scroll">
                                                        {#each grid as row (row.id)}
                                                                <div class="class-block" class:compact class:is-sub={isSubRow(row.id)}>
                                                                        <h3>{row.label} {#if subParentName(row.id)}<span class="sub-badge">подгруппа {subParentName(row.id)}</span>{/if}</h3>
                                                                        <table class:compact>
                                                                                <thead><tr><th class="day-h">День</th>{#each Array(slots) as _, si}<th>П{si + 1}{#if periodLabel(si)}<span class="tm">{periodLabel(si)}</span>{/if}</th>{/each}</tr></thead>
                                                                                <tbody>
                                                                                        {#each row.cells as dayCells, di}
                                                                                                <tr><td class="day">{dayName(activeDayIdx[di])}</td>{#each dayCells as cell, si}<td
                                                                                                        class:filled={!!cell}
                                                                                                        class:selected={selectedEntry && cell && selectedEntry.id === cell.id}
                                                                                                        class:drop-target={dropTarget && dropTarget.rowId === row.id && dropTarget.day === activeDayIdx[di] && dropTarget.slot === si}
                                                                                                        title={cell ? cell.label : ''}
                                                                                                        data-cell
                                                                                                        data-day={activeDayIdx[di]}
                                                                                                        data-slot={si}
                                                                                                        data-row={row.id}
                                                                                                        data-kind={kind}
                                                                                                        on:pointerdown={(e) => onPointerDown(e, cell, kind, row.id, activeDayIdx[di], si)}>{#if cell}<div class="chip" class:half={cellSubs(kind, row.id, activeDayIdx[di], si).length > 0} class:conflict={cell.conflict} style="background:{cell.conflict ? '#dc2626' : subjectColor(cell.subject_id)}"><b>{subjName(subjects, cell.subject_id)}</b>{#if pdfShowTeacher}<span>{teachName(teachers, cell.teacher_id)}</span>{/if}{#if pdfShowRoom && cell.room_id}<span>{roomById.get(cell.room_id)?.name || ""}</span>{/if}</div><button class="cell-x" title="Удалить" on:click={(e) => { e.stopPropagation(); removeEntry(cell.id); }}>✕</button>{/if}{#each cellSubs(kind, row.id, activeDayIdx[di], si) as sc}<div class="chip half" role="button" tabindex="-1" class:conflict={conflictIDs.has(sc.id)} style="background:{conflictIDs.has(sc.id) ? '#dc2626' : subjectColor(sc.subject_id)}" title="Подгруппа {classById.get(sc.class_id)?.name || ''}" on:pointerdown={(e) => { e.stopPropagation(); onPointerDown(e, cellAt("class", sc.class_id, activeDayIdx[di], si), "class", row.id, activeDayIdx[di], si); }}><b>{subjName(subjects, sc.subject_id)}</b>{#if pdfShowTeacher}<span>{teachName(teachers, sc.teacher_id)}</span>{/if}</div>{/each}</td>{/each}</tr>
                                                                                        {/each}
                                                                                </tbody>
                                                                        </table>
                                                                </div>
                                                        {/each}
                                                </div>
                                                {/if}
                                                {#if ghost}<div class="drag-ghost" style="left:{ghost.x}px; top:{ghost.y}px;">{ghost.label}</div>{/if}
                                                {#if report.conflicts.length || report.overloads.length || report.violations.length}
                                                        <div class="report">
                                                                <h3>Отчёт по конфликтам</h3>
                                                                {#if report.conflicts.length}
                                                                        <div class="rep-sec"><b>Накладки ({report.conflicts.length}):</b>
                                                                                <ul>{#each report.conflicts as c}<li>{dayName(c.day)} П{c.slot + 1}: {c.type} — {#each c.items as it, i}{it.subject} ({it.who}){#if i < c.items.length - 1}, {/if}{/each}</li>{/each}</ul>
                                                                        </div>
                                                                {/if}
                                                                {#if report.violations.length}
                                                                        <div class="rep-sec"><b>Нарушено жёсткое ограничение ({report.violations.length}):</b>
                                                                                <ul>{#each report.violations as v}<li>{dayName(v.day)} П{v.slot + 1}: {v.what} — урок стоит в запрещённой ячейке</li>{/each}</ul>
                                                                        </div>
                                                                {/if}
                                                                {#if report.overloads.length}
                                                                        <div class="rep-sec"><b>Перегрузки учителей ({report.overloads.length}):</b>
                                                                                <ul>{#each report.overloads as o}<li>{o.name} — {o.got}ч при лимите {o.max}ч</li>{/each}</ul>
                                                                        </div>
                                                                {/if}
                                                        </div>
                                                {/if}
                                        {/if}
                                        {#if lessons.length || schedule.length}
                                                <div class="panel" class:ok={report.unplaced.length === 0}>
                                                        <div class="panel-head">
                                                                <h3>Панель расстановки</h3>
                                                                <span class="panel-pct" class:bad={report.unplaced.length > 0}>{report.placedPct}%</span>
                                                        </div>
                                                        <div class="bar"><div class="bar-fill" class:bad={report.unplaced.length > 0} style="width:{report.placedPct}%"></div></div>
                                                        <div class="panel-stats">
                                                                <span>Размещено <b>{report.placedHours}</b> из <b>{report.totalHours}</b> ч</span>
                                                                {#if report.missingHours > 0}<span class="warn">осталось {report.missingHours} ч</span>{:else}<span class="good">все уроки размещены</span>{/if}
                                                                <span class="muted">уроков в плане: {lessons.length}</span>
                                                        </div>
                                                        {#if report.reasonGroups.length}
                                                                <div class="panel-reasons">
                                                                        <b>Почему не расставлено ({report.unplaced.length} уроков):</b>
                                                                        {#each report.reasonGroups as g}
                                                                                <div class="reason-row">
                                                                                        <span class="reason-badge">{g.items.length}</span>
                                                                                        <span class="reason-text">{g.reason}</span>
                                                                                        <span class="reason-who">{g.items.map((i) => i.subject + " · " + i.cls).join(", ")}</span>
                                                                                </div>
                                                                        {/each}
                                                                </div>
                                                        {/if}
                                                        {#if report.classStats.length}
                                                                <details class="panel-classes">
                                                                        <summary>По классам: размещено / нужно недельных часов</summary>
                                                                        <div class="cls-grid">
                                                                                {#each report.classStats as cs}
                                                                                        <div class="cls-cell" class:bad={cs.missing > 0} title={cs.missing > 0 ? `не хватает ${cs.missing}ч` : "полностью размещён"}>
                                                                                                <span class="cls-name">{cs.name}</span>
                                                                                                <span class="cls-val">{cs.placed}/{cs.need}</span>
                                                                                        </div>
                                                                                {/each}
                                                                        </div>
                                                                </details>
                                                        {/if}
                                                </div>
                                        {/if}
                                </section>
                        {/if}
                </main>

                {#if saveModal}
                        <div class="modal-backdrop" role="button" tabindex="-1" on:click={cancelSave} on:keydown={(e) => { if (e.key === 'Escape') cancelSave(); }}>
                                <div class="modal" role="dialog" tabindex="0" on:click|stopPropagation on:keydown|stopPropagation>
                                        <h3>Сохранить файл</h3>
                                        <p class="muted">Файл будет записан в папку ~/Downloads</p>
                                        <input class="modal-input" bind:value={saveModal.filename} placeholder="имя файла" on:keydown={(e) => { if (e.key === "Enter") confirmSave(); }} />
                                        <div class="modal-actions">
                                                <button on:click={cancelSave}>Отмена</button>
                                                <button class="primary" on:click={confirmSave}>Сохранить</button>
                                        </div>
                                </div>
                        </div>
                {/if}

                {#if confirmBox}
                        <div class="modal-backdrop" role="button" tabindex="-1" on:click={() => settleConfirm(false)} on:keydown={(e) => { if (e.key === 'Escape') settleConfirm(false); }}>
                                <div class="modal" role="dialog" tabindex="0" on:click|stopPropagation on:keydown|stopPropagation>
                                        <h3>Подтверждение</h3>
                                        <p class="modal-msg">{confirmBox.message}</p>
                                        <div class="modal-actions">
                                                <button on:click={() => settleConfirm(false)}>Отмена</button>
                                                <button class="danger" on:click={() => settleConfirm(true)}>{confirmBox.confirmLabel}</button>
                                        </div>
                                </div>
                        </div>
                {/if}


        </div>
</div>

<style>
        :global(body) { margin: 0; }
        .app {
                display: flex;
                min-height: 100vh;
                font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, Arial, sans-serif;
                color: #1e293b;
                background: #f1f5f9;
        }
        .sidebar {
                width: 232px;
                flex: 0 0 232px;
                background: #ffffff;
                border-right: 1px solid #e2e8f0;
                display: flex;
                flex-direction: column;
                padding: 18px 14px;
                position: sticky;
                top: 0;
                height: 100vh;
                box-sizing: border-box;
        }
        .brand { font-size: 20px; font-weight: 700; color: #4f46e5; padding: 4px 8px 18px; }
        .brand span { color: #0f172a; }
        .nav { display: flex; flex-direction: column; gap: 4px; }
        .nav button {
                display: flex; align-items: center; gap: 10px;
                text-align: left;
                background: transparent; border: none; border-radius: 10px;
                padding: 10px 12px; color: #475569; font-size: 14px; cursor: pointer;
        }
        .nav button:hover { background: #f1f5f9; color: #0f172a; }
        .nav button.active { background: #eef2ff; color: #4f46e5; font-weight: 600; }
        .nav .ico { font-size: 16px; width: 20px; text-align: center; }

        .content { flex: 1; display: flex; flex-direction: column; min-width: 0; }
        .topbar {
                display: flex; align-items: center; gap: 14px;
                padding: 14px 24px; background: #ffffff; border-bottom: 1px solid #e2e8f0;
                position: sticky; top: 0; z-index: 5;
        }
        .school { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
        .school select { min-width: 160px; }
        .school-new { width: 150px; }
        main { padding: 24px; }

        input, select {
                background: #fff; color: #1e293b; border: 1px solid #cbd5e1;
                border-radius: 8px; padding: 7px 10px; font-size: 13px; outline: none;
        }
        input:focus, select:focus { border-color: #4f46e5; box-shadow: 0 0 0 3px #e0e7ff; }
        button {
                background: #e2e8f0; color: #1e293b; border: none; border-radius: 8px;
                padding: 8px 14px; cursor: pointer; font-size: 13px; font-weight: 500;
        }
        button:hover { background: #cbd5e1; }
        button.primary { background: #4f46e5; color: #fff; }
        button.primary:hover { background: #4338ca; }
        button.sm { padding: 5px 10px; font-size: 12px; }
        button.danger { background: #fee2e2; color: #b91c1c; }
        button.danger:hover { background: #fecaca; }
        button.mini { padding: 4px 8px; font-size: 12px; background: #f1f5f9; }
        button.mini:hover { background: #e2e8f0; }
        .chk { display: inline-flex; align-items: center; gap: 5px; font-size: 13px; color: #475569; }
        .export-panel { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 6px 10px; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 10px; }
        .export-panel .panel-label { font-size: 10px; font-weight: 700; letter-spacing: 0.5px; color: #64748b; text-transform: uppercase; margin-right: 2px; }
        .grid-info { font-size: 13px; font-weight: 600; color: #475569; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 8px; padding: 5px 10px; }
        .list li { flex-wrap: nowrap; }
        .list .li-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .list input.edit { flex: 1; min-width: 0; padding: 3px 6px; font-size: 12px; }
        .list input.edit.w-s { flex: 0 0 64px; }
        .list input.edit.w-xs { flex: 0 0 56px; }
        .ghost-ico { background: transparent; border: none; color: #64748b; cursor: pointer; font-size: 12px; padding: 2px 4px; }
        .ghost-ico:hover { color: #4f46e5; }
        .field-label { font-size: 11px; color: #64748b; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; }
        /* Визуальный редактор запретов: клик по ячейке сетки. */
        .pick-bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-bottom: 10px; }
        .pick-kind { display: flex; border: 1px solid #e2e8f0; border-radius: 8px; overflow: hidden; }
        .pick-kind button { border: none; background: #f8fafc; padding: 6px 14px; cursor: pointer; font-size: 13px; color: #475569; }
        .pick-kind button.active { background: #4f46e5; color: #fff; }
        .pick-hint { margin: 0; }
        .pick-grid { border-collapse: collapse; width: 100%; max-width: 760px; table-layout: fixed; margin-bottom: 14px; }
        .pick-grid th { font-size: 10.5px; color: #475569; font-weight: 600; padding: 2px 1px; }
        .pick-grid th.d { width: 42px; }
        .pick-grid td.day { background: #f1f5f9; font-weight: 700; color: #475569; font-size: 11.5px; text-align: center; padding: 2px; border: 1px solid #e2e8f0; }
        .pick-grid td.slot { height: 30px; border: 1px solid #e2e8f0; text-align: center; cursor: pointer; color: #fff; font-size: 11px; font-weight: 700; background: #fff; user-select: none; }
        .pick-grid td.slot:hover { background: #dbeafe; }
        .pick-grid td.slot.forbidden { background: #dc2626; }
        .pick-grid td.slot.forbidden:hover { background: #b91c1c; }
        .constraints-list li { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
        .con-summary { flex: 1; line-height: 1.4; }
        .chk.warn { color: #b45309; }
        .badge-warn { display: inline-block; background: #fde68a; color: #92400e; padding: 1px 6px; border-radius: 6px; font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; }
        .chk input { width: auto; }
        .school-days { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; padding: 8px 12px; margin-bottom: 12px; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 8px; }
        .school-days .chk { font-weight: 600; }
        /* aSc-style multi-select panes: chips in a wrapping, scrollable strip */
        .row { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; align-items: center; }

        .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 18px; }
        .card {
                background: #fff; border: 1px solid #e2e8f0; border-radius: 14px;
                padding: 18px; box-shadow: 0 1px 2px rgba(15,23,42,0.04);
        }
        .card-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
        .card-head h2 { margin: 0; font-size: 16px; color: #0f172a; }
        h3 { font-size: 14px; color: #334155; margin: 16px 0 8px; }

        .csvbar { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; }
        .file { background: #f1f5f9; padding: 4px 8px; border-radius: 8px; cursor: pointer; }
        .file:hover { background: #e2e8f0; }
        .file input { display: none; }

        .list { list-style: none; padding: 0; margin: 0; max-height: 220px; overflow: auto; }
        .list li { padding: 6px 8px; border-bottom: 1px solid #f1f5f9; font-size: 13px; }
        .list li:last-child { border-bottom: none; }
        .muted { color: #64748b; }

        table.data { border-collapse: collapse; width: 100%; margin-top: 8px; }
        table.data th, table.data td { border-bottom: 1px solid #e2e8f0; padding: 8px 10px; text-align: left; font-size: 13px; }
        table.data thead th { background: #f8fafc; color: #64748b; font-weight: 600; font-size: 12px; }
        table.data tbody tr:hover { background: #f8fafc; }
        table.data td.act { width: 1%; white-space: nowrap; }

        .gen-bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
        .gen-bar .sep { flex-basis: 100%; height: 0; }
        .pager { display: inline-flex; align-items: center; gap: 6px; }
        .status { background: #ecfdf5; border: 1px solid #a7f3d0; color: #065f46; padding: 8px 12px; border-radius: 10px; font-size: 13px; }
        .hint { color: #64748b; font-size: 12px; margin: 6px 0 14px; }
        .rule-form { display: flex; align-items: center; gap: 8px; margin-top: 14px; padding-top: 12px; border-top: 1px solid #e2e8f0; flex-wrap: wrap; }
        .rule-form select { min-width: 160px; }
        .rule-max { font-size: 13px; color: #334155; display: inline-flex; align-items: center; gap: 6px; }
        .rule-max input { width: 64px; }
        .bell-time { width: 84px; font-variant-numeric: tabular-nums; }
        .empty { color: #94a3b8; padding: 24px; text-align: center; background: #f8fafc; border-radius: 10px; }
        .grid-scroll { overflow-x: auto; }

        .toast { background: #16a34a; color: #fff; padding: 8px 14px; border-radius: 10px; font-size: 13px; margin-left: auto; }
        .ver { margin-left: auto; font-size: 12px; color: #94a3b8; font-family: ui-monospace, monospace; }
        .modal-backdrop { position: fixed; inset: 0; background: rgba(15,23,42,0.5); display: flex; align-items: center; justify-content: center; z-index: 10000; }
        .modal { background: #fff; border-radius: 14px; padding: 22px; width: 360px; max-width: 90vw; box-shadow: 0 20px 60px rgba(0,0,0,0.3); }
        .modal h3 { margin: 0 0 6px; font-size: 16px; color: #0f172a; }
        .modal-input { width: 100%; margin: 12px 0; box-sizing: border-box; }
        .modal-msg { margin: 12px 0 0; font-size: 13.5px; line-height: 1.45; color: #334155; }
        .modal-actions { display: flex; justify-content: flex-end; gap: 8px; }
        .report { margin-top: 14px; border: 1px solid #fecaca; background: #fef2f2; border-radius: 10px; padding: 10px 14px; font-size: 13px; }
        .report h3 { margin: 0 0 6px; font-size: 13px; color: #b91c1c; }
        .rep-sec { margin: 6px 0; }
        .rep-sec ul { margin: 4px 0 0; padding-left: 18px; }
        .rep-sec li { margin: 2px 0; }

        /* Панель расстановки: сколько уроков размещено и почему не все
           (сводка как в отчётах aSc Timetables / FET). */
        .panel { margin-top: 14px; border: 1px solid #fca5a5; background: #fff7f7; border-radius: 12px; padding: 12px 16px; font-size: 13px; }
        .panel.ok { border-color: #a7f3d0; background: #f0fdf9; }
        .panel-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
        .panel-head h3 { margin: 0; font-size: 14px; color: #0f172a; }
        .panel.ok .panel-head h3 { color: #065f46; }
        .panel-pct { font-size: 20px; font-weight: 700; color: #b91c1c; }
        .panel.ok .panel-pct { color: #059669; }
        .bar { height: 8px; margin: 8px 0; background: #e2e8f0; border-radius: 999px; overflow: hidden; }
        .bar-fill { height: 100%; background: #ef4444; border-radius: 999px; transition: width 0.3s; }
        .panel.ok .bar-fill { background: #10b981; }
        .panel-stats { display: flex; flex-wrap: wrap; gap: 14px; color: #334155; }
        .panel .warn { color: #b45309; font-weight: 600; }
        .panel .good { color: #059669; font-weight: 600; }
        .panel .muted { color: #64748b; }
        .panel-reasons { margin-top: 10px; padding-top: 8px; border-top: 1px dashed #e2e8f0; }
        .panel-reasons b { color: #b91c1c; }
        .reason-row { display: flex; align-items: flex-start; gap: 8px; margin: 6px 0; }
        .reason-badge { flex: 0 0 auto; min-width: 20px; text-align: center; background: #dc2626; color: #fff; border-radius: 999px; font-size: 11.5px; font-weight: 700; padding: 2px 6px; }
        .reason-text { color: #7f1d1d; }
        .reason-who { color: #64748b; }
        .panel-classes { margin-top: 10px; padding-top: 8px; border-top: 1px dashed #e2e8f0; }
        .panel-classes summary { cursor: pointer; color: #0f172a; font-weight: 600; }
        .cls-grid { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
        .cls-cell { display: flex; align-items: center; gap: 8px; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 8px; padding: 4px 10px; }
        .cls-cell.bad { border-color: #fca5a5; background: #fef2f2; }
        .cls-name { font-weight: 600; color: #334155; }
        .cls-val { color: #64748b; font-variant-numeric: tabular-nums; }
        .cls-cell.bad .cls-val { color: #b91c1c; font-weight: 700; }

        /* Обзор «вся школа»: все классы мини-таблицами на одном экране
           (как лист «Timetable for all classes» в aSc Timetables). */
        /* ================= Полный редизайн сетки расписания =================
           Современный стиль «чипов» (как aSc Timetables online): белые
           карточки-таблицы, между ячейками зазор (border-spacing), урок —
           цветной чип со скруглением и белым текстом, светлые фоны. */

        .grid-scroll { overflow-x: auto; }
        .class-block { margin-bottom: 24px; min-width: 560px; }
        .class-block h3 { margin: 0 0 8px; font-size: 15px; font-weight: 800; color: #0f172a; letter-spacing: 0.2px; }
        .class-block table, .mini table { border-collapse: separate; border-spacing: 3px; width: 100%; table-layout: fixed; }

        /* Шапка: номер урока + время звонка, без рамок — как панель. */
        .class-block th, .mini th {
                background: transparent; color: #94a3b8; font-weight: 700;
                font-size: 10px; padding: 2px 1px; text-align: center;
                text-transform: uppercase; letter-spacing: 0.4px;
                user-select: none; -webkit-user-select: none;
        }
        .class-block th .tm { display: block; font-weight: 500; font-size: 8.5px; color: #b6c2d2; text-transform: none; letter-spacing: 0; }

        /* Колонка дней: светлая «пилюля» с днём недели. */
        .class-block td.day, .mini td.day {
                background: #f1f5f9; border-radius: 8px; font-weight: 700;
                color: #475569; font-size: 11px; white-space: nowrap;
                text-align: center; padding: 2px; user-select: none;
        }

        /* Ячейка-слот: светлый фон-«лунка», без рамок. */
        .class-block td, .mini td {
                background: #f8fafc; border-radius: 8px; padding: 0;
                text-align: center; font-size: 12px; height: 40px;
                overflow: hidden; user-select: none; -webkit-user-select: none;
                touch-action: none; position: relative;
        }
        .mini td { height: 26px; font-size: 10px; }

        /* Урок-чип: цветной блок на всю ячейку с внутренним отступом. */
        .chip {
                margin: 2px; height: calc(100% - 4px); border-radius: 6px;
                color: #fff; font-weight: 700; padding: 2px 4px;
                display: flex; flex-direction: column; align-items: center;
                justify-content: center; gap: 1px; line-height: 1.15;
                overflow: hidden; cursor: grab;
                box-shadow: 0 1px 2px rgba(15, 23, 42, 0.18);
        }
        td.filled:active .chip { cursor: grabbing; }
        .chip b { font-size: 11.5px; font-weight: 700; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .chip span { font-size: 9.5px; font-weight: 500; opacity: 0.92; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        /* Мини-чип обзора: просто короткое имя предмета мелким шрифтом. */
        .mini .chip { font-size: 9.5px; font-weight: 700; }
        .chip.conflict { box-shadow: 0 0 0 2px #fca5a5, 0 1px 2px rgba(15, 23, 42, 0.25); }

        /* Состояния: выделение и цель перетаскивания — на ячейке. */
        td.filled.selected { outline: 3px solid #2563eb; outline-offset: -3px; border-radius: 10px; }
        td.drop-target { outline: 3px dashed #2563eb; outline-offset: -3px; background: #dbeafe !important; border-radius: 10px; }
        .cell-x {
                position: absolute; top: 3px; right: 3px; border: none;
                background: rgba(255,255,255,0.35); color: #fff; width: 16px; height: 16px;
                line-height: 14px; border-radius: 5px; cursor: pointer; font-size: 10px; padding: 0;
        }
        .cell-x:hover { background: rgba(255,255,255,0.55); }
        .drag-ghost { position: fixed; z-index: 9999; pointer-events: none; transform: translate(-50%, -50%); background: #1d4ed8; color: #fff; padding: 3px 10px; border-radius: 8px; font-size: 12px; font-weight: 600; max-width: 220px; box-shadow: 0 6px 18px rgba(0,0,0,0.3); }

        .class-block table.compact { border-spacing: 1px; }
        .class-block table.compact th, .class-block table.compact td { height: 24px; font-size: 9px; }
        .class-block table.compact .chip b { font-size: 8.5px; }
        .class-block table.compact .chip span { display: none; }
        .class-block.compact h3 { font-size: 11px; margin: 0 0 4px; }
        .mini { border: 1px solid #e5e9f0; border-radius: 12px; padding: 10px; background: #fff; min-width: 0; box-shadow: 0 1px 3px rgba(15, 23, 42, 0.06); }
        .mini .mini-title { display: block; background: none; border: none; padding: 0; margin: 0 0 8px; font-family: inherit; font-size: 13.5px; font-weight: 800; color: #1d4ed8; cursor: pointer; text-align: left; }
        /* Подгруппа: бейдж + акцентная полоса слева на карточке */
        .sub-badge { background: #ede9fe; color: #6d28d9; font-size: 9px; font-weight: 700; padding: 1px 7px; border-radius: 6px; margin-left: 6px; vertical-align: 1px; white-space: nowrap; }
        .mini.is-sub, .class-block.is-sub { border-left: 3px solid #8b5cf6; }
        /* Половинки разделённой ячейки (параллельные подгруппы) */
        .chip.half { height: 50%; margin: 1px 2px; font-size: 9.5px; }
        .chip.half b { font-size: 9.5px; }
        .chip.half span { font-size: 8px; }
        .mini .mini-title:hover { text-decoration: underline; }
        .mini th.d, .mini td.day { width: 34px; }
        .overview { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 14px; align-items: start; }
</style>
