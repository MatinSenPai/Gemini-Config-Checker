export namespace scan {
	
	export class Profile {
	    enabled: boolean;
	    finalmask: string;
	    fingerprint: string;
	    alpn: string;
	    cipherSuites: string;
	    cleanIP: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.finalmask = source["finalmask"];
	        this.fingerprint = source["fingerprint"];
	        this.alpn = source["alpn"];
	        this.cipherSuites = source["cipherSuites"];
	        this.cleanIP = source["cleanIP"];
	    }
	}
	export class ChainRequest {
	    base: string;
	    link: string;
	    profile: Profile;
	    flavor: string;
	
	    static createFrom(source: any = {}) {
	        return new ChainRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base = source["base"];
	        this.link = source["link"];
	        this.profile = this.convertValues(source["profile"], Profile);
	        this.flavor = source["flavor"];
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
	
	export class Request {
	    base: string;
	    source: string;
	    custom: string;
	    pasted: string;
	    need: string;
	    level: string;
	    mode: string;
	    limit: number;
	    concurrency: number;
	    browserConc: number;
	    timeout: number;
	    profile: Profile;
	
	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base = source["base"];
	        this.source = source["source"];
	        this.custom = source["custom"];
	        this.pasted = source["pasted"];
	        this.need = source["need"];
	        this.level = source["level"];
	        this.mode = source["mode"];
	        this.limit = source["limit"];
	        this.concurrency = source["concurrency"];
	        this.browserConc = source["browserConc"];
	        this.timeout = source["timeout"];
	        this.profile = this.convertValues(source["profile"], Profile);
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
	export class Result {
	    name: string;
	    link: string;
	    status: string;
	    ms: number;
	    studio: string;
	    gemini: string;
	    err?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.link = source["link"];
	        this.status = source["status"];
	        this.ms = source["ms"];
	        this.studio = source["studio"];
	        this.gemini = source["gemini"];
	        this.err = source["err"];
	    }
	}
	export class Status {
	    state: string;
	    msg: string;
	    auth: string;
	    authMsg: string;
	    level: string;
	    mode: string;
	    browser: boolean;
	    base?: Result;
	    total: number;
	    done: number;
	    skipped: number;
	    results: Result[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.msg = source["msg"];
	        this.auth = source["auth"];
	        this.authMsg = source["authMsg"];
	        this.level = source["level"];
	        this.mode = source["mode"];
	        this.browser = source["browser"];
	        this.base = this.convertValues(source["base"], Result);
	        this.total = source["total"];
	        this.done = source["done"];
	        this.skipped = source["skipped"];
	        this.results = this.convertValues(source["results"], Result);
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

