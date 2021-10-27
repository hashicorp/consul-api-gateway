package core

type HTTPService struct {
	Service ResolvedService
	Weight  int32
	Filters []HTTPFilter
}

type HTTPFilterType int

const (
	HTTPHeaderFilterType HTTPFilterType = iota
	HTTPRedirectFilterType
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

type HTTPFilter struct {
	Type     HTTPFilterType
	Header   HTTPHeaderFilter
	Redirect HTTPRedirectFilter
}

type HTTPMethod int

const (
	HTTPMethodNone HTTPMethod = iota
	HTTPMethodConnect
	HTTPMethodDelete
	HTTPMethodGet
	HTTPMethodHead
	HTTPMethodOptions
	HTTPMethodPatch
	HTTPMethodPost
	HTTPMethodPut
	HTTPMethodTrace
)

type HTTPPathMatchType int

const (
	HTTPPathMatchNoneType HTTPPathMatchType = iota
	HTTPPathMatchExactType
	HTTPPathMatchPrefixType
	HTTPPathMatchRegularExpressionType
)

type HTTPPathMatch struct {
	Type  HTTPPathMatchType
	Value string
}

type HTTPHeaderMatchType int

const (
	HTTPHeaderMatchNoneType HTTPHeaderMatchType = iota
	HTTPHeaderMatchExactType
	HTTPHeaderMatchPrefixType
	HTTPHeaderMatchSuffixType
	HTTPHeaderMatchPresentType
	HTTPHeaderMatchRegularExpressionType
)

type HTTPHeaderMatch struct {
	Type  HTTPHeaderMatchType
	Name  string
	Value string
}

type HTTPQueryMatchType int

const (
	HTTPQueryMatchNoneType HTTPQueryMatchType = iota
	HTTPQueryMatchExactType
	HTTPQueryMatchPresentType
	HTTPQueryMatchRegularExpressionType
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
