export namespace main {
	
	export class UpdateInfo {
	    available: boolean;
	    current: string;
	    version?: string;
	    notes?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.current = source["current"];
	        this.version = source["version"];
	        this.notes = source["notes"];
	    }
	}

}

export namespace onec {
	
	export class Section {
	    id: string;
	    group: string;
	    title: string;
	    kind: string;
	    text?: string;
	    lines: number;
	    editable: boolean;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Section(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.group = source["group"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.lines = source["lines"];
	        this.editable = source["editable"];
	        this.note = source["note"];
	    }
	}
	export class Document {
	    path: string;
	    name: string;
	    synonym?: string;
	    kind: string;
	    format: string;
	    sections: Section[];
	    canSave: boolean;
	    lazy?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.synonym = source["synonym"];
	        this.kind = source["kind"];
	        this.format = source["format"];
	        this.sections = this.convertValues(source["sections"], Section);
	        this.canSave = source["canSave"];
	        this.lazy = source["lazy"];
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
	export class GroupCount {
	    name: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new GroupCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	    }
	}
	
	export class SectionContent {
	    text: string;
	    note?: string;
	    lines: number;
	    editable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SectionContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.note = source["note"];
	        this.lines = source["lines"];
	        this.editable = source["editable"];
	    }
	}
	export class SummaryModule {
	    title: string;
	    text: string;
	    lines: number;
	    truncated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SummaryModule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.text = source["text"];
	        this.lines = source["lines"];
	        this.truncated = source["truncated"];
	    }
	}
	export class Summary {
	    kind: string;
	    format: string;
	    name: string;
	    synonym?: string;
	    sections: number;
	    modules: number;
	    groups: GroupCount[];
	    main?: SummaryModule;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.format = source["format"];
	        this.name = source["name"];
	        this.synonym = source["synonym"];
	        this.sections = source["sections"];
	        this.modules = source["modules"];
	        this.groups = this.convertValues(source["groups"], GroupCount);
	        this.main = this.convertValues(source["main"], SummaryModule);
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

export namespace preview {
	
	export class Sheet {
	    name: string;
	    rows: string[][];
	    truncated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Sheet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	    }
	}
	export class Result {
	    kind: string;
	    path: string;
	    name: string;
	    ext: string;
	    size: number;
	    mod: number;
	    text?: string;
	    encoding?: string;
	    truncated?: boolean;
	    sheets?: Sheet[];
	    note?: string;
	    url?: string;
	    onec?: onec.Summary;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.size = source["size"];
	        this.mod = source["mod"];
	        this.text = source["text"];
	        this.encoding = source["encoding"];
	        this.truncated = source["truncated"];
	        this.sheets = this.convertValues(source["sheets"], Sheet);
	        this.note = source["note"];
	        this.url = source["url"];
	        this.onec = this.convertValues(source["onec"], onec.Summary);
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

