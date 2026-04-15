export namespace auth {
	
	export class AuthSessionView {
	    sessionId: string;
	    authUrl: string;
	    callbackUrl?: string;
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
	        return new AuthSessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.callbackUrl = source["callbackUrl"];
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
	export class AuthStart {
	    sessionId: string;
	    authUrl: string;
	    callbackUrl?: string;
	    verificationUrl?: string;
	    userCode?: string;
	    expiresAt?: number;
	    status: string;
	    authMethod?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.authUrl = source["authUrl"];
	        this.callbackUrl = source["callbackUrl"];
	        this.verificationUrl = source["verificationUrl"];
	        this.userCode = source["userCode"];
	        this.expiresAt = source["expiresAt"];
	        this.status = source["status"];
	        this.authMethod = source["authMethod"];
	        this.provider = source["provider"];
	    }
	}
	export class AuthSyncResult {
	    target: string;
	    targetPath: string;
	    fileExisted: boolean;
	    openAICreated: boolean;
	    backupPath?: string;
	    backupCreated: boolean;
	    updatedFields: string[];
	    accountID: string;
	    provider: string;
	    syncedExpires: number;
	    syncedExpiresAt?: string;
	    syncedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.targetPath = source["targetPath"];
	        this.fileExisted = source["fileExisted"];
	        this.openAICreated = source["openAICreated"];
	        this.backupPath = source["backupPath"];
	        this.backupCreated = source["backupCreated"];
	        this.updatedFields = source["updatedFields"];
	        this.accountID = source["accountID"];
	        this.provider = source["provider"];
	        this.syncedExpires = source["syncedExpires"];
	        this.syncedExpiresAt = source["syncedExpiresAt"];
	        this.syncedAt = source["syncedAt"];
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
	    authMethod?: string;
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
	        this.authMethod = source["authMethod"];
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
	    event: string;
	    requestId?: string;
	    fields?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.level = source["level"];
	        this.scope = source["scope"];
	        this.event = source["event"];
	        this.requestId = source["requestId"];
	        this.fields = source["fields"];
	    }
	}

}

export namespace main {
	
	export class CLISyncFile {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new CLISyncFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class CLISyncFileInput {
	    target: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new CLISyncFileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.path = source["path"];
	    }
	}
	export class CLISyncResult {
	    id: string;
	    label: string;
	    model?: string;
	    currentBaseUrl?: string;
	    files: CLISyncFile[];
	
	    static createFrom(source: any = {}) {
	        return new CLISyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.model = source["model"];
	        this.currentBaseUrl = source["currentBaseUrl"];
	        this.files = this.convertValues(source["files"], CLISyncFile);
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
	export class CLISyncStatus {
	    id: string;
	    label: string;
	    installed: boolean;
	    installPath?: string;
	    version?: string;
	    synced: boolean;
	    currentBaseUrl?: string;
	    currentModel?: string;
	    files: CLISyncFile[];
	
	    static createFrom(source: any = {}) {
	        return new CLISyncStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.installed = source["installed"];
	        this.installPath = source["installPath"];
	        this.version = source["version"];
	        this.synced = source["synced"];
	        this.currentBaseUrl = source["currentBaseUrl"];
	        this.currentModel = source["currentModel"];
	        this.files = this.convertValues(source["files"], CLISyncFile);
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
	export class ModelCatalogItem {
	    id: string;
	    ownedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelCatalogItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ownedBy = source["ownedBy"];
	    }
	}
	export class ProxyCloudflaredStatus {
	    enabled: boolean;
	    mode: string;
	    token: string;
	    useHttp2: boolean;
	    installed: boolean;
	    version?: string;
	    running: boolean;
	    url?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyCloudflaredStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.mode = source["mode"];
	        this.token = source["token"];
	        this.useHttp2 = source["useHttp2"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.running = source["running"];
	        this.url = source["url"];
	        this.error = source["error"];
	    }
	}
	export class ProxySettingsUpdateResult {
	    restartedProxy: boolean;
	    restartedCloudflared: boolean;
	    generatedApiKey?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxySettingsUpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.restartedProxy = source["restartedProxy"];
	        this.restartedCloudflared = source["restartedCloudflared"];
	        this.generatedApiKey = source["generatedApiKey"];
	    }
	}
	export class ProxyStatus {
	    running: boolean;
	    port: number;
	    url: string;
	    bindAddress: string;
	    allowLan: boolean;
	    autoStartProxy: boolean;
	    proxyApiKey: string;
	    authorizationMode: boolean;
	    schedulingMode: string;
	    cloudflared: ProxyCloudflaredStatus;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.bindAddress = source["bindAddress"];
	        this.allowLan = source["allowLan"];
	        this.autoStartProxy = source["autoStartProxy"];
	        this.proxyApiKey = source["proxyApiKey"];
	        this.authorizationMode = source["authorizationMode"];
	        this.schedulingMode = source["schedulingMode"];
	        this.cloudflared = this.convertValues(source["cloudflared"], ProxyCloudflaredStatus);
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
	export class RunAccountActionInput {
	    accountId: string;
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new RunAccountActionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.action = source["action"];
	    }
	}
	export class RunCLISyncInput {
	    target: string;
	    model?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunCLISyncInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.model = source["model"];
	    }
	}
	export class RunQuotaActionInput {
	    action: string;
	    accountId?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunQuotaActionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.accountId = source["accountId"];
	    }
	}
	export class RunSystemActionInput {
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new RunSystemActionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	    }
	}
	export class SaveCLISyncFileInput {
	    target: string;
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveCLISyncFileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}
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
	    proxyRunning: boolean;
	    availableCount: number;
	    accounts: config.Account[];
	    stats: config.ProxyStats;
	    startupWarnings?: config.StartupWarning[];
	    traySupported: boolean;
	    trayAvailable: boolean;
	
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
	        this.proxyRunning = source["proxyRunning"];
	        this.availableCount = source["availableCount"];
	        this.accounts = this.convertValues(source["accounts"], config.Account);
	        this.stats = this.convertValues(source["stats"], config.ProxyStats);
	        this.startupWarnings = this.convertValues(source["startupWarnings"], config.StartupWarning);
	        this.traySupported = source["traySupported"];
	        this.trayAvailable = source["trayAvailable"];
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
	export class UpdateCloudflaredSettingsInput {
	    mode?: string;
	    token?: string;
	    useHttp2?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCloudflaredSettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.token = source["token"];
	        this.useHttp2 = source["useHttp2"];
	    }
	}
	export class UpdateProxySettingsInput {
	    port?: number;
	    allowLan?: boolean;
	    autoStartProxy?: boolean;
	    proxyApiKey?: string;
	    regenerateApiKey?: boolean;
	    authorizationMode?: boolean;
	    schedulingMode?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateProxySettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.allowLan = source["allowLan"];
	        this.autoStartProxy = source["autoStartProxy"];
	        this.proxyApiKey = source["proxyApiKey"];
	        this.regenerateApiKey = source["regenerateApiKey"];
	        this.authorizationMode = source["authorizationMode"];
	        this.schedulingMode = source["schedulingMode"];
	    }
	}

}

