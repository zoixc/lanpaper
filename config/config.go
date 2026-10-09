// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

type RateConfig struct {
	PublicPerMin int `json:"publicPerMin"`
	UploadPerMin int `json:"uploadPerMin"`
	Burst        int `json:"burst"`
}

type CompressionConfig struct {
	Quality int `json:"quality"` // 1-100, JPEG quality
	Scale   int `json:"scale"`   // 1-100, percentage of max dimensions
}

// HistoryConfig bounds the version history kept for replaced media.
// Limit is the number of previous versions per link (0 disables history);
// MaxMB is the budget shared by all links (0 means unlimited).
type HistoryConfig struct {
	Limit int `json:"limit"`
	MaxMB int `json:"maxMB"`
}

type Config struct {
	Port                 string            `json:"port"`
	MaxUploadMB          int               `json:"maxUploadMB"`
	MaxImages            int               `json:"maxImages"`
	MaxConcurrentUploads int               `json:"maxConcurrentUploads"`
	MaxWalkDepth         int               `json:"maxWalkDepth"`
	ExternalImageDir     string            `json:"externalImageDir"`
	AdminUser            string            `json:"adminUser"`
	AdminPass            string            `json:"adminPass"`
	DisableAuth          bool              `json:"disableAuth,omitempty"`
	InsecureSkipVerify   bool              `json:"insecureSkipVerify,omitempty"`
	ProxyHost            string            `json:"proxyHost,omitempty"`
	ProxyPort            string            `json:"proxyPort,omitempty"`
	ProxyType            string            `json:"proxyType,omitempty"`
	ProxyUsername        string            `json:"proxyUsername,omitempty"`
	ProxyPassword        string            `json:"proxyPassword,omitempty"`
	Rate                 RateConfig        `json:"rate"`
	Compression          CompressionConfig `json:"compression"`
	// TrustedProxy lists the reverse proxies in front of Lanpaper: one or
	// more IPs or CIDRs, comma-separated ("192.168.20.1,172.24.0.0/16").
	// The rightmost X-Forwarded-For entry and X-Forwarded-Proto/-Host are
	// trusted only for requests from these addresses. X-Real-IP is never read.
	// A list is needed whenever a proxy reaches the container
	// through more than one hop or address — for example a proxy on the LAN
	// plus the Docker bridge gateway the container actually sees.
	TrustedProxy string `json:"trustedProxy,omitempty"`

	// History bounds the versions kept when a link's media is replaced.
	History HistoryConfig `json:"history"`
	// PlaylistMax is how many extra items one link may serve from the same URL.
	PlaylistMax int `json:"playlistMax"`
	// AllowEmbed drops X-Frame-Options and the CSP sandbox from public media so
	// dashboards and digital frames can embed it in an <iframe>.
	AllowEmbed bool `json:"allowEmbed,omitempty"`
	// CORSOrigins lists the browser origins allowed to read public media with
	// fetch()/canvas ("*" allows every origin). Empty keeps CORS closed.
	CORSOrigins []string `json:"corsOrigins,omitempty"`
	// PublishKeys authorize POST /api/upload and POST /api/link without admin
	// credentials. Environment-only (PUBLISH_KEYS): a secret is never written
	// to config.json, so the field is deliberately not serialized.
	PublishKeys []string `json:"-"`

	// TLSCertFile and TLSKeyFile make the server terminate HTTPS itself instead
	// of relying on a reverse proxy. Both must be set, or neither: a partial
	// configuration is a startup error rather than a silent downgrade. The
	// paths are not secrets, so they may live in config.json.
	TLSCertFile string `json:"tlsCertFile,omitempty"`
	TLSKeyFile  string `json:"tlsKeyFile,omitempty"`
}

// TLSEnabled reports whether the server should listen with TLS.
func TLSEnabled() bool {
	return Current.TLSCertFile != "" && Current.TLSKeyFile != ""
}

// TLSMisconfigured reports a certificate without a key (or the reverse). The
// caller must not start: serving plaintext to an operator who configured TLS
// would expose admin credentials and every token URL in the clear.
func TLSMisconfigured() bool {
	return (Current.TLSCertFile == "") != (Current.TLSKeyFile == "")
}

var Current Config

// cachedProxy caches the parsed TrustedProxy value set during Load/validate.
// Stored as *parsedProxy via atomic pointer to avoid any lock on the hot path.
// The lists stay immutable after validate(), so readers need no lock.
type parsedProxy struct {
	ips   []*net.IP
	cidrs []*net.IPNet
}

var cachedProxyPtr atomic.Pointer[parsedProxy]

// Load loads configuration with priority: env vars > config.json > defaults
func Load() {
	// Step 1: Load defaults
	Current = Config{
		Port:                 "8080",
		MaxUploadMB:          DefaultMaxUploadMB,
		MaxConcurrentUploads: DefaultMaxConcurrentUploads,
		MaxWalkDepth:         DefaultMaxWalkDepth,
		ExternalImageDir:     "external/images",
		ProxyType:            "http",
		Rate: RateConfig{
			PublicPerMin: DefaultPublicRatePerMin,
			UploadPerMin: DefaultUploadRatePerMin,
			Burst:        DefaultRateBurst,
		},
		Compression: CompressionConfig{
			Quality: DefaultCompressionQuality,
			Scale:   DefaultCompressionScale,
		},
		History: HistoryConfig{
			Limit: DefaultHistoryLimit,
			MaxMB: DefaultHistoryMaxMB,
		},
		PlaylistMax: DefaultPlaylistMax,
	}

	// Step 2: Override with config.json (if exists)
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	// Step 3: Override with environment variables (highest priority)
	envString("PORT", &Current.Port)
	envInt("MAX_UPLOAD_MB", &Current.MaxUploadMB)
	envInt("MAX_IMAGES", &Current.MaxImages)
	envInt("MAX_CONCURRENT_UPLOADS", &Current.MaxConcurrentUploads)
	envInt("MAX_WALK_DEPTH", &Current.MaxWalkDepth)
	envString("EXTERNAL_IMAGE_DIR", &Current.ExternalImageDir)
	envString("ADMIN_USER", &Current.AdminUser)
	envString("ADMIN_PASS", &Current.AdminPass)
	envBool("DISABLE_AUTH", &Current.DisableAuth)
	envBool("INSECURE_SKIP_VERIFY", &Current.InsecureSkipVerify)
	envString("PROXY_HOST", &Current.ProxyHost)
	envString("PROXY_PORT", &Current.ProxyPort)
	envString("PROXY_TYPE", &Current.ProxyType)
	// PROXY_USER / PROXY_PASS are accepted as shorter aliases.
	envString("PROXY_USER", &Current.ProxyUsername)
	envString("PROXY_USERNAME", &Current.ProxyUsername)
	envString("PROXY_PASS", &Current.ProxyPassword)
	envString("PROXY_PASSWORD", &Current.ProxyPassword)
	envString("TRUSTED_PROXY", &Current.TrustedProxy)
	envInt("RATE_PUBLIC_PER_MIN", &Current.Rate.PublicPerMin)
	envInt("RATE_UPLOAD_PER_MIN", &Current.Rate.UploadPerMin)
	envInt("RATE_BURST", &Current.Rate.Burst)
	envInt("COMPRESSION_QUALITY", &Current.Compression.Quality)
	envInt("COMPRESSION_SCALE", &Current.Compression.Scale)
	envInt("HISTORY_LIMIT", &Current.History.Limit)
	envInt("HISTORY_MAX_MB", &Current.History.MaxMB)
	envInt("PLAYLIST_MAX", &Current.PlaylistMax)
	envBool("ALLOW_EMBED", &Current.AllowEmbed)
	envList("CORS_ORIGINS", &Current.CORSOrigins)
	envString("TLS_CERT_FILE", &Current.TLSCertFile)
	envString("TLS_KEY_FILE", &Current.TLSKeyFile)
	// PUBLISH_KEYS is a comma-separated list of API keys. Like ADMIN_PASS it is
	// a secret and is only read from the environment, never from config.json.
	envList("PUBLISH_KEYS", &Current.PublishKeys)

	validate()

	mode := "compressed"
	if Current.Compression.Quality == 100 && Current.Compression.Scale == 100 {
		mode = "lossless"
	}
	log.Printf("Config loaded: compression quality=%d scale=%d (%s)",
		Current.Compression.Quality, Current.Compression.Scale, mode)
	if Current.History.Limit > 0 {
		log.Printf("Version history: up to %d previous version(s) per link, %d MB total budget",
			Current.History.Limit, Current.History.MaxMB)
	}
	if len(Current.PublishKeys) > 0 {
		log.Printf("Publish API keys: %d key(s) accepted for POST /api/upload and POST /api/link",
			len(Current.PublishKeys))
	}
	if Current.AllowEmbed {
		log.Println("Warning: ALLOW_EMBED=true — public media can be framed by other sites (X-Frame-Options and the CSP sandbox are not sent).")
	}
	if CORSConfigured() {
		log.Printf("CORS for public media enabled for: %s", strings.Join(Current.CORSOrigins, ", "))
		if slices.Contains(Current.CORSOrigins, "*") {
			log.Println("Warning: CORS_ORIGINS=* lets every website read public media with JavaScript. List the origins that need it instead.")
		}
	}
	if Current.InsecureSkipVerify {
		log.Println("Warning: INSECURE_SKIP_VERIFY=true — certificates of downloaded media are not validated. Use it only for a trusted internal source with a self-signed certificate.")
	}
	if Current.AdminPass != "" && len(Current.AdminPass) < MinRecommendedPassLen {
		log.Printf("Warning: ADMIN_PASS is shorter than %d characters. Any internet-facing deployment needs a long random password.",
			MinRecommendedPassLen)
	}
	if TLSMisconfigured() {
		log.Println("Warning: TLS_CERT_FILE and TLS_KEY_FILE must be set together; ignoring both.")
	}
}

func envString(name string, dst *string) {
	if v := os.Getenv(name); v != "" {
		*dst = v
	}
}

// envInt and envBool never silently drop a typo: an invalid value is logged
// and the previous (config.json or default) value is kept.
func envInt(name string, dst *int) {
	v := os.Getenv(name)
	if v == "" {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		log.Printf("Warning: ignoring invalid %s=%q (expected an integer)", name, v)
		return
	}
	*dst = n
}

func envBool(name string, dst *bool) {
	v := os.Getenv(name)
	if v == "" {
		return
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		log.Printf("Warning: ignoring invalid %s=%q (expected true or false)", name, v)
		return
	}
	*dst = b
}

// envList reads a comma-separated environment variable into a string slice.
// As with envInt/envBool an unset or empty variable keeps the previous value,
// and empty entries are dropped rather than stored.
func envList(name string, dst *[]string) {
	v := os.Getenv(name)
	if strings.TrimSpace(v) == "" {
		return
	}
	*dst = splitList(v)
}

// splitList splits a comma-separated setting, trimming space and dropping
// empty entries.
func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseTrustedProxyValue parses a single TrustedProxy string.
func parseTrustedProxyValue(s string) (*net.IP, *net.IPNet, error) {
	if s == "" {
		return nil, nil, nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return &ip, nil, nil
	}
	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		return nil, nil, err
	}
	return nil, cidr, nil
}

// IsTrustedProxy reports whether remoteAddr matches any entry of the
// configured TrustedProxy list.
func IsTrustedProxy(remoteAddr string) bool {
	p := cachedProxyPtr.Load()
	if p == nil || (len(p.ips) == 0 && len(p.cidrs) == 0) {
		return false
	}
	host, _, splitErr := net.SplitHostPort(remoteAddr)
	if splitErr != nil {
		host = remoteAddr
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return false
	}
	for _, ip := range p.ips {
		if ip.Equal(remote) {
			return true
		}
	}
	for _, cidr := range p.cidrs {
		if cidr.Contains(remote) {
			return true
		}
	}
	return false
}

// validate clamps out-of-range values to safe defaults. Missing admin
// credentials are deliberately NOT treated as "auth disabled": the admin
// endpoints then fail closed (503) while public links and health checks keep
// working. DISABLE_AUTH=true is the only way to turn authentication off.
// ApplyTrustedProxy re-reads Current.TrustedProxy and remembers what is
// actually in effect: invalid entries are dropped with a warning, and the
// setting is normalised to the entries that were accepted. Load calls it once
// at startup; tests that change the setting call it again to refresh the cache
// IsTrustedProxy reads.
func ApplyTrustedProxy() {
	var proxies parsedProxy
	valid := make([]string, 0, 2)
	for _, entry := range splitList(Current.TrustedProxy) {
		ip, cidr, err := parseTrustedProxyValue(entry)
		if err != nil {
			log.Printf("Warning: invalid TRUSTED_PROXY entry %q — ignoring (must be an IP or CIDR)", entry)
			continue
		}
		if ip != nil {
			proxies.ips = append(proxies.ips, ip)
		}
		if cidr != nil {
			proxies.cidrs = append(proxies.cidrs, cidr)
		}
		valid = append(valid, entry)
	}
	if len(proxies.ips) == 0 && len(proxies.cidrs) == 0 {
		if strings.TrimSpace(Current.TrustedProxy) != "" {
			log.Printf("Warning: TRUSTED_PROXY %q has no valid entries — ignoring (comma-separated IPs or CIDRs)", Current.TrustedProxy)
		}
		Current.TrustedProxy = ""
	} else {
		// Keep what is actually in effect: the invalid entries were dropped
		// above and must not be reported as trusted.
		Current.TrustedProxy = strings.Join(valid, ",")
	}
	cachedProxyPtr.Store(&proxies)
}

func validate() {
	portStr := strings.TrimPrefix(Current.Port, ":")
	if n, err := strconv.Atoi(portStr); err != nil || n < 1 || n > 65535 {
		log.Printf("Warning: invalid port %q, using 8080", Current.Port)
		Current.Port = "8080"
	}

	if Current.MaxUploadMB < MinUploadMB || Current.MaxUploadMB > MaxUploadMBLimit {
		log.Printf("Warning: MaxUploadMB %d out of range (%d-%d), using %d", Current.MaxUploadMB, MinUploadMB, MaxUploadMBLimit, DefaultMaxUploadMB)
		Current.MaxUploadMB = DefaultMaxUploadMB
	}
	if Current.MaxImages < 0 {
		Current.MaxImages = 0
	}
	if Current.MaxConcurrentUploads <= 0 || Current.MaxConcurrentUploads > MaxConcurrentUploadsLimit {
		Current.MaxConcurrentUploads = DefaultMaxConcurrentUploads
	}
	if Current.MaxWalkDepth <= 0 || Current.MaxWalkDepth > 10 {
		log.Printf("Warning: MaxWalkDepth %d out of range (1-10), using %d", Current.MaxWalkDepth, DefaultMaxWalkDepth)
		Current.MaxWalkDepth = DefaultMaxWalkDepth
	}

	if Current.Rate.PublicPerMin < 0 {
		Current.Rate.PublicPerMin = DefaultPublicRatePerMin
	}
	if Current.Rate.UploadPerMin < 0 {
		Current.Rate.UploadPerMin = DefaultUploadRatePerMin
	}
	if Current.Rate.Burst <= 0 {
		Current.Rate.Burst = DefaultRateBurst
	}

	if Current.Compression.Quality < 1 || Current.Compression.Quality > 100 {
		log.Printf("Warning: COMPRESSION_QUALITY %d out of range (1-100), using %d", Current.Compression.Quality, DefaultCompressionQuality)
		Current.Compression.Quality = DefaultCompressionQuality
	}
	if Current.Compression.Scale < 1 || Current.Compression.Scale > 100 {
		log.Printf("Warning: COMPRESSION_SCALE %d out of range (1-100), using %d", Current.Compression.Scale, DefaultCompressionScale)
		Current.Compression.Scale = DefaultCompressionScale
	}

	if Current.ProxyHost != "" {
		switch Current.ProxyType {
		case "http", "https", "socks5":
		default:
			log.Printf("Warning: invalid proxy type %q, using http", Current.ProxyType)
			Current.ProxyType = "http"
		}
	}

	ApplyTrustedProxy()
	if Current.History.Limit < 0 || Current.History.Limit > MaxHistoryLimit {
		log.Printf("Warning: HISTORY_LIMIT %d out of range (0-%d), using %d",
			Current.History.Limit, MaxHistoryLimit, DefaultHistoryLimit)
		Current.History.Limit = DefaultHistoryLimit
	}
	if Current.History.MaxMB < 0 || Current.History.MaxMB > MaxHistoryMaxMB {
		log.Printf("Warning: HISTORY_MAX_MB %d out of range (0-%d), using %d",
			Current.History.MaxMB, MaxHistoryMaxMB, DefaultHistoryMaxMB)
		Current.History.MaxMB = DefaultHistoryMaxMB
	}
	if Current.PlaylistMax <= 0 || Current.PlaylistMax > MaxPlaylistItems {
		log.Printf("Warning: PLAYLIST_MAX %d out of range (1-%d), using %d",
			Current.PlaylistMax, MaxPlaylistItems, DefaultPlaylistMax)
		Current.PlaylistMax = DefaultPlaylistMax
	}

	// Publish keys and CORS origins are matched from parsed snapshots so the
	// request path never locks, splits strings or hashes a list.
	RefreshDerived()
}
