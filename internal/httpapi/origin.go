package httpapi

import "net/url"

// trustedOrigins derives local development aliases only from the configured
// origin, never from request headers or the incoming hostname.
func trustedOrigins(options Options) map[string]bool {
	origins := map[string]bool{options.Origin: true}
	if !options.Development || options.SecureCookies {
		return origins
	}
	configured, err := url.Parse(options.Origin)
	if err != nil || (configured.Scheme != "http" && configured.Scheme != "https") || configured.User != nil || configured.Path != "" || configured.RawQuery != "" || configured.Fragment != "" {
		return origins
	}
	var alias string
	switch configured.Hostname() {
	case "127.0.0.1":
		alias = "localhost"
	case "localhost":
		alias = "127.0.0.1"
	default:
		return origins
	}
	if port := configured.Port(); port != "" {
		alias += ":" + port
	}
	configured.Host = alias
	origins[configured.String()] = true
	return origins
}
