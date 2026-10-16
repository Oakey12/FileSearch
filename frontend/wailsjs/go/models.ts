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
	    image?: string;
	
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
	        this.image = source["image"];
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

