export namespace domain {
	
	export class Constraint {
	    id: number;
	    school_id: number;
	    type: string;
	    entity_type: string;
	    entity_id: number;
	    day_of_week?: number;
	    timeslot_start?: number;
	    timeslot_end?: number;
	    weight: number;
	    is_hard: boolean;
	    params_json: string;
	
	    static createFrom(source: any = {}) {
	        return new Constraint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.type = source["type"];
	        this.entity_type = source["entity_type"];
	        this.entity_id = source["entity_id"];
	        this.day_of_week = source["day_of_week"];
	        this.timeslot_start = source["timeslot_start"];
	        this.timeslot_end = source["timeslot_end"];
	        this.weight = source["weight"];
	        this.is_hard = source["is_hard"];
	        this.params_json = source["params_json"];
	    }
	}
	export class Lesson {
	    id: number;
	    school_id: number;
	    class_id: number;
	    subject_id: number;
	    teacher_id: number;
	    hours_per_week: number;
	    min_gap_days: number;
	    can_split: boolean;
	    preferred_rooms: string;
	
	    static createFrom(source: any = {}) {
	        return new Lesson(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.class_id = source["class_id"];
	        this.subject_id = source["subject_id"];
	        this.teacher_id = source["teacher_id"];
	        this.hours_per_week = source["hours_per_week"];
	        this.min_gap_days = source["min_gap_days"];
	        this.can_split = source["can_split"];
	        this.preferred_rooms = source["preferred_rooms"];
	    }
	}
	export class Room {
	    id: number;
	    school_id: number;
	    name: string;
	    capacity: number;
	    room_type: string;
	
	    static createFrom(source: any = {}) {
	        return new Room(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.name = source["name"];
	        this.capacity = source["capacity"];
	        this.room_type = source["room_type"];
	    }
	}
	export class ScheduleEntry {
	    id: number;
	    school_id: number;
	    lesson_id: number;
	    class_id: number;
	    teacher_id: number;
	    subject_id: number;
	    room_id: number;
	    day_of_week: number;
	    timeslot: number;
	    week_type: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.lesson_id = source["lesson_id"];
	        this.class_id = source["class_id"];
	        this.teacher_id = source["teacher_id"];
	        this.subject_id = source["subject_id"];
	        this.room_id = source["room_id"];
	        this.day_of_week = source["day_of_week"];
	        this.timeslot = source["timeslot"];
	        this.week_type = source["week_type"];
	    }
	}
	export class School {
	    id: number;
	    name: string;
	    settings_json: string;
	
	    static createFrom(source: any = {}) {
	        return new School(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.settings_json = source["settings_json"];
	    }
	}
	export class SchoolClass {
	    id: number;
	    school_id: number;
	    name: string;
	    grade: number;
	    student_count: number;
	    subgroup_of?: number;
	
	    static createFrom(source: any = {}) {
	        return new SchoolClass(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.name = source["name"];
	        this.grade = source["grade"];
	        this.student_count = source["student_count"];
	        this.subgroup_of = source["subgroup_of"];
	    }
	}
	export class Subject {
	    id: number;
	    school_id: number;
	    name: string;
	    short_name: string;
	    requires_room_type: string;
	
	    static createFrom(source: any = {}) {
	        return new Subject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.name = source["name"];
	        this.short_name = source["short_name"];
	        this.requires_room_type = source["requires_room_type"];
	    }
	}
	export class Teacher {
	    id: number;
	    school_id: number;
	    name: string;
	    short_name: string;
	    max_hours_per_week: number;
	    preferences_json: string;
	
	    static createFrom(source: any = {}) {
	        return new Teacher(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.school_id = source["school_id"];
	        this.name = source["name"];
	        this.short_name = source["short_name"];
	        this.max_hours_per_week = source["max_hours_per_week"];
	        this.preferences_json = source["preferences_json"];
	    }
	}

}

export namespace io {
	
	export class Snapshot {
	    school: domain.School;
	    teachers: domain.Teacher[];
	    subjects: domain.Subject[];
	    classes: domain.SchoolClass[];
	    rooms: domain.Room[];
	    lessons: domain.Lesson[];
	    constraints: domain.Constraint[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.school = this.convertValues(source["school"], domain.School);
	        this.teachers = this.convertValues(source["teachers"], domain.Teacher);
	        this.subjects = this.convertValues(source["subjects"], domain.Subject);
	        this.classes = this.convertValues(source["classes"], domain.SchoolClass);
	        this.rooms = this.convertValues(source["rooms"], domain.Room);
	        this.lessons = this.convertValues(source["lessons"], domain.Lesson);
	        this.constraints = this.convertValues(source["constraints"], domain.Constraint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace solver {
	
	export class Result {
	    entries: domain.ScheduleEntry[];
	    placed: number;
	    total: number;
	    violations: number;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], domain.ScheduleEntry);
	        this.placed = source["placed"];
	        this.total = source["total"];
	        this.violations = source["violations"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

