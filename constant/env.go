package constant

var StreamingTimeout int

// ===================== BEGIN NEW: TTFT timeout config =====================
// TTFTTimeoutSeconds controls timeout before receiving the first valid stream
// chunk ("time to first token"), in seconds.
//
// Value semantics:
// - 0: disabled
// - >0: enabled
//
// This timeout is complementary to StreamingTimeout:
// - TTFTTimeoutSeconds: before first token only
// - StreamingTimeout: idle timeout between stream chunks
// ====================== END NEW: TTFT timeout config ======================
var TTFTTimeoutSeconds int

var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AnonymousRequestBodyLimitKB int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// TrustedRedirectDomains is a list of trusted domains for redirect URL validation.
// Domains support subdomain matching (e.g., "example.com" matches "sub.example.com").
var TrustedRedirectDomains []string
