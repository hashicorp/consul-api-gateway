package core

import "encoding/json"

type HTTPService struct {
	Service ResolvedService
	Weight  int32
	Filters []HTTPFilter
}

type HTTPFilterType string

const (
	HTTPHeaderFilterType     HTTPFilterType = "HTTPHeaderFilter"
	HTTPRedirectFilterType   HTTPFilterType = "HTTPRedirectFilter"
	HTTPURLRewriteFilterType HTTPFilterType = "HTTPURLRewriteFilter"
)

type HTTPHeaderFilter struct {
	Set    map[string]string
	Add    map[string]string
	Remove []string
}

type HTTPRedirectFilter struct {
	Scheme   string
	Hostname string
	Port     int
	Status   int
}

type URLRewriteType string

const (
	URLRewriteReplacePrefixMatchType URLRewriteType = "URLRewriteReplacePrefixMatch"
)

type HTTPURLRewriteFilter struct {
	Type               URLRewriteType
	ReplacePrefixMatch string
}

type HTTPFilter struct {
	Type       HTTPFilterType
	Header     HTTPHeaderFilter
	Redirect   HTTPRedirectFilter
	URLRewrite HTTPURLRewriteFilter
}

type HTTPMethod string

const (
	HTTPMethodNone    HTTPMethod = ""
	HTTPMethodConnect HTTPMethod = "CONNECT"
	HTTPMethodDelete  HTTPMethod = "DELETE"
	HTTPMethodGet     HTTPMethod = "GET"
	HTTPMethodHead    HTTPMethod = "HEAD"
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	HTTPMethodPatch   HTTPMethod = "PATCH"
	HTTPMethodPost    HTTPMethod = "POST"
	HTTPMethodPut     HTTPMethod = "PUT"
	HTTPMethodTrace   HTTPMethod = "TRACE"
)

type HTTPPathMatchType string

const (
	HTTPPathMatchNoneType              HTTPPathMatchType = ""
	HTTPPathMatchExactType             HTTPPathMatchType = "HTTPPathMatchExact"
	HTTPPathMatchPrefixType            HTTPPathMatchType = "HTTPPathMatchPrefix"
	HTTPPathMatchRegularExpressionType HTTPPathMatchType = "HTTPPathMatchRegularExpression"
)

type HTTPPathMatch struct {
	Type  HTTPPathMatchType
	Value string
}

type HTTPHeaderMatchType string

const (
	HTTPHeaderMatchNoneType              HTTPHeaderMatchType = ""
	HTTPHeaderMatchExactType             HTTPHeaderMatchType = "HTTPHeaderMatchExact"
	HTTPHeaderMatchPrefixType            HTTPHeaderMatchType = "HTTPHeaderMatchPrefix"
	HTTPHeaderMatchSuffixType            HTTPHeaderMatchType = "HTTPHeaderMatchSuffix"
	HTTPHeaderMatchPresentType           HTTPHeaderMatchType = "HTTPHeaderMatchPresent"
	HTTPHeaderMatchRegularExpressionType HTTPHeaderMatchType = "HTTPHeaderMatchRegularExpression"
)

type HTTPHeaderMatch struct {
	Type  HTTPHeaderMatchType
	Name  string
	Value string
}

type HTTPQueryMatchType string

const (
	HTTPQueryMatchNoneType              HTTPQueryMatchType = ""
	HTTPQueryMatchExactType             HTTPQueryMatchType = "HTTPQueryMatchExact"
	HTTPQueryMatchPresentType           HTTPQueryMatchType = "HTTPQueryMatchPresent"
	HTTPQueryMatchRegularExpressionType HTTPQueryMatchType = "HTTPQueryMatchRegularExpression"
)

type HTTPQueryMatch struct {
	Type  HTTPQueryMatchType
	Name  string
	Value string
}

type HTTPMatch struct {
	Path    HTTPPathMatch
	Headers []HTTPHeaderMatch
	Query   []HTTPQueryMatch
	Method  HTTPMethod
}

type HTTPRouteRule struct {
	Matches  []HTTPMatch
	Filters  []HTTPFilter
	Services []HTTPService
}

type HTTPRoute struct {
	CommonRoute
	Hostnames []string
	Rules     []HTTPRouteRule
}

func (r HTTPRoute) GetType() ResolvedRouteType {
	return ResolvedHTTPRouteType
}

func (r HTTPRoute) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type HTTPRouteBuilder struct {
	meta      map[string]string
	name      string
	namespace string
	rules     []HTTPRouteRule
	hostnames []string
}

func (b *HTTPRouteBuilder) WithMeta(meta map[string]string) *HTTPRouteBuilder {
	b.meta = meta
	return b
}

func (b *HTTPRouteBuilder) WithName(name string) *HTTPRouteBuilder {
	b.name = name
	return b
}

func (b *HTTPRouteBuilder) WithNamespace(namespace string) *HTTPRouteBuilder {
	b.namespace = namespace
	return b
}

func (b *HTTPRouteBuilder) WithHostnames(hostnames []string) *HTTPRouteBuilder {
	b.hostnames = hostnames
	return b
}

func (b *HTTPRouteBuilder) WithRules(rules []HTTPRouteRule) *HTTPRouteBuilder {
	b.rules = rules
	return b
}

func (b *HTTPRouteBuilder) Build() ResolvedRoute {
	return HTTPRoute{
		CommonRoute: CommonRoute{
			Meta:      b.meta,
			Name:      b.name,
			Namespace: b.namespace,
		},
		Hostnames: b.hostnames,
		Rules:     b.rules,
	}
}

func NewHTTPRouteBuilder() *HTTPRouteBuilder {
	return &HTTPRouteBuilder{}
}
