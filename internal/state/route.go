package state

type commonRoute struct {
	meta      map[string]string
	name      string
	namespace string
}

func (c commonRoute) Meta() map[string]string {
	return c.meta
}

func (c commonRoute) Name() string {
	return c.name
}

func (c commonRoute) Namespace() string {
	return c.namespace
}
