export namespace auth {
	
	export class CodexAuthSessionView {
	    sessionId: string;
	    authUrl: string;
	    callbackUrl: string;
	    status: string;
	    error?: string;
	    accountId?: string;
	    email?: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexAuthSessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.callbackUrl = source["callbackUrl"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.accountId = source["accountId"];
	        this.email = source["email"];
	    }
	}
	export class CodexAuthStart {
	    sessionId: string;
	    authUrl: string;
	    callbackUrl: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexAuthStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.callbackUrl = source["callbackUrl"];
	        this.status = source["status"];
	    }
	}
	export class CodexAuthSyncResult {
	    targetPath: string;
	    backupPath?: string;
	    fileExisted: boolean;
	    backupCreated: boolean;
	    updatedFields: string[];
	    accountID: string;
	    provider: string;
	    syncedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexAuthSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetPath = source["targetPath"];
	        this.backupPath = source["backupPath"];
	        this.fileExisted = source["fileExisted"];
	        this.backupCreated = source["backupCreated"];
	        this.updatedFields = source["updatedFields"];
	        this.accountID = source["accountID"];
	        this.provider = source["provider"];
	        this.syncedAt = source["syncedAt"];
	    }
	}
	export class KiloAuthSyncResult {
	    targetPath: string;
	    fileExisted: boolean;
	    openAICreated: boolean;
	    updatedFields: string[];
	    accountID: string;
	    provider: string;
	    syncedExpires: number;
	    syncedExpiresAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new KiloAuthSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetPath = source["targetPath"];
	        this.fileExisted = source["fileExisted"];
	        this.openAICreated = source["openAICreated"];
	        this.updatedFields = source["updatedFields"];
	        this.accountID = source["accountID"];
	        this.provider = source["provider"];
	        this.syncedExpires = source["syncedExpires"];
	        this.syncedExpiresAt = source["syncedExpiresAt"];
	    }
	}
	export class KiroAuthSessionView {
	    sessionId: string;
	    authUrl: string;
	    verificationUrl?: string;
	    userCode?: string;
	    expiresAt?: number;
	    status: string;
	    error?: string;
	    accountId?: string;
	    email?: string;
	    authMethod?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new KiroAuthSessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.verificationUrl = source["verificationUrl"];
	        this.userCode = source["userCode"];
	        this.expiresAt = source["expiresAt"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.accountId = source["accountId"];
	        this.email = source["email"];
	        this.authMethod = source["authMethod"];
	        this.provider = source["provider"];
	    }
	}
	export class KiroAuthStart {
	    sessionId: string;
	    authUrl: string;
	    verificationUrl?: string;
	    userCode: string;
	    expiresAt?: number;
	    status: string;
	    authMethod?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new KiroAuthStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.verificationUrl = source["verificationUrl"];
	        this.userCode = source["userCode"];
	        this.expiresAt = source["expiresAt"];
	        this.status = source["status"];
	        this.authMethod = source["authMethod"];
	        this.provider = source["provider"];
	    }
	}
	export class OpencodeAuthSyncResult {
	    targetPath: string;
	    fileExisted: boolean;
	    openAICreated: boolean;
	    updatedFields: string[];
	    accountID: string;
	    provider: string;
	    syncedExpires: number;
	    syncedExpiresAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new OpencodeAuthSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetPath = source["targetPath"];
	        this.fileExisted = source["fileExisted"];
	        this.openAICreated = source["openAICreated"];
	        this.updatedFields = source["updatedFields"];
	        this.accountID = source["accountID"];
	        this.provider = source["provider"];
	        this.syncedExpires = source["syncedExpires"];
	        this.syncedExpiresAt = source["syncedExpiresAt"];
	    }
	}

}

export namespace config {
	
	export class QuotaBucket {
	    name: string;
	    used?: number;
	    total?: number;
	    remaining?: number;
	    percent?: number;
	    resetAt?: number;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new QuotaBucket(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.used = source["used"];
	        this.total = source["total"];
	        this.remaining = source["remaining"];
	        this.percent = source["percent"];
	        this.resetAt = source["resetAt"];
	        this.status = source["status"];
	    }
	}
	export class QuotaInfo {
	    status?: string;
	    summary?: string;
	    source?: string;
	    error?: string;
	    lastCheckedAt?: number;
	    buckets?: QuotaBucket[];
	
	    static createFrom(source: any = {}) {
	        return new QuotaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.summary = source["summary"];
	        this.source = source["source"];
	        this.error = source["error"];
	        this.lastCheckedAt = source["lastCheckedAt"];
	        this.buckets = this.convertValues(source["buckets"], QuotaBucket);
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
	export class Account {
	    id: string;
	    provider?: string;
	    email: string;
	    accountId?: string;
	    planType?: string;
	    quota?: QuotaInfo;
	    accessToken: string;
	    refreshToken: string;
	    idToken?: string;
	    clientId?: string;
	    clientSecret?: string;
	    expiresAt?: number;
	    enabled: boolean;
	    banned?: boolean;
	    bannedReason?: string;
	    healthState?: string;
	    healthReason?: string;
	    cooldownUntil?: number;
	    lastFailureAt?: number;
	    consecutiveFailures?: number;
	    lastError?: string;
	    requestCount?: number;
	    errorCount?: number;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    lastUsed?: number;
	    lastRefresh?: number;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.email = source["email"];
	        this.accountId = source["accountId"];
	        this.planType = source["planType"];
	        this.quota = this.convertValues(source["quota"], QuotaInfo);
	        this.accessToken = source["accessToken"];
	        this.refreshToken = source["refreshToken"];
	        this.idToken = source["idToken"];
	        this.clientId = source["clientId"];
	        this.clientSecret = source["clientSecret"];
	        this.expiresAt = source["expiresAt"];
	        this.enabled = source["enabled"];
	        this.banned = source["banned"];
	        this.bannedReason = source["bannedReason"];
	        this.healthState = source["healthState"];
	        this.healthReason = source["healthReason"];
	        this.cooldownUntil = source["cooldownUntil"];
	        this.lastFailureAt = source["lastFailureAt"];
	        this.consecutiveFailures = source["consecutiveFailures"];
	        this.lastError = source["lastError"];
	        this.requestCount = source["requestCount"];
	        this.errorCount = source["errorCount"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.lastUsed = source["lastUsed"];
	        this.lastRefresh = source["lastRefresh"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class ProxyStats {
	    totalRequests: number;
	    successRequests: number;
	    failedRequests: number;
	    promptTokens: number;
	    completionTokens: number;
	    totalTokens: number;
	    lastRequestAt?: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalRequests = source["totalRequests"];
	        this.successRequests = source["successRequests"];
	        this.failedRequests = source["failedRequests"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.lastRequestAt = source["lastRequestAt"];
	    }
	}
	
	
	export class StartupWarning {
	    code: string;
	    filePath: string;
	    backupPath?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupWarning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.filePath = source["filePath"];
	        this.backupPath = source["backupPath"];
	        this.message = source["message"];
	    }
	}

}

export namespace logger {
	
	export class Entry {
	    timestamp: number;
	    level: string;
	    scope: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.level = source["level"];
	        this.scope = source["scope"];
	        this.message = source["message"];
	    }
	}

}

export namespace main {
	
	export class State {
	    authMode: string;
	    proxyPort: number;
	    proxyUrl: string;
	    proxyBindAddress: string;
	    allowLan: boolean;
	    autoStartProxy: boolean;
	    proxyApiKey?: string;
	    authorizationMode?: boolean;
	    schedulingMode?: string;
	    circuitBreaker?: boolean;
	    circuitSteps?: number[];
	    proxyRunning: boolean;
	    availableCount: number;
	    accounts: config.Account[];
	    stats: config.ProxyStats;
	    startupWarnings?: config.StartupWarning[];
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.authMode = source["authMode"];
	        this.proxyPort = source["proxyPort"];
	        this.proxyUrl = source["proxyUrl"];
	        this.proxyBindAddress = source["proxyBindAddress"];
	        this.allowLan = source["allowLan"];
	        this.autoStartProxy = source["autoStartProxy"];
	        this.proxyApiKey = source["proxyApiKey"];
	        this.authorizationMode = source["authorizationMode"];
	        this.schedulingMode = source["schedulingMode"];
	        this.circuitBreaker = source["circuitBreaker"];
	        this.circuitSteps = source["circuitSteps"];
	        this.proxyRunning = source["proxyRunning"];
	        this.availableCount = source["availableCount"];
	        this.accounts = this.convertValues(source["accounts"], config.Account);
	        this.stats = this.convertValues(source["stats"], config.ProxyStats);
	        this.startupWarnings = this.convertValues(source["startupWarnings"], config.StartupWarning);
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

